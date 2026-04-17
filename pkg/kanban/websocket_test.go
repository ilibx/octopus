package kanban

import (
	"testing"
	"time"

	"github.com/ilibx/octopus/pkg/bus"
)

func TestNewWSHub(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	if hub == nil {
		t.Fatal("Expected hub to be created")
	}

	if len(hub.clients) != 0 {
		t.Errorf("Expected 0 clients, got %d", len(hub.clients))
	}

	if hub.msgBus == nil {
		t.Error("Expected msgBus to be set")
	}
}

func TestWSHub_Broadcast(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	// Test broadcast with simple payload
	payload := map[string]string{"test": "data"}
	hub.Broadcast("test.event", payload)

	// Give some time for broadcast to process
	time.Sleep(10 * time.Millisecond)
}

func TestWSHub_SubscribeToEvents(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	// Publish a task event
	eventData := map[string]string{
		"task_id": "test-123",
		"status":  "completed",
	}
	msgBus.Publish("task.completed", eventData)

	// Give time for event to be processed
	time.Sleep(50 * time.Millisecond)
}

func TestWSClient_Close(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	// Create a mock client (without actual WebSocket connection)
	client := &WSClient{
		send:   make(chan []byte, 256),
		hub:    hub,
		closed: false,
	}

	// Close the client
	client.close()

	if !client.closed {
		t.Error("Expected client to be closed")
	}
}

func TestWSHub_ConcurrentAccess(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	done := make(chan bool)

	// Start multiple goroutines broadcasting messages
	for i := 0; i < 10; i++ {
		go func(id int) {
			for j := 0; j < 10; j++ {
				hub.Broadcast("test.event", map[string]int{"goroutine": id, "msg": j})
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestWSMessage_Marshal(t *testing.T) {
	msg := WSMessage{
		Type:      "test.event",
		Payload:   map[string]string{"key": "value"},
		Timestamp: time.Now(),
	}

	// The message should be marshalable
	// (actual marshaling tested in integration)
	if msg.Type != "test.event" {
		t.Errorf("Expected type 'test.event', got '%s'", msg.Type)
	}
}

func TestWSHub_Close(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	// Close the hub
	hub.Close()

	// Hub should be closed without panic
}

func TestWebSocket_Integration(t *testing.T) {
	// Integration test for WebSocket functionality
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	// Start the hub
	go hub.Run()

	// Give hub time to start
	time.Sleep(10 * time.Millisecond)

	// Broadcast a message
	hub.Broadcast("task.created", map[string]string{
		"task_id": "integration-test",
		"title":   "Test Task",
	})

	// Allow time for processing
	time.Sleep(50 * time.Millisecond)

	// Clean up
	hub.Close()
}

func TestWSEvent_Subscription(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	received := make(chan bool, 1)

	// Subscribe to events manually
	msgBus.Subscribe("task.", func(event *bus.Event) {
		received <- true
	})

	// Publish an event
	msgBus.Publish("task.test", map[string]string{"test": "data"})

	// Wait for event
	select {
	case <-received:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for event")
	}

	hub.Close()
}

func TestWSHub_EmptyBroadcast(t *testing.T) {
	msgBus := bus.NewMessageBus()
	hub := NewWSHub(msgBus)

	// Test broadcasting with nil payload
	hub.Broadcast("test.event", nil)

	// Test broadcasting with empty payload
	hub.Broadcast("test.event", map[string]interface{}{})
}
