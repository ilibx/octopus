package kanban

import (
	"testing"
	"time"
)

// TestMetrics is a mock metrics implementation for testing
type TestMetrics struct {
	TasksCreatedCount   int
	TasksCompletedCount int
	TasksFailedCount    int
	AgentsSpawnedCount  int
	AgentsReleasedCount int
	TaskLatencies       []time.Duration
	OrchestratorLoops   []time.Duration
	MutexWaitTimes      []time.Duration
	ZoneTaskCounts      map[string]map[string]int
	AgentConcurrency    int
}

func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		ZoneTaskCounts: make(map[string]map[string]int),
	}
}

func (m *TestMetrics) RecordTaskCreated() {
	m.TasksCreatedCount++
}

func (m *TestMetrics) RecordTaskCompleted(latency time.Duration) {
	m.TasksCompletedCount++
	m.TaskLatencies = append(m.TaskLatencies, latency)
}

func (m *TestMetrics) RecordTaskFailed() {
	m.TasksFailedCount++
}

func (m *TestMetrics) RecordAgentSpawned() {
	m.AgentsSpawnedCount++
}

func (m *TestMetrics) RecordAgentReleased() {
	m.AgentsReleasedCount++
}

func (m *TestMetrics) RecordOrchestratorLoop(duration time.Duration) {
	m.OrchestratorLoops = append(m.OrchestratorLoops, duration)
}

func (m *TestMetrics) RecordMutexWaitTime(duration time.Duration) {
	m.MutexWaitTimes = append(m.MutexWaitTimes, duration)
}

func (m *TestMetrics) UpdateZoneTaskCount(zoneID string, status TaskStatus, count int) {
	if _, exists := m.ZoneTaskCounts[zoneID]; !exists {
		m.ZoneTaskCounts[zoneID] = make(map[string]int)
	}
	m.ZoneTaskCounts[zoneID][string(status)] = count
}

func (m *TestMetrics) UpdateAgentConcurrency(count int) {
	m.AgentConcurrency = count
}

func (m *TestMetrics) ClearZoneMetrics(zoneID string) {
	delete(m.ZoneTaskCounts, zoneID)
}

// Test that metrics can be recorded
func TestMetricsRecording(t *testing.T) {
	metrics := NewTestMetrics()

	// Record various metrics
	metrics.RecordTaskCreated()
	metrics.RecordTaskCreated()
	metrics.RecordTaskCompleted(100 * time.Millisecond)
	metrics.RecordTaskFailed()
	metrics.RecordAgentSpawned()
	metrics.RecordAgentReleased()
	metrics.RecordOrchestratorLoop(50 * time.Millisecond)
	metrics.RecordMutexWaitTime(5 * time.Millisecond)
	metrics.UpdateZoneTaskCount("zone1", TaskPending, 5)
	metrics.UpdateAgentConcurrency(3)

	// Verify counts
	if metrics.TasksCreatedCount != 2 {
		t.Errorf("Expected 2 tasks created, got %d", metrics.TasksCreatedCount)
	}
	if metrics.TasksCompletedCount != 1 {
		t.Errorf("Expected 1 task completed, got %d", metrics.TasksCompletedCount)
	}
	if metrics.TasksFailedCount != 1 {
		t.Errorf("Expected 1 task failed, got %d", metrics.TasksFailedCount)
	}
	if metrics.AgentsSpawnedCount != 1 {
		t.Errorf("Expected 1 agent spawned, got %d", metrics.AgentsSpawnedCount)
	}
	if metrics.AgentsReleasedCount != 1 {
		t.Errorf("Expected 1 agent released, got %d", metrics.AgentsReleasedCount)
	}
	if len(metrics.TaskLatencies) != 1 {
		t.Errorf("Expected 1 latency recorded, got %d", len(metrics.TaskLatencies))
	}
	if metrics.TaskLatencies[0] != 100*time.Millisecond {
		t.Errorf("Expected latency 100ms, got %v", metrics.TaskLatencies[0])
	}
	if len(metrics.OrchestratorLoops) != 1 {
		t.Errorf("Expected 1 loop recorded, got %d", len(metrics.OrchestratorLoops))
	}
	if metrics.OrchestratorLoops[0] != 50*time.Millisecond {
		t.Errorf("Expected loop duration 50ms, got %v", metrics.OrchestratorLoops[0])
	}
	if len(metrics.MutexWaitTimes) != 1 {
		t.Errorf("Expected 1 mutex wait recorded, got %d", len(metrics.MutexWaitTimes))
	}
	if metrics.MutexWaitTimes[0] != 5*time.Millisecond {
		t.Errorf("Expected mutex wait 5ms, got %v", metrics.MutexWaitTimes[0])
	}
	if metrics.AgentConcurrency != 3 {
		t.Errorf("Expected concurrency 3, got %d", metrics.AgentConcurrency)
	}

	// Verify zone task counts
	if zoneCounts, exists := metrics.ZoneTaskCounts["zone1"]; exists {
		if count, exists := zoneCounts[string(TaskPending)]; !exists || count != 5 {
			t.Errorf("Expected zone1 pending count to be 5, got %d", count)
		}
	} else {
		t.Error("Expected zone1 to exist in zone task counts")
	}
}

func TestMetrics_ClearZoneMetrics(t *testing.T) {
	metrics := NewTestMetrics()

	// Add some zone metrics
	metrics.UpdateZoneTaskCount("zone1", TaskPending, 5)
	metrics.UpdateZoneTaskCount("zone1", TaskRunning, 2)
	metrics.UpdateZoneTaskCount("zone2", TaskPending, 3)

	// Clear zone1 metrics
	metrics.ClearZoneMetrics("zone1")

	// Verify zone1 is cleared
	if _, exists := metrics.ZoneTaskCounts["zone1"]; exists {
		t.Error("Expected zone1 to be cleared")
	}

	// Verify zone2 still exists
	if zoneCounts, exists := metrics.ZoneTaskCounts["zone2"]; !exists {
		t.Error("Expected zone2 to still exist")
	} else if count, exists := zoneCounts[string(TaskPending)]; !exists || count != 3 {
		t.Errorf("Expected zone2 pending count to be 3, got %d", count)
	}
}

func TestMetrics_MultipleZones(t *testing.T) {
	metrics := NewTestMetrics()

	// Update multiple zones
	zones := []string{"zone1", "zone2", "zone3"}
	statuses := []TaskStatus{TaskPending, TaskRunning, TaskCompleted}

	for i, zone := range zones {
		for j, status := range statuses {
			count := (i + 1) * (j + 1)
			metrics.UpdateZoneTaskCount(zone, status, count)
		}
	}

	// Verify all zones and statuses
	for i, zone := range zones {
		zoneCounts, exists := metrics.ZoneTaskCounts[zone]
		if !exists {
			t.Errorf("Expected zone %s to exist", zone)
			continue
		}
		for j, status := range statuses {
			expected := (i + 1) * (j + 1)
			if count, exists := zoneCounts[string(status)]; !exists || count != expected {
				t.Errorf("Expected %s %s count to be %d, got %d", zone, status, expected, count)
			}
		}
	}
}

func TestMetrics_ConcurrentUpdates(t *testing.T) {
	metrics := NewTestMetrics()
	done := make(chan bool)

	// Start multiple goroutines updating metrics concurrently
	for i := 0; i < 10; i++ {
		go func(zoneNum int) {
			zoneID := "zone" + string(rune('0'+zoneNum))
			for j := 0; j < 100; j++ {
				metrics.UpdateZoneTaskCount(zoneID, TaskPending, j)
				metrics.RecordTaskCreated()
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines to complete
	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify total tasks created
	expectedTasks := 10 * 100
	if metrics.TasksCreatedCount != expectedTasks {
		t.Errorf("Expected %d tasks created, got %d", expectedTasks, metrics.TasksCreatedCount)
	}

	// Verify all zones have been updated
	for i := 0; i < 10; i++ {
		zoneID := "zone" + string(rune('0'+i))
		if zoneCounts, exists := metrics.ZoneTaskCounts[zoneID]; !exists {
			t.Errorf("Expected zone %s to exist", zoneID)
		} else if _, exists := zoneCounts[string(TaskPending)]; !exists {
			t.Errorf("Expected zone %s to have pending count", zoneID)
		}
	}
}

func TestMetrics_AgentConcurrency(t *testing.T) {
	metrics := NewTestMetrics()

	// Simulate agent concurrency changes
	metrics.UpdateAgentConcurrency(5)
	if metrics.AgentConcurrency != 5 {
		t.Errorf("Expected concurrency 5, got %d", metrics.AgentConcurrency)
	}

	metrics.UpdateAgentConcurrency(10)
	if metrics.AgentConcurrency != 10 {
		t.Errorf("Expected concurrency 10, got %d", metrics.AgentConcurrency)
	}

	metrics.UpdateAgentConcurrency(0)
	if metrics.AgentConcurrency != 0 {
		t.Errorf("Expected concurrency 0, got %d", metrics.AgentConcurrency)
	}
}
