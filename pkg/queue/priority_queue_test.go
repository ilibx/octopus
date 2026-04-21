package queue

import (
	"testing"
	"time"
)

func TestPriorityQueueBasic(t *testing.T) {
	pq := NewPriorityQueue()

	if !pq.IsEmpty() {
		t.Error("Expected new queue to be empty")
	}

	if pq.Size() != 0 {
		t.Errorf("Expected size 0, got %d", pq.Size())
	}
}

func TestPriorityQueueEnqueueDequeue(t *testing.T) {
	pq := NewPriorityQueue()

	// Enqueue items with different priorities
	pq.Enqueue("low1", PriorityLow, "data1")
	pq.Enqueue("high1", PriorityHigh, "data2")
	pq.Enqueue("normal1", PriorityNormal, "data3")

	if pq.Size() != 3 {
		t.Errorf("Expected size 3, got %d", pq.Size())
	}

	// Dequeue should return high priority first
	item := pq.Dequeue()
	if item == nil {
		t.Fatal("Expected item, got nil")
	}
	if item.ID != "high1" {
		t.Errorf("Expected high1, got %s", item.ID)
	}
	if item.Priority != PriorityHigh {
		t.Errorf("Expected PriorityHigh, got %v", item.Priority)
	}

	// Then normal
	item = pq.Dequeue()
	if item.ID != "normal1" {
		t.Errorf("Expected normal1, got %s", item.ID)
	}

	// Then low
	item = pq.Dequeue()
	if item.ID != "low1" {
		t.Errorf("Expected low1, got %s", item.ID)
	}

	// Queue should be empty
	if !pq.IsEmpty() {
		t.Error("Expected queue to be empty")
	}
}

func TestPriorityQueueSamePriority(t *testing.T) {
	pq := NewPriorityQueue()

	// Enqueue items with same priority
	pq.Enqueue("first", PriorityNormal, "data1")
	time.Sleep(10 * time.Millisecond)
	pq.Enqueue("second", PriorityNormal, "data2")
	time.Sleep(10 * time.Millisecond)
	pq.Enqueue("third", PriorityNormal, "data3")

	// Should dequeue in FIFO order
	item := pq.Dequeue()
	if item.ID != "first" {
		t.Errorf("Expected first, got %s", item.ID)
	}

	item = pq.Dequeue()
	if item.ID != "second" {
		t.Errorf("Expected second, got %s", item.ID)
	}

	item = pq.Dequeue()
	if item.ID != "third" {
		t.Errorf("Expected third, got %s", item.ID)
	}
}

func TestPriorityQueuePeek(t *testing.T) {
	pq := NewPriorityQueue()

	// Peek on empty queue
	item := pq.Peek()
	if item != nil {
		t.Error("Expected nil on empty queue")
	}

	pq.Enqueue("item1", PriorityNormal, "data")

	// Peek should not remove item
	item = pq.Peek()
	if item == nil {
		t.Fatal("Expected item, got nil")
	}
	if item.ID != "item1" {
		t.Errorf("Expected item1, got %s", item.ID)
	}

	// Size should still be 1
	if pq.Size() != 1 {
		t.Errorf("Expected size 1, got %d", pq.Size())
	}
}

func TestPriorityQueueRemove(t *testing.T) {
	pq := NewPriorityQueue()

	pq.Enqueue("item1", PriorityNormal, "data1")
	pq.Enqueue("item2", PriorityHigh, "data2")
	pq.Enqueue("item3", PriorityLow, "data3")

	// Remove middle item
	removed := pq.Remove("item2")
	if !removed {
		t.Error("Expected removal to succeed")
	}

	if pq.Size() != 2 {
		t.Errorf("Expected size 2, got %d", pq.Size())
	}

	// Try to remove non-existent item
	removed = pq.Remove("nonexistent")
	if removed {
		t.Error("Expected removal to fail")
	}
}

func TestPriorityQueueClear(t *testing.T) {
	pq := NewPriorityQueue()

	pq.Enqueue("item1", PriorityNormal, "data1")
	pq.Enqueue("item2", PriorityHigh, "data2")

	pq.Clear()

	if !pq.IsEmpty() {
		t.Error("Expected queue to be empty after clear")
	}
	if pq.Size() != 0 {
		t.Errorf("Expected size 0, got %d", pq.Size())
	}
}

func TestPriorityQueueStats(t *testing.T) {
	pq := NewPriorityQueue()

	pq.Enqueue("h1", PriorityHigh, "data")
	pq.Enqueue("h2", PriorityHigh, "data")
	pq.Enqueue("n1", PriorityNormal, "data")
	pq.Enqueue("n2", PriorityNormal, "data")
	pq.Enqueue("n3", PriorityNormal, "data")
	pq.Enqueue("l1", PriorityLow, "data")

	stats := pq.Stats()

	if stats["total"] != 6 {
		t.Errorf("Expected total 6, got %v", stats["total"])
	}
	if stats["high_count"] != 2 {
		t.Errorf("Expected high_count 2, got %v", stats["high_count"])
	}
	if stats["normal_count"] != 3 {
		t.Errorf("Expected normal_count 3, got %v", stats["normal_count"])
	}
	if stats["low_count"] != 1 {
		t.Errorf("Expected low_count 1, got %v", stats["low_count"])
	}
}

func TestPriorityQueueList(t *testing.T) {
	pq := NewPriorityQueue()

	pq.Enqueue("item1", PriorityNormal, "data1")
	pq.Enqueue("item2", PriorityHigh, "data2")

	items := pq.List()
	if len(items) != 2 {
		t.Errorf("Expected 2 items, got %d", len(items))
	}
}

func TestPriorityQueueConcurrentAccess(t *testing.T) {
	pq := NewPriorityQueue()

	done := make(chan bool)

	// Concurrent enqueues
	go func() {
		for i := 0; i < 50; i++ {
			pq.Enqueue(string(rune(i)), PriorityNormal, i)
		}
		done <- true
	}()

	// Concurrent dequeues
	go func() {
		for i := 0; i < 50; i++ {
			pq.Dequeue()
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	// Should not panic
}

func TestPriorityItemData(t *testing.T) {
	pq := NewPriorityQueue()

	type TestData struct {
		Name  string
		Value int
	}

	data := TestData{Name: "test", Value: 42}
	pq.Enqueue("item1", PriorityHigh, data)

	item := pq.Dequeue()
	if item == nil {
		t.Fatal("Expected item, got nil")
	}

	retrieved, ok := item.Data.(TestData)
	if !ok {
		t.Fatal("Expected TestData type")
	}

	if retrieved.Name != "test" {
		t.Errorf("Expected name 'test', got %s", retrieved.Name)
	}
	if retrieved.Value != 42 {
		t.Errorf("Expected value 42, got %d", retrieved.Value)
	}
}
