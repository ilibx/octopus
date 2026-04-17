package kanban

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/agent"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/logger"
)

// AgentOrchestrator manages the lifecycle of sub-agents based on kanban board state
type AgentOrchestrator struct {
	board         *KanbanBoard
	agentRegistry *agent.AgentRegistry
	msgBus        *bus.MessageBus
	activeAgents  map[string]string // zoneID -> agentID
	mu            sync.RWMutex
}

// NewAgentOrchestrator creates a new orchestrator
func NewAgentOrchestrator(board *KanbanBoard, registry *agent.AgentRegistry, msgBus *bus.MessageBus) *AgentOrchestrator {
	return &AgentOrchestrator{
		board:         board,
		agentRegistry: registry,
		msgBus:        msgBus,
		activeAgents:  make(map[string]string),
	}
}

// MonitorBoard continuously monitors the kanban board for pending tasks
// and spawns agents as needed
func (o *AgentOrchestrator) MonitorBoard(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.InfoCF("orchestrator", "Stopping board monitor", nil)
			return
		case <-ticker.C:
			o.checkAndSpawnAgents()
		}
	}
}

// checkAndSpawnAgents checks all zones for pending tasks and spawns agents if needed
func (o *AgentOrchestrator) checkAndSpawnAgents() {
	o.mu.Lock()
	defer o.mu.Unlock()

	pendingTasks := o.board.GetPendingTasks()
	for zoneID, tasks := range pendingTasks {
		if len(tasks) == 0 {
			continue
		}

		// Check if zone already has an active agent
		if _, exists := o.activeAgents[zoneID]; exists {
			continue
		}

		// Check if zone has any running tasks
		if o.board.HasActiveAgent(zoneID) {
			continue
		}

		// Get the required agent type for this zone
		agentType, err := o.board.GetZoneAgentType(zoneID)
		if err != nil {
			logger.ErrorCF("orchestrator", "Failed to get agent type for zone",
				map[string]any{"zone_id": zoneID, "error": err.Error()})
			continue
		}

		// Spawn a new agent for this zone
		if err := o.spawnAgentForZone(zoneID, agentType); err != nil {
			logger.ErrorCF("orchestrator", "Failed to spawn agent for zone",
				map[string]any{"zone_id": zoneID, "agent_type": agentType, "error": err.Error()})
		}
	}
}

// spawnAgentForZone creates and starts a new agent instance for a specific zone
func (o *AgentOrchestrator) spawnAgentForZone(zoneID, agentType string) error {
	// Generate a unique agent ID for this zone
	agentID := fmt.Sprintf("%s_%s", agentType, zoneID)

	// Check if agent already exists in registry
	if _, exists := o.agentRegistry.GetAgent(agentID); exists {
		logger.InfoCF("orchestrator", "Agent already exists for zone",
			map[string]any{"zone_id": zoneID, "agent_id": agentID})
		o.activeAgents[zoneID] = agentID
		return nil
	}

	// TODO: Implement dynamic agent spawning logic
	// This would involve:
	// 1. Loading agent.md configuration from a template or file
	// 2. Creating a new AgentInstance with the appropriate configuration
	// 3. Registering it with the agent registry
	// 4. Starting the agent loop

	logger.InfoCF("orchestrator", "Spawning new agent for zone",
		map[string]any{
			"zone_id":    zoneID,
			"agent_id":   agentID,
			"agent_type": agentType,
		})

	// For now, just mark the zone as having an active agent
	// In a full implementation, we would actually create and start the agent
	o.activeAgents[zoneID] = agentID

	return nil
}

// OnTaskCompleted handles task completion events
func (o *AgentOrchestrator) OnTaskCompleted(zoneID, taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	// Check if all tasks in the zone are completed
	zone, err := o.board.GetZone(zoneID)
	if err != nil {
		logger.ErrorCF("orchestrator", "Failed to get zone",
			map[string]any{"zone_id": zoneID, "error": err.Error()})
		return
	}

	allCompleted := true
	for _, task := range zone.Tasks {
		if task.Status != TaskCompleted && task.Status != TaskFailed {
			allCompleted = false
			break
		}
	}

	if allCompleted {
		// Remove the agent from active agents
		if agentID, exists := o.activeAgents[zoneID]; exists {
			logger.InfoCF("orchestrator", "All tasks completed in zone, releasing agent",
				map[string]any{"zone_id": zoneID, "agent_id": agentID})
			delete(o.activeAgents, zoneID)
			// TODO: Actually stop/shutdown the agent instance
		}
	}
}

// GetActiveAgents returns a map of zone IDs to active agent IDs
func (o *AgentOrchestrator) GetActiveAgents() map[string]string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	result := make(map[string]string)
	for k, v := range o.activeAgents {
		result[k] = v
	}
	return result
}
