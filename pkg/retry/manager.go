// Package retry provides retry logic for task execution with configurable policies
package retry

import (
	"context"
	"fmt"
	"time"

	"github.com/ilibx/octopus/pkg/channels"
	"github.com/ilibx/octopus/pkg/kanban/types"
	"github.com/ilibx/octopus/pkg/logger"
)

// RetryManager handles task retry logic and failure notifications
type RetryManager struct {
	channelMgr *channels.Manager
}

// NewRetryManager creates a new retry manager
func NewRetryManager(channelMgr *channels.Manager) *RetryManager {
	return &RetryManager{
		channelMgr: channelMgr,
	}
}

// ShouldRetry determines if a task should be retried based on its configuration
func (rm *RetryManager) ShouldRetry(task *types.Task) bool {
	if task.MaxRetries <= 0 {
		return false // No retry configured
	}
	return task.RetryCount < task.MaxRetries
}

// ExecuteWithRetry executes a function with retry logic
func (rm *RetryManager) ExecuteWithRetry(ctx context.Context, task *types.Task, executeFn func() error) error {
	var lastErr error

	for task.RetryCount <= task.MaxRetries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		lastErr = executeFn()
		if lastErr == nil {
			// Success
			return nil
		}

		task.RetryCount++
		logger.WarnCF("retry", "Task execution failed, will retry",
			map[string]any{
				"task_id":     task.ID,
				"trace_id":    task.TraceID,
				"retry_count": task.RetryCount,
				"max_retries": task.MaxRetries,
				"error":       lastErr.Error(),
			})

		if task.RetryCount > task.MaxRetries {
			break
		}

		// Wait before retry (exponential backoff could be added here)
		time.Sleep(1 * time.Second)
	}

	// All retries exhausted
	logger.ErrorCF("retry", "Task failed after all retries",
		map[string]any{
			"task_id":     task.ID,
			"trace_id":    task.TraceID,
			"retry_count": task.RetryCount,
			"error":       lastErr.Error(),
		})

	return fmt.Errorf("task failed after %d retries: %w", task.RetryCount, lastErr)
}

// NotifyFailure sends failure notifications to configured channels
func (rm *RetryManager) NotifyFailure(task *types.Task, notifyChannels []string, errorMsg string) {
	for _, channelID := range notifyChannels {
		channel, err := rm.channelMgr.GetChannel(channelID)
		if err != nil {
			logger.WarnCF("retry", "Failed to get channel for failure notification",
				map[string]any{
					"channel_id": channelID,
					"task_id":    task.ID,
					"error":      err.Error(),
				})
			continue
		}

		message := fmt.Sprintf("❌ Task Failed: %s\nError: %s\nRetries: %d/%d",
			task.Title, errorMsg, task.RetryCount, task.MaxRetries)

		if err := channel.SendMessage(task.TraceID, message); err != nil {
			logger.WarnCF("retry", "Failed to send failure notification",
				map[string]any{
					"channel_id": channelID,
					"task_id":    task.ID,
					"error":      err.Error(),
				})
		} else {
			logger.InfoCF("retry", "Failure notification sent",
				map[string]any{
					"channel_id": channelID,
					"task_id":    task.ID,
				})
		}
	}
}

// GetRetryConfig returns the retry configuration for a task
func (rm *RetryManager) GetRetryConfig(maxRetries int, backoff time.Duration, notifyChannels []string) types.RetryConfig {
	return types.RetryConfig{
		MaxRetries:      maxRetries,
		Backoff:         backoff,
		NotifyOnFailure: notifyChannels,
	}
}

// ApplyRetryConfig applies retry configuration to a task
func (rm *RetryManager) ApplyRetryConfig(task *types.Task, config types.RetryConfig) {
	task.MaxRetries = config.MaxRetries
	task.RetryCount = 0
	// Store notification channels in metadata
	if task.Metadata == nil {
		task.Metadata = make(map[string]string)
	}

	// Convert slice to comma-separated string for storage
	for i, ch := range config.NotifyOnFailure {
		task.Metadata[fmt.Sprintf("notify_channel_%d", i)] = ch
	}
}

// ExtractNotifyChannels extracts notification channels from task metadata
func (rm *RetryManager) ExtractNotifyChannels(task *types.Task) []string {
	var channels []string
	if task.Metadata == nil {
		return channels
	}

	i := 0
	for {
		key := fmt.Sprintf("notify_channel_%d", i)
		ch, exists := task.Metadata[key]
		if !exists {
			break
		}
		channels = append(channels, ch)
		i++
	}

	return channels
}
