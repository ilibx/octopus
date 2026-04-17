package kanban

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/agent"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
	"github.com/ilibx/octopus/pkg/providers"
)

// AgentOrchestrator manages the lifecycle of sub-agents based on kanban board state
type AgentOrchestrator struct {
	board         *KanbanBoard
	agentRegistry *agent.AgentRegistry
	msgBus        *bus.MessageBus
	activeAgents  map[string]string // zoneID -> agentID
	mu            sync.RWMutex
	cfg           *config.Config
	provider      providers.LLMProvider
}

// NewAgentOrchestrator creates a new orchestrator
func NewAgentOrchestrator(board *KanbanBoard, registry *agent.AgentRegistry, msgBus *bus.MessageBus, cfg *config.Config, provider providers.LLMProvider) *AgentOrchestrator {
	return &AgentOrchestrator{
		board:         board,
		agentRegistry: registry,
		msgBus:        msgBus,
		activeAgents:  make(map[string]string),
		cfg:           cfg,
		provider:      provider,
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

	// If no pending tasks at all, release all sub-agents
	if len(pendingTasks) == 0 {
		// Release all active sub-agents when board is empty
		for zoneID, agentID := range o.activeAgents {
			logger.InfoCF("orchestrator", "Kanban board empty, releasing sub-agent",
				map[string]any{"zone_id": zoneID, "agent_id": agentID})

			// Remove the agent from the registry (but never the main agent)
			if err := o.agentRegistry.RemoveAgent(agentID); err != nil {
				logger.WarnCF("orchestrator", "Failed to remove agent from registry",
					map[string]any{"zone_id": zoneID, "agent_id": agentID, "error": err.Error()})
			} else {
				logger.InfoCF("orchestrator", "Agent successfully released due to empty board",
					map[string]any{"zone_id": zoneID, "agent_id": agentID})
			}
			delete(o.activeAgents, zoneID)
		}
		return
	}

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

	// Create agent configuration dynamically
	agentCfg := &config.AgentConfig{
		ID:      agentID,
		Name:    fmt.Sprintf("Subagent for zone %s", zoneID),
		Default: false,
	}

	// Add the agent to the registry
	addedID, err := o.agentRegistry.AddAgent(agentCfg, &o.cfg.Agents.Defaults, o.cfg, o.provider)
	if err != nil {
		return fmt.Errorf("failed to add agent: %w", err)
	}

	logger.InfoCF("orchestrator", "Spawning new agent for zone",
		map[string]any{
			"zone_id":    zoneID,
			"agent_id":   addedID,
			"agent_type": agentType,
		})

	// Mark the zone as having an active agent
	o.activeAgents[zoneID] = addedID

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
		// Remove the agent from active agents and release it
		if agentID, exists := o.activeAgents[zoneID]; exists {
			logger.InfoCF("orchestrator", "All tasks completed in zone, releasing agent",
				map[string]any{"zone_id": zoneID, "agent_id": agentID})

			// Remove from active agents map
			delete(o.activeAgents, zoneID)

			// Actually remove the agent from the registry (but never the main agent)
			if err := o.agentRegistry.RemoveAgent(agentID); err != nil {
				logger.WarnCF("orchestrator", "Failed to remove agent from registry",
					map[string]any{"zone_id": zoneID, "agent_id": agentID, "error": err.Error()})
			} else {
				logger.InfoCF("orchestrator", "Agent successfully released from registry",
					map[string]any{"zone_id": zoneID, "agent_id": agentID})
			}
		}
	}
}

// ReleaseAllAgents releases all sub-agents when the kanban board is empty
func (o *AgentOrchestrator) ReleaseAllAgents() {
	o.mu.Lock()
	defer o.mu.Unlock()

	logger.InfoCF("orchestrator", "Releasing all sub-agents due to empty kanban board",
		map[string]any{"active_agents_count": len(o.activeAgents)})

	for zoneID, agentID := range o.activeAgents {
		// Remove the agent from the registry (but never the main agent)
		if err := o.agentRegistry.RemoveAgent(agentID); err != nil {
			logger.WarnCF("orchestrator", "Failed to remove agent from registry",
				map[string]any{"zone_id": zoneID, "agent_id": agentID, "error": err.Error()})
		} else {
			logger.InfoCF("orchestrator", "Agent successfully released",
				map[string]any{"zone_id": zoneID, "agent_id": agentID})
		}
		delete(o.activeAgents, zoneID)
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
