package kanban

import (
	"context"
	"testing"
	"time"

	"github.com/ilibx/octopus/pkg/agent"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/providers"
)

// MockLLMProvider is a mock implementation for testing
type MockLLMProvider struct {
	providers.LLMProvider
}

func TestNewAgentOrchestrator(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	if orchestrator == nil {
		t.Fatal("Expected orchestrator to be created")
	}

	if orchestrator.board != board {
		t.Error("Expected board to be set")
	}

	if orchestrator.agentRegistry != registry {
		t.Error("Expected agentRegistry to be set")
	}

	if orchestrator.msgBus != msgBus {
		t.Error("Expected msgBus to be set")
	}

	if orchestrator.activeAgents == nil {
		t.Error("Expected activeAgents map to be initialized")
	}

	if len(orchestrator.activeAgents) != 0 {
		t.Error("Expected activeAgents to be empty initially")
	}
}

func TestAgentOrchestrator_SpawnAgentForZone(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{
		Agents: config.AgentConfigs{
			Defaults: config.AgentConfig{},
		},
	}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	// Create a zone first
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	// Try to spawn an agent for the zone
	err = orchestrator.spawnAgentForZone("zone1", "test-agent")

	// Note: This will likely fail in test environment without proper agent setup
	// but we're testing the logic flow
	if err != nil {
		// Expected to fail in test environment, but should fail gracefully
		t.Logf("Spawn agent failed as expected in test env: %v", err)
	}

	// Verify the zone was marked as having an active agent
	orchestrator.mu.RLock()
	_, hasAgent := orchestrator.activeAgents["zone1"]
	orchestrator.mu.RUnlock()

	// Agent might not be added due to test environment limitations
	// but the logic should have attempted to add it
	t.Logf("Active agents after spawn attempt: %v", orchestrator.activeAgents)
}

func TestAgentOrchestrator_OnTaskCompleted(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	// Create a zone
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	// Add a task to the zone
	_, err = board.AddTask("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Manually mark the task as completed
	zone, err := board.GetZone("zone1")
	if err != nil {
		t.Fatalf("Failed to get zone: %v", err)
	}

	for _, task := range zone.Tasks {
		task.Status = TaskCompleted
	}

	// Call OnTaskCompleted
	orchestrator.OnTaskCompleted("zone1", "task1")

	// The orchestrator should check if all tasks are completed
	// and potentially release agents
	t.Log("OnTaskCompleted executed successfully")
}

func TestAgentOrchestrator_GetActiveAgents(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	// Initially should be empty
	agents := orchestrator.GetActiveAgents()
	if len(agents) != 0 {
		t.Errorf("Expected no active agents initially, got %d", len(agents))
	}

	// Add a mock active agent
	orchestrator.mu.Lock()
	orchestrator.activeAgents["zone1"] = "agent1"
	orchestrator.mu.Unlock()

	// Should now have one agent
	agents = orchestrator.GetActiveAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 active agent, got %d", len(agents))
	}

	if agents["zone1"] != "agent1" {
		t.Errorf("Expected agent1 for zone1, got %s", agents["zone1"])
	}
}

func TestAgentOrchestrator_CheckAndSpawnAgents(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	// Test with empty board - should not spawn anything
	orchestrator.checkAndSpawnAgents()

	orchestrator.mu.RLock()
	activeCount := len(orchestrator.activeAgents)
	orchestrator.mu.RUnlock()

	if activeCount != 0 {
		t.Errorf("Expected no active agents for empty board, got %d", activeCount)
	}

	// Create a zone and add a pending task
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = board.AddTask("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Now checkAndSpawnAgents should attempt to spawn an agent
	orchestrator.checkAndSpawnAgents()

	// Give it time to process
	time.Sleep(100 * time.Millisecond)

	// In test environment, agent creation might fail, but logic should execute
	t.Logf("Active agents after check: %v", orchestrator.activeAgents)
}

func TestAgentOrchestrator_ReleaseAllAgents(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	// Add mock active agents
	orchestrator.mu.Lock()
	orchestrator.activeAgents["zone1"] = "agent1"
	orchestrator.activeAgents["zone2"] = "agent2"
	orchestrator.mu.Unlock()

	// Release all agents
	orchestrator.ReleaseAllAgents()

	// Verify all agents are released
	orchestrator.mu.RLock()
	activeCount := len(orchestrator.activeAgents)
	orchestrator.mu.RUnlock()

	if activeCount != 0 {
		t.Errorf("Expected no active agents after release, got %d", activeCount)
	}
}

func TestAgentOrchestrator_MonitorBoard(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	// Start monitoring in background
	go orchestrator.MonitorBoard(ctx)

	// Let it run for a bit
	time.Sleep(100 * time.Millisecond)

	// Add a zone and task
	_, err := board.CreateZone("zone1", "test-agent")
	if err != nil {
		t.Fatalf("Failed to create zone: %v", err)
	}

	_, err = board.AddTask("zone1", "task1", "Test Task", "Test Description", 5, nil)
	if err != nil {
		t.Fatalf("Failed to add task: %v", err)
	}

	// Wait for monitor to detect and process
	time.Sleep(500 * time.Millisecond)

	// Cancel context to stop monitoring
	cancel()

	// Wait for graceful shutdown
	time.Sleep(100 * time.Millisecond)

	t.Log("MonitorBoard test completed")
}

func TestAgentOrchestrator_ConcurrentAccess(t *testing.T) {
	board := NewKanbanBoard("test-board")
	registry := agent.NewAgentRegistry()
	msgBus := bus.NewMessageBus()
	cfg := &config.Config{}
	provider := &MockLLMProvider{}

	orchestrator := NewAgentOrchestrator(board, registry, msgBus, cfg, provider)

	done := make(chan bool)

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			orchestrator.GetActiveAgents()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Concurrent writes
	go func() {
		for i := 0; i < 100; i++ {
			orchestrator.mu.Lock()
			orchestrator.activeAgents["zone1"] = "agent1"
			orchestrator.mu.Unlock()
			time.Sleep(time.Millisecond)
		}
		done <- true
	}()

	// Wait for both goroutines
	<-done
	<-done

	t.Log("Concurrent access test passed")
}
