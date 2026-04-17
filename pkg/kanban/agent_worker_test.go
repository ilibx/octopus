package kanban

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/ilibx/octopus/pkg/agent"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/config"
)

// MockAgentInstance for testing
type MockAgentInstance struct {
	*agent.AgentInstance
}

func TestNewAgentWorker(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker(
		"zone1",
		"agent1",
		board,
		service,
		nil, // agentInstance can be nil for basic tests
		msgBus,
		cfg,
		5, // maxConcurrency
	)

	if worker == nil {
		t.Fatal("Expected worker to be created")
	}

	if worker.zoneID != "zone1" {
		t.Errorf("Expected zoneID 'zone1', got %s", worker.zoneID)
	}

	if worker.agentID != "agent1" {
		t.Errorf("Expected agentID 'agent1', got %s", worker.agentID)
	}

	if worker.maxConcurrency != 5 {
		t.Errorf("Expected maxConcurrency 5, got %d", worker.maxConcurrency)
	}

	if worker.currentTasks == nil {
		t.Error("Expected currentTasks map to be initialized")
	}

	if worker.ctx == nil {
		t.Error("Expected context to be initialized")
	}

	if worker.cancel == nil {
		t.Error("Expected cancel function to be initialized")
	}
}

func TestAgentWorker_ClaimTask(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Claim a task
	success := worker.claimTask("task1")
	if !success {
		t.Error("Expected to successfully claim task")
	}

	// Try to claim the same task again
	success = worker.claimTask("task1")
	if success {
		t.Error("Expected to fail claiming already claimed task")
	}

	// Claim a different task
	success = worker.claimTask("task2")
	if !success {
		t.Error("Expected to successfully claim second task")
	}
}

func TestAgentWorker_ReleaseTask(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Claim and release a task
	worker.claimTask("task1")
	worker.releaseTask("task1")

	// Should be able to claim again after release
	success := worker.claimTask("task1")
	if !success {
		t.Error("Expected to successfully claim released task")
	}
}

func TestAgentWorker_GetActiveTasks(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Initially should have no active tasks
	count := worker.GetActiveTasks()
	if count != 0 {
		t.Errorf("Expected 0 active tasks initially, got %d", count)
	}

	// Claim some tasks
	worker.claimTask("task1")
	worker.claimTask("task2")
	worker.claimTask("task3")

	count = worker.GetActiveTasks()
	if count != 3 {
		t.Errorf("Expected 3 active tasks, got %d", count)
	}

	// Release one
	worker.releaseTask("task1")

	count = worker.GetActiveTasks()
	if count != 2 {
		t.Errorf("Expected 2 active tasks after release, got %d", count)
	}
}

func TestAgentWorker_FetchNextPendingTask(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Create a zone and add tasks with different priorities
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = board.AddTask("zone1", "task1", "Low Priority", "Description", 1, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	_, err = board.AddTask("zone1", "task2", "High Priority", "Description", 10, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	_, err = board.AddTask("zone1", "task3", "Medium Priority", "Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Fetch next pending task - should return highest priority
	task := worker.fetchNextPendingTask()
	if task == nil {
		t.Fatal("Expected to fetch a task")
	}

	if task.ID != "task2" {
		t.Errorf("Expected highest priority task (task2), got %s", task.ID)
	}
}

func TestAgentWorker_Stop(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Start worker in background
	go worker.Start()

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	// Stop the worker
	worker.Stop()

	// Give it time to stop gracefully
	time.Sleep(200 * time.Millisecond)

	t.Log("Worker stopped test completed")
}

func TestAgentWorker_ConcurrentTaskManagement(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	var wg sync.WaitGroup

	// Concurrent claim operations
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(taskNum int) {
			defer wg.Done()
			taskID := "task" + string(rune('a'+taskNum%26))
			worker.claimTask(taskID)
			time.Sleep(time.Millisecond)
			worker.releaseTask(taskID)
		}(i)
	}

	wg.Wait()

	// After all operations, should have no active tasks
	count := worker.GetActiveTasks()
	if count != 0 {
		t.Errorf("Expected 0 active tasks after concurrent operations, got %d", count)
	}
}

func TestAgentWorker_MaxConcurrency(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	maxConcurrency := 3
	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, maxConcurrency)

	// Try to claim more tasks than max concurrency
	for i := 0; i < 5; i++ {
		taskID := "task" + string(rune('a'+i))
		worker.claimTask(taskID)
	}

	count := worker.GetActiveTasks()
	if count > maxConcurrency {
		t.Errorf("Expected at most %d active tasks, got %d", maxConcurrency, count)
	}
}

func TestAgentWorker_ContextCancellation(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Cancel the context immediately
	worker.cancel()

	// Worker should handle cancellation gracefully
	worker.Stop()

	t.Log("Context cancellation test completed")
}

func TestAgentWorker_GetZoneID(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	zoneID := worker.GetZoneID()
	if zoneID != "zone1" {
		t.Errorf("Expected zoneID 'zone1', got %s", zoneID)
	}
}

func TestAgentWorker_GetAgentID(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	agentID := worker.GetAgentID()
	if agentID != "agent1" {
		t.Errorf("Expected agentID 'agent1', got %s", agentID)
	}
}

func TestAgentWorker_ProcessTasksLoop(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Create a zone and add a task
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = board.AddTask("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Start processing in background
	done := make(chan bool)
	go func() {
		worker.processTasksLoop(0)
		done <- true
	}()

	// Let it process briefly
	time.Sleep(200 * time.Millisecond)

	// Stop the worker
	worker.Stop()

	// Wait for goroutine to finish
	select {
	case <-done:
		t.Log("ProcessTasksLoop completed successfully")
	case <-time.After(2 * time.Second):
		t.Error("ProcessTasksLoop did not complete in time")
	}
}

func TestAgentWorker_TryProcessNextTask(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)
	cfg := &config.Config{}

	worker := NewAgentWorker("zone1", "agent1", board, service, nil, msgBus, cfg, 5)

	// Create a zone and add a task
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = board.AddTask("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Try to process next task
	worker.tryProcessNextTask(0)

	// Give it time to start processing
	time.Sleep(100 * time.Millisecond)

	// Check if task was claimed
	count := worker.GetActiveTasks()
	t.Logf("Active tasks after tryProcessNextTask: %d", count)
}
