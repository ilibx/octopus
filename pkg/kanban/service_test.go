package kanban

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ilibx/octopus/pkg/bus"
)

func TestNewKanbanService(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()

	service := NewKanbanService(board, msgBus)

	if service == nil {
		t.Fatal("Expected service to be created")
	}

	if service.board != board {
		t.Error("Expected board to be set")
	}

	if service.msgBus != msgBus {
		t.Error("Expected msgBus to be set")
	}

	if service.subject != "kanban.events" {
		t.Errorf("Expected subject 'kanban.events', got %s", service.subject)
	}
}

func TestKanbanService_PublishTaskEvent(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	// Subscribe to events to verify publishing
	eventReceived := make(chan string, 1)
	service.SubscribeToEvents(func(msg string) {
		eventReceived <- msg
	})

	// Publish an event
	service.PublishTaskEvent("task_created", "zone1", "task1", TaskPending, "Test Task", "", "")

	// Wait for event
	select {
	case msg := <-eventReceived:
		var event TaskEvent
		err := json.Unmarshal([]byte(msg), &event)
		if err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if event.Type != "task_created" {
			t.Errorf("Expected type 'task_created', got %s", event.Type)
		}

		if event.ZoneID != "zone1" {
			t.Errorf("Expected zone_id 'zone1', got %s", event.ZoneID)
		}

		if event.TaskID != "task1" {
			t.Errorf("Expected task_id 'task1', got %s", event.TaskID)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestKanbanService_CreateTaskWithEvent(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	// Create a zone first
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	// Subscribe to events
	eventReceived := make(chan string, 1)
	service.SubscribeToEvents(func(msg string) {
		eventReceived <- msg
	})

	// Create task with event
	task, err := service.CreateTaskWithEvent(
		"zone1",
		"task1",
		"Test Task",
		"Test Description",
		5,
		map[string]string{"key": "value"},
	)

	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	if task == nil {
		t.Fatal("Expected task to be created")
	}

	if task.ID != "task1" {
		t.Errorf("Expected task ID 'task1', got %s", task.ID)
	}

	if task.Title != "Test Task" {
		t.Errorf("Expected title 'Test Task', got %s", task.Title)
	}

	// Verify event was published
	select {
	case msg := <-eventReceived:
		var event TaskEvent
		err := json.Unmarshal([]byte(msg), &event)
		if err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if event.Type != "task_created" {
			t.Errorf("Expected type 'task_created', got %s", event.Type)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestKanbanService_UpdateTaskStatusWithEvent(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	// Create a zone and task
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = board.AddTask("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Subscribe to events
	eventReceived := make(chan string, 10)
	service.SubscribeToEvents(func(msg string) {
		eventReceived <- msg
	})

	// Update task status
	err = service.UpdateTaskStatusWithEvent("zone1", "task1", TaskRunning, "", "")
	if err != nil {
		t.Fatalf("Failed to update task status: %v", err)
	}

	// Verify event was published
	select {
	case msg := <-eventReceived:
		var event TaskEvent
		err := json.Unmarshal([]byte(msg), &event)
		if err != nil {
			t.Fatalf("Failed to unmarshal event: %v", err)
		}

		if event.Type != "task_updated" {
			t.Errorf("Expected type 'task_updated', got %s", event.Type)
		}

		if event.Status != TaskRunning {
			t.Errorf("Expected status 'running', got %s", event.Status)
		}

	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for event")
	}
}

func TestKanbanService_GetBoard(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	retrievedBoard := service.GetBoard()
	if retrievedBoard != board {
		t.Error("Expected GetBoard to return the same board instance")
	}
}

func TestKanbanService_HTTPHandler(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	handler := service.HTTPHandler()
	if handler == nil {
		t.Fatal("Expected HTTP handler to be created")
	}

	// Test GET /kanban endpoint
	req := httptest.NewRequest(http.MethodGet, "/kanban", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["id"] != "test-board" {
		t.Errorf("Expected board id 'test-board', got %v", response["id"])
	}
}

func TestKanbanService_HTTPHandler_ZoneEndpoint(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	// Create a zone
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	handler := service.HTTPHandler()

	// Test GET /kanban/zones/{zoneID} endpoint
	req := httptest.NewRequest(http.MethodGet, "/kanban/zones/zone1", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err = json.NewDecoder(w.Body).Decode(&response)
	if err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response["id"] != "zone1" {
		t.Errorf("Expected zone id 'zone1', got %v", response["id"])
	}
}

func TestKanbanService_HTTPHandler_NonExistentZone(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	handler := service.HTTPHandler()

	// Test GET /kanban/zones/{zoneID} with non-existent zone
	req := httptest.NewRequest(http.MethodGet, "/kanban/zones/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestKanbanService_StartStatusReporter(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start status reporter
	service.StartStatusReporter(ctx)

	// Give it time to start
	time.Sleep(100 * time.Millisecond)

	// Create a zone and task to trigger status report
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = service.CreateTaskWithEvent("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// Wait for status report to be processed
	time.Sleep(200 * time.Millisecond)

	t.Log("Status reporter test completed")
}

func TestTaskEvent_MarshalUnmarshal(t *testing.T) {
	event := TaskEvent{
		Type:      "task_completed",
		BoardID:   "board1",
		ZoneID:    "zone1",
		TaskID:    "task1",
		Status:    TaskCompleted,
		Title:     "Test Task",
		Result:    "Success",
		Error:     "",
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Failed to marshal event: %v", err)
	}

	var unmarshaled Event
	err = json.Unmarshal(data, &unmarshaled)
	if err != nil {
		t.Fatalf("Failed to unmarshal event: %v", err)
	}

	// Note: We can't directly compare because unmarshaled is different type
	// This test ensures the JSON structure is valid
	t.Logf("Marshaled event: %s", string(data))
}

func TestKanbanService_ConcurrentEventPublishing(t *testing.T) {
	board := NewKanbanBoard("test-board")
	msgBus := bus.NewMessageBus()
	service := NewKanbanService(board, msgBus)

	eventsReceived := make(chan string, 100)
	service.SubscribeToEvents(func(msg string) {
		eventsReceived <- msg
	})

	done := make(chan bool)

	// Concurrent event publishing
	go func() {
		for i := 0; i < 50; i++ {
			service.PublishTaskEvent("task_created", "zone1", "task1", TaskPending, "Test Task", "", "")
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	go func() {
		for i := 0; i < 50; i++ {
			service.PublishTaskEvent("task_updated", "zone1", "task1", TaskRunning, "Test Task", "", "")
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	<-done
	<-done

	// Count received events
	count := 0
	timeout := time.After(2 * time.Second)
loop:
	for {
		select {
		case <-eventsReceived:
			count++
		case <-timeout:
			break loop
		}
	}

	t.Logf("Received %d events out of 100 published", count)
	if count < 90 {
		t.Errorf("Expected at least 90 events, got %d", count)
	}
}
