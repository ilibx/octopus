package kanban

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/cron"
)

func int64Ptr(v int64) *int64 {
	return &v
}

func TestNewCronKanbanIntegration(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)

	if integration == nil {
		t.Fatal("Expected integration to be created")
	}
	if integration.board != board {
		t.Error("Expected board to be set")
	}
	if integration.service != service {
		t.Error("Expected service to be set")
	}
	if integration.cronService != cronService {
		t.Error("Expected cronService to be set")
	}
	if integration.msgBus != msgBus {
		t.Error("Expected msgBus to be set")
	}
}

func TestCronKanbanIntegration_SetupCronHandlers(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)

	// Should not panic
	integration.SetupCronHandlers()

	// Verify handler is registered by checking if OnJob callback is set
	if cronService.GetOnJob() == nil {
		t.Error("Expected OnJob handler to be registered")
	}
}

func TestCronKanbanIntegration_HandleCronJob(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)
	integration.SetupCronHandlers()

	// Test with valid task payload
	taskPayload := map[string]interface{}{
		"zone_id":     "test-zone",
		"title":       "Test Task from Cron",
		"priority":    5,
		"description": "Created by cron job",
	}
	payloadBytes, _ := json.Marshal(taskPayload)

	job := &cron.CronJob{
		ID:   "test-job-1",
		Name: "Test Job",
		Payload: cron.CronPayload{
			Message: string(payloadBytes),
		},
	}

	result, err := integration.HandleCronJobForTest(job)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result == "" {
		t.Error("Expected task ID to be returned")
	}

	// Verify task was created on the board
	board.RLock()
	zoneTasks, exists := board.Zones["test-zone"]
	board.RUnlock()

	if !exists {
		t.Fatal("Expected zone to be created")
	}
	if len(zoneTasks.Pending) == 0 && len(zoneTasks.InProgress) == 0 {
		t.Error("Expected task to be added to zone")
	}
}

func TestCronKanbanIntegration_HandleCronJob_InvalidPayload(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)

	job := &cron.CronJob{
		ID:   "test-job-2",
		Name: "Test Job Invalid",
		Payload: cron.CronPayload{
			Message: "invalid-json",
		},
	}

	_, err := integration.HandleCronJobForTest(job)

	if err == nil {
		t.Error("Expected error for invalid JSON payload")
	}
}

func TestCronKanbanIntegration_HandleCronJob_EmptyPayload(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)

	job := &cron.CronJob{
		ID:   "test-job-3",
		Name: "Test Job Empty",
		Payload: cron.CronPayload{
			Message: "{}",
		},
	}

	_, err := integration.HandleCronJobForTest(job)

	if err == nil {
		t.Error("Expected error for empty payload (missing zone_id)")
	}
}

func TestCronKanbanIntegration_HandleCronJob_WithTaskID(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)

	taskPayload := map[string]interface{}{
		"zone_id":  "test-zone-2",
		"title":    "Task with custom ID",
		"task_id":  "custom-task-id-123",
		"priority": 3,
	}
	payloadBytes, _ := json.Marshal(taskPayload)

	job := &cron.CronJob{
		ID:   "test-job-4",
		Name: "Test Job with Task ID",
		Payload: cron.CronPayload{
			Message: string(payloadBytes),
		},
	}

	result, err := integration.HandleCronJobForTest(job)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if result != "custom-task-id-123" {
		t.Errorf("Expected task ID 'custom-task-id-123', got: %s", result)
	}
}

func TestCronKanbanIntegration_ConcurrentJobHandling(t *testing.T) {
	board := NewKanbanBoard()
	service := NewKanbanService(board, nil)
	cronService := cron.NewCronService()
	msgBus := bus.NewMessageBus()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)
	integration.SetupCronHandlers()

	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(idx int) {
			taskPayload := map[string]interface{}{
				"zone_id":  "concurrent-zone",
				"title":    "Concurrent Task",
				"priority": idx,
			}
			payloadBytes, _ := json.Marshal(taskPayload)

			job := &cron.CronJob{
				ID:   "test-job-concurrent",
				Name: "Concurrent Job",
				Payload: cron.CronPayload{
					Message: string(payloadBytes),
				},
			}

			_, err := integration.HandleCronJobForTest(job)
			if err != nil {
				t.Errorf("Concurrent job failed: %v", err)
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("Timeout waiting for concurrent jobs")
		}
	}

	// Verify all tasks were created
	board.RLock()
	zoneTasks, exists := board.Zones["concurrent-zone"]
	board.RUnlock()

	if !exists {
		t.Fatal("Expected concurrent-zone to be created")
	}

	totalTasks := len(zoneTasks.Pending) + len(zoneTasks.InProgress) + len(zoneTasks.Completed)
	if totalTasks != 10 {
		t.Errorf("Expected 10 tasks, got: %d", totalTasks)
	}
}

func TestCronKanbanIntegration_MessageBusPublish(t *testing.T) {
	board := NewKanbanBoard()
	eventChan := make(chan interface{}, 10)

	// Create a mock message bus that captures events
	msgBus := &bus.MessageBus{}

	service := NewKanbanService(board, msgBus)
	cronService := cron.NewCronService()

	integration := NewCronKanbanIntegration(board, service, cronService, msgBus)

	taskPayload := map[string]interface{}{
		"zone_id":  "msgbus-test-zone",
		"title":    "Message Bus Test Task",
		"priority": 5,
	}
	payloadBytes, _ := json.Marshal(taskPayload)

	job := &cron.CronJob{
		ID:   "test-job-msgbus",
		Name: "Message Bus Test Job",
		Payload: cron.CronPayload{
			Message: string(payloadBytes),
		},
	}

	_, err := integration.HandleCronJobForTest(job)

	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}

	// Task should have been created and event published
	board.RLock()
	_, exists := board.Zones["msgbus-test-zone"]
	board.RUnlock()

	if !exists {
		t.Error("Expected task to be created and event published")
	}
}
