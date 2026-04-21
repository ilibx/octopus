package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/approval"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/decomposer"
	kanbanTypes "github.com/ilibx/octopus/pkg/kanban/types"
	"github.com/ilibx/octopus/pkg/logger"
	"github.com/ilibx/octopus/pkg/retry"
	"github.com/ilibx/octopus/pkg/trace"
)

// TaskEvent represents an event published when task status changes
type TaskEvent struct {
	Type      string     `json:"type"` // "task_created", "task_updated", "task_completed"
	BoardID   string     `json:"board_id"`
	ZoneID    string     `json:"zone_id"`
	TaskID    string     `json:"task_id"`
	Status    TaskStatus `json:"status,omitempty"`
	Title     string     `json:"title,omitempty"`
	Result    string     `json:"result,omitempty"`
	Error     string     `json:"error,omitempty"`
	Timestamp int64      `json:"timestamp"`
}

// KanbanService manages the kanban board and publishes events
type KanbanService struct {
	board           *KanbanBoard
	msgBus          *bus.MessageBus
	mu              sync.RWMutex
	subject         string
	wsHub           *WSHub
	wsEnabled       bool
	traceManager    *trace.TraceManager
	retryManager    *retry.RetryManager
	approvalManager *approval.ApprovalManager
	decomposer      *decomposer.TaskDecomposer
	channelManager  *channels.Manager
}

// NewKanbanService creates a new kanban service
func NewKanbanService(board *KanbanBoard, msgBus *bus.MessageBus) *KanbanService {
	return &KanbanService{
		board:     board,
		msgBus:    msgBus,
		subject:   "kanban.events",
		wsEnabled: false,
	}
}

// NewEnhancedKanbanService creates a kanban service with full feature support (trace, retry, approval, decomposer)
func NewEnhancedKanbanService(
	board *KanbanBoard,
	msgBus *bus.MessageBus,
	channelMgr *channels.Manager,
	decomposerInst *decomposer.TaskDecomposer,
	defaultApprovalTimeout time.Duration,
) *KanbanService {
	traceMgr := trace.NewTraceManager()
	retryMgr := retry.NewRetryManager(channelMgr)
	approvalMgr := approval.NewApprovalManager(channelMgr, defaultApprovalTimeout)

	return &KanbanService{
		board:           board,
		msgBus:          msgBus,
		subject:         "kanban.events",
		wsEnabled:       false,
		traceManager:    traceMgr,
		retryManager:    retryMgr,
		approvalManager: approvalMgr,
		decomposer:      decomposerInst,
		channelManager:  channelMgr,
	}
}

// GetTraceManager returns the trace manager for external access
func (s *KanbanService) GetTraceManager() *trace.TraceManager {
	return s.traceManager
}

// GetRetryManager returns the retry manager for external access
func (s *KanbanService) GetRetryManager() *retry.RetryManager {
	return s.retryManager
}

// GetApprovalManager returns the approval manager for external access
func (s *KanbanService) GetApprovalManager() *approval.ApprovalManager {
	return s.approvalManager
}

// GetDecomposer returns the task decomposer for external access
func (s *KanbanService) GetDecomposer() *decomposer.TaskDecomposer {
	return s.decomposer
}

// EnableWebSocket enables WebSocket support for real-time updates
func (s *KanbanService) EnableWebSocket() {
	s.wsHub = NewWSHub(s.msgBus)
	s.wsEnabled = true
	go s.wsHub.Run()
	logger.InfoCF("kanban", "WebSocket support enabled", nil)
}

// GetWSHub returns the WebSocket hub if enabled
func (s *KanbanService) GetWSHub() *WSHub {
	return s.wsHub
}

// PublishTaskEvent publishes a task event to the message bus
func (s *KanbanService) PublishTaskEvent(eventType, zoneID, taskID string, status TaskStatus, title, result, errMsg string) {
	event := TaskEvent{
		Type:      eventType,
		BoardID:   s.board.ID,
		ZoneID:    zoneID,
		TaskID:    taskID,
		Status:    status,
		Title:     title,
		Result:    result,
		Error:     errMsg,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		logger.ErrorCF("kanban", "Failed to marshal task event",
			map[string]any{"error": err.Error()})
		return
	}

	s.msgBus.Publish(s.subject, string(data))
	logger.DebugCF("kanban", "Published task event",
		map[string]any{
			"event_type": eventType,
			"zone_id":    zoneID,
			"task_id":    taskID,
		})
}

// CreateTaskWithEvent creates a task and publishes an event with full trace support
func (s *KanbanService) CreateTaskWithEvent(zoneID, taskID, title, description string, priority int, metadata map[string]string) (*Task, error) {
	// Start trace for this task
	var traceID string
	if s.traceManager != nil {
		traceID = s.traceManager.StartTrace(taskID, title, metadata)
	}

	task, err := s.board.AddTask(zoneID, taskID, title, description, priority, metadata)
	if err != nil {
		return nil, err
	}

	s.PublishTaskEvent("task_created", zoneID, taskID, TaskPending, title, "", "")
	return task, nil
}

// DecomposeAndCreateTasks uses LLM to decompose a user request into sub-tasks and creates them on the board
func (s *KanbanService) DecomposeAndCreateTasks(ctx context.Context, zoneID, mainTaskID, title, description string, priority int) ([]*Task, error) {
	if s.decomposer == nil {
		return nil, fmt.Errorf("decomposer not configured")
	}

	// Use LLM to decompose the task
	result, err := s.decomposer.DecomposeTask(ctx, mainTaskID, title, description)
	if err != nil {
		return nil, fmt.Errorf("task decomposition failed: %w", err)
	}

	logger.InfoCF("kanban", "Task decomposed by LLM",
		map[string]any{
			"trace_id":       result.TraceID,
			"main_title":     result.MainTaskTitle,
			"sub_task_count": len(result.SubTasks),
			"agent_type":     result.AgentType,
		})

	// Create sub-tasks from decomposition result
	var createdTasks []*Task
	for i, subTask := range result.SubTasks {
		subTaskID := fmt.Sprintf("%s_sub_%d", mainTaskID, i+1)

		// Build metadata with trace info and skill IDs
		subTaskMetadata := make(map[string]string)
		if metadata != nil {
			for k, v := range metadata {
				subTaskMetadata[k] = v
			}
		}
		subTaskMetadata["trace_id"] = result.TraceID
		subTaskMetadata["parent_task_id"] = mainTaskID

		// Add skill IDs as comma-separated string
		if len(subTask.SkillIDs) > 0 {
			subTaskMetadata["skill_ids"] = strings.Join(subTask.SkillIDs, ",")
		}

		// Create the sub-task
		task, err := s.CreateTaskWithEvent(zoneID, subTaskID, subTask.Title, subTask.Description, priority, subTaskMetadata)
		if err != nil {
			logger.WarnCF("kanban", "Failed to create sub-task",
				map[string]any{"sub_task_id": subTaskID, "error": err.Error()})
			continue
		}

		createdTasks = append(createdTasks, task)

		// Record dependency if needed
		if len(subTask.DependsOn) > 0 {
			s.setTaskDependencies(zoneID, subTaskID, subTask.DependsOn)
		}
	}

	return createdTasks, nil
}

// setTaskDependencies sets dependencies for a task
func (s *KanbanService) setTaskDependencies(zoneID, taskID string, dependsOn []string) {
	zone, err := s.board.GetZone(zoneID)
	if err != nil {
		return
	}

	for _, task := range zone.Tasks {
		if task.ID == taskID {
			task.DependsOn = dependsOn
			break
		}
	}
}

// UpdateTaskStatusWithEvent updates task status and publishes an event with retry and approval support
func (s *KanbanService) UpdateTaskStatusWithEvent(zoneID, taskID string, status TaskStatus, result, errorMsg string) error {
	err := s.board.UpdateTaskStatus(zoneID, taskID, status, result, errorMsg)
	if err != nil {
		return err
	}

	eventType := "task_updated"
	if status == TaskCompleted || status == TaskFailed {
		eventType = "task_completed"
	}

	var title string
	var traceID string
	var retryCount int
	zone, err := s.board.GetZone(zoneID)
	if err == nil {
		for _, task := range zone.Tasks {
			if task.ID == taskID {
				title = task.Title
				traceID = task.TraceID
				retryCount = task.RetryCount
				break
			}
		}
	}

	// Handle task failure with retry logic
	if status == TaskFailed && s.retryManager != nil {
		s.handleTaskFailure(zoneID, taskID, title, traceID, errorMsg, retryCount)
	}

	// Update trace if available
	if s.traceManager != nil && traceID != "" {
		if status == TaskCompleted || status == TaskFailed {
			// End the trace span
			s.traceManager.EndSpan(traceID, string(status))
		} else {
			s.traceManager.UpdateStatus(traceID, string(status))
		}
	}

	s.PublishTaskEvent(eventType, zoneID, taskID, status, title, result, errorMsg)
	return nil
}

// handleTaskFailure handles task failure with retry and notification logic
func (s *KanbanService) handleTaskFailure(zoneID, taskID, title, traceID, errorMsg string, retryCount int) {
	zone, err := s.board.GetZone(zoneID)
	if err != nil {
		return
	}

	var task *Task
	for _, t := range zone.Tasks {
		if t.ID == taskID {
			task = t
			break
		}
	}

	if task == nil {
		return
	}

	// Check if we should retry
	if s.retryManager.ShouldRetry(task) {
		logger.InfoCF("kanban", "Task will be retried",
			map[string]any{
				"task_id":     taskID,
				"trace_id":    traceID,
				"retry_count": task.RetryCount,
				"max_retries": task.MaxRetries,
			})
		// Reset task to pending for retry
		task.Status = TaskPending
		task.RetryCount++
		return
	}

	// All retries exhausted, notify configured channels
	notifyChannels := s.retryManager.ExtractNotifyChannels(task)
	if len(notifyChannels) > 0 {
		s.retryManager.NotifyFailure(task, notifyChannels, errorMsg)
	}
}

// RequestApproval creates an approval request for a task (HITL)
func (s *KanbanService) RequestApproval(taskID string, approvers []string, reason string) error {
	if s.approvalManager == nil {
		return fmt.Errorf("approval manager not configured")
	}

	// Search in all zones for the task
	for _, z := range s.board.Zones {
		for _, task := range z.Tasks {
			if task.ID == taskID {
				return s.approvalManager.RequestApproval(&kanbanTypes.Task{
					ID:       task.ID,
					TraceID:  task.TraceID,
					Title:    task.Title,
					Status:   kanbanTypes.TaskStatus(task.Status),
					Approval: task.Approval,
					Metadata: task.Metadata,
				}, approvers, reason)
			}
		}
	}

	return ErrTaskNotFound
}

// SubscribeToEvents subscribes to kanban events
func (s *KanbanService) SubscribeToEvents(handler func(string)) {
	s.msgBus.Subscribe(s.subject, handler)
}

// GetBoard returns the underlying kanban board
func (s *KanbanService) GetBoard() *KanbanBoard {
	return s.board
}

// HTTPHandler returns an HTTP handler for kanban API endpoints
// Note: Direct task creation via HTTP is disabled to enforce that only cron and channels can submit tasks to the main agent
func (s *KanbanService) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// GET /kanban - Get board status (read-only)
	mux.HandleFunc("/kanban", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		s.mu.RLock()
		board := s.board
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(board)
	})

	// GET /kanban/zones/{zoneID} - Get zone details (read-only)
	mux.HandleFunc("/kanban/zones/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		zoneID := r.URL.Path[len("/kanban/zones/"):]
		if zoneID == "" {
			http.Error(w, "Zone ID required", http.StatusBadRequest)
			return
		}

		zone, err := s.board.GetZone(zoneID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(zone)
	})

	// GET /kanban/tasks/{zoneID} - Get tasks in a zone (read-only)
	mux.HandleFunc("/kanban/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse zoneID and optional status from query params
		zoneID := r.URL.Query().Get("zone")
		statusStr := r.URL.Query().Get("status")

		if zoneID == "" {
			http.Error(w, "Zone ID required", http.StatusBadRequest)
			return
		}

		var tasks []*Task
		if statusStr != "" {
			tasks = s.board.GetTasksByStatus(zoneID, TaskStatus(statusStr))
		} else {
			zone, err := s.board.GetZone(zoneID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			tasks = zone.Tasks
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tasks)
	})

	// WebSocket endpoint for real-time updates (read-only subscription)
	if s.wsEnabled {
		mux.HandleFunc("/kanban/ws", s.wsHub.HandleWebSocket)
	}

	// NOTE: Task creation endpoints are intentionally NOT provided here.
	// Tasks can ONLY be created through:
	// 1. Cron jobs (via CronKanbanIntegration)
	// 2. Channel commands (via Channel Integration Layer)
	// This ensures strict control over task submission to the Main Agent.

	return mux
}

// StartStatusReporter starts a goroutine that reports task status changes to channels
func (s *KanbanService) StartStatusReporter(ctx context.Context) {
	go func() {
		handler := func(msg string) {
			var event TaskEvent
			if err := json.Unmarshal([]byte(msg), &event); err != nil {
				logger.ErrorCF("kanban", "Failed to unmarshal task event",
					map[string]any{"error": err.Error()})
				return
			}

			// Format status report message
			report := fmt.Sprintf("📋 Task Update: %s\nZone: %s\nStatus: %s",
				event.Title, event.ZoneID, event.Status)

			if event.Result != "" {
				report += fmt.Sprintf("\nResult: %s", event.Result)
			}
			if event.Error != "" {
				report += fmt.Sprintf("\nError: %s", event.Error)
			}

			// Publish to message bus for channels to pick up
			s.msgBus.Publish("channel.broadcast", report)
			logger.InfoCF("kanban", "Status reported to channels",
				map[string]any{"task_id": event.TaskID, "status": event.Status})
		}

		s.SubscribeToEvents(handler)

		<-ctx.Done()
	}()
}
