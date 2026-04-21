// Package approval provides Human-in-the-Loop (HITL) approval functionality
package approval

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/kanban/types"
	"github.com/ilibx/octopus/pkg/logger"
)

// ApprovalManager handles HITL approval workflows
type ApprovalManager struct {
	channelMgr       *channels.Manager
	pendingApprovals map[string]*types.ApprovalRequest // taskID -> ApprovalRequest
	mu               sync.RWMutex
	defaultTimeout   time.Duration
}

// NewApprovalManager creates a new approval manager
func NewApprovalManager(channelMgr *channels.Manager, defaultTimeout time.Duration) *ApprovalManager {
	return &ApprovalManager{
		channelMgr:       channelMgr,
		pendingApprovals: make(map[string]*types.ApprovalRequest),
		defaultTimeout:   defaultTimeout,
	}
}

// RequestApproval creates a new approval request and blocks task execution
func (am *ApprovalManager) RequestApproval(task *types.Task, approvers []string, reason string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	now := time.Now()
	approval := &types.ApprovalRequest{
		TaskID:    task.ID,
		Status:    types.ApprovalPending,
		Deadline:  now.Add(am.defaultTimeout),
		Reason:    reason,
		CreatedAt: now,
		UpdatedAt: now,
	}

	// Store the approval request
	am.pendingApprovals[task.ID] = approval

	// Update task with approval info
	task.Approval = approval
	task.Status = types.TaskWaitingApproval

	logger.InfoCF("approval", "Approval request created",
		map[string]any{
			"task_id":   task.ID,
			"trace_id":  task.TraceID,
			"approvers": approvers,
			"deadline":  approval.Deadline,
		})

	// Notify approvers
	go am.notifyApprovers(task, approvers, reason)

	// Start timeout watcher
	go am.watchApprovalTimeout(task.ID)

	return nil
}

// Approve approves a pending approval request
func (am *ApprovalManager) Approve(taskID, approver string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	approval, exists := am.pendingApprovals[taskID]
	if !exists {
		return fmt.Errorf("approval request not found for task: %s", taskID)
	}

	if approval.Status != types.ApprovalPending {
		return fmt.Errorf("approval request is not pending: %s", approval.Status)
	}

	approval.Status = types.ApprovalApproved
	approval.Approver = approver
	approval.UpdatedAt = time.Now()

	delete(am.pendingApprovals, taskID)

	logger.InfoCF("approval", "Approval granted",
		map[string]any{
			"task_id":  taskID,
			"approver": approver,
		})

	return nil
}

// Reject rejects a pending approval request
func (am *ApprovalManager) Reject(taskID, approver, reason string) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	approval, exists := am.pendingApprovals[taskID]
	if !exists {
		return fmt.Errorf("approval request not found for task: %s", taskID)
	}

	if approval.Status != types.ApprovalPending {
		return fmt.Errorf("approval request is not pending: %s", approval.Status)
	}

	approval.Status = types.ApprovalRejected
	approval.Approver = approver
	approval.Reason = reason
	approval.UpdatedAt = time.Now()

	delete(am.pendingApprovals, taskID)

	logger.WarnCF("approval", "Approval rejected",
		map[string]any{
			"task_id":  taskID,
			"approver": approver,
			"reason":   reason,
		})

	return nil
}

// GetApprovalStatus returns the status of an approval request
func (am *ApprovalManager) GetApprovalStatus(taskID string) (*types.ApprovalRequest, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	approval, exists := am.pendingApprovals[taskID]
	if !exists {
		return nil, fmt.Errorf("approval request not found for task: %s", taskID)
	}

	return approval, nil
}

// notifyApprovers sends approval notifications to specified channels/users
func (am *ApprovalManager) notifyApprovers(task *types.Task, approvers []string, reason string) {
	message := fmt.Sprintf("🔔 Approval Required\nTask: %s\nReason: %s\nReply 'approve' or 'reject'",
		task.Title, reason)

	for _, approver := range approvers {
		channel, err := am.channelMgr.GetChannel(approver)
		if err != nil {
			logger.WarnCF("approval", "Failed to get channel for approval notification",
				map[string]any{
					"channel_id": approver,
					"task_id":    task.ID,
					"error":      err.Error(),
				})
			continue
		}

		if err := channel.SendMessage(task.TraceID, message); err != nil {
			logger.WarnCF("approval", "Failed to send approval notification",
				map[string]any{
					"channel_id": approver,
					"task_id":    task.ID,
					"error":      err.Error(),
				})
		} else {
			logger.InfoCF("approval", "Approval notification sent",
				map[string]any{
					"channel_id": approver,
					"task_id":    task.ID,
				})
		}
	}
}

// watchApprovalTimeout watches for approval timeout
func (am *ApprovalManager) watchApprovalTimeout(taskID string) {
	am.mu.RLock()
	approval, exists := am.pendingApprovals[taskID]
	am.mu.RUnlock()

	if !exists {
		return
	}

	// Wait until deadline
	time.Sleep(time.Until(approval.Deadline))

	am.mu.Lock()
	defer am.mu.Unlock()

	// Check if still pending
	approval, exists = am.pendingApprovals[taskID]
	if exists && approval.Status == types.ApprovalPending {
		approval.Status = types.ApprovalTimeout
		approval.UpdatedAt = time.Now()
		delete(am.pendingApprovals, taskID)

		logger.WarnCF("approval", "Approval request timed out",
			map[string]any{
				"task_id": taskID,
			})
	}
}

// HandleApprovalResponse processes user response to approval request
func (am *ApprovalManager) HandleApprovalResponse(taskID, response, approver string) error {
	response = normalizeResponse(response)

	switch response {
	case "approve", "approved", "yes", "y":
		return am.Approve(taskID, approver)
	case "reject", "rejected", "no", "n":
		return am.Reject(taskID, approver, "User rejected via chat")
	default:
		return fmt.Errorf("unrecognized approval response: %s", response)
	}
}

// normalizeResponse normalizes user approval response
func normalizeResponse(response string) string {
	response = response[:len(response)/2] // Simple truncation for demo
	if len(response) > 20 {
		response = response[:20]
	}

	// In production, use proper string normalization
	return response
}

// CleanupExpiredApprovals removes expired approval requests
func (am *ApprovalManager) CleanupExpiredApprovals(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			am.mu.Lock()
			now := time.Now()
			for taskID, approval := range am.pendingApprovals {
				if now.After(approval.Deadline) && approval.Status == types.ApprovalPending {
					approval.Status = types.ApprovalTimeout
					approval.UpdatedAt = now
					delete(am.pendingApprovals, taskID)
					logger.WarnCF("approval", "Cleaned up expired approval",
						map[string]any{"task_id": taskID})
				}
			}
			am.mu.Unlock()
		}
	}
}

// GetPendingApprovalsCount returns the count of pending approvals
func (am *ApprovalManager) GetPendingApprovalsCount() int {
	am.mu.RLock()
	defer am.mu.RUnlock()
	return len(am.pendingApprovals)
}
