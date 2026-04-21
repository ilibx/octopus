package kanban

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/agent"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/circuitbreaker"
	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
	"github.com/ilibx/octopus/pkg/observability"
	"github.com/ilibx/octopus/pkg/providers"
	"github.com/ilibx/octopus/pkg/queue"
	"github.com/ilibx/octopus/pkg/scanner"
)

// AgentOrchestrator manages the lifecycle of sub-agents based on kanban board state
// Main Agent responsibilities:
// - Monitor board for pending tasks and spawn sub-agents
// - Publish task events and status notifications
// - Manage sub-agent lifecycle (spawn/release)
// Sub-Agent responsibilities:
// - Execute tasks assigned to their zone
// - Report task execution status and results back to the board
type AgentOrchestrator struct {
	board            *KanbanBoard
	agentRegistry    *agent.AgentRegistry
	msgBus           *bus.MessageBus
	activeAgents     map[string]string // zoneID -> agentID
	mu               sync.RWMutex
	cfg              *config.Config
	provider         providers.LLMProvider
	mainAgentID      string // ID of the main agent that owns this orchestrator
	taskQueue        *queue.PriorityQueue
	circuitBreaker   *circuitbreaker.CircuitBreaker
	metricsCollector *observability.MetricsCollector
}

// NewAgentOrchestrator creates a new orchestrator for the main agent
func NewAgentOrchestrator(board *KanbanBoard, registry *agent.AgentRegistry, msgBus *bus.MessageBus, cfg *config.Config, provider providers.LLMProvider) *AgentOrchestrator {
	// Initialize priority queue
	taskQueue := queue.NewPriorityQueue()

	// Initialize circuit breaker for LLM provider
	cbConfig := cfg.CircuitBreaker
	circuitBreaker := circuitbreaker.NewCircuitBreaker(
		"llm_provider",
		cbConfig.FailureThreshold,
		cbConfig.RecoveryWindow,
	)

	// Initialize metrics collector
	metricsCollector := observability.NewMetricsCollector()

	return &AgentOrchestrator{
		board:            board,
		agentRegistry:    registry,
		msgBus:           msgBus,
		activeAgents:     make(map[string]string),
		cfg:              cfg,
		provider:         provider,
		mainAgentID:      board.MainAgentID,
		taskQueue:        taskQueue,
		circuitBreaker:   circuitBreaker,
		metricsCollector: metricsCollector,
	}
}

// GetMainAgentID returns the ID of the main agent
func (o *AgentOrchestrator) GetMainAgentID() string {
	return o.mainAgentID
}

// MonitorBoard continuously monitors the kanban board for pending tasks
// and spawns agents as needed with dynamic polling interval
func (o *AgentOrchestrator) MonitorBoard(ctx context.Context) {
	baseInterval := 2 * time.Second
	maxInterval := 10 * time.Second
	currentInterval := baseInterval

	for {
		select {
		case <-ctx.Done():
			logger.InfoCF("orchestrator", "Stopping board monitor", nil)
			return
		case <-time.After(currentInterval):
			hasWork := o.checkAndSpawnAgents()
			if !hasWork {
				// No work found, increase interval exponentially
				currentInterval = currentInterval * 2
				if currentInterval > maxInterval {
					currentInterval = maxInterval
				}
			} else {
				// Work found, reset to base interval
				currentInterval = baseInterval
			}
		}
	}
}

// checkAndSpawnAgents checks all zones for pending tasks and spawns agents if needed
// Returns true if there is work to do, false otherwise
func (o *AgentOrchestrator) checkAndSpawnAgents() bool {
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
		return false
	}

	hasWork := false
	for zoneID, tasks := range pendingTasks {
		if len(tasks) == 0 {
			continue
		}

		hasWork = true

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

	return hasWork
}

// spawnAgentForZone creates and starts a new sub-agent instance for a specific zone
// This method is called by the Main Agent to delegate task execution to Sub-Agents
// The sub-agent's workspace is set to the directory containing its AGENT.md file,
// which is then loaded as the system prompt by the agent's ContextBuilder.
func (o *AgentOrchestrator) spawnAgentForZone(zoneID, agentType string) error {
	// Generate a unique agent ID for this sub-agent
	agentID := fmt.Sprintf("%s_%s", agentType, zoneID)

	// Check if agent already exists in registry
	if _, exists := o.agentRegistry.GetAgent(agentID); exists {
		logger.InfoCF("orchestrator", "Sub-agent already exists for zone",
			map[string]any{"zone_id": zoneID, "agent_id": agentID})
		o.activeAgents[zoneID] = agentID
		return nil
	}

	// Find the agent directory based on agent_type
	// The agent_type should match the directory name in the agents folder
	agentDir, err := o.findAgentDirectory(agentType)
	if err != nil {
		logger.WarnCF("orchestrator", "Failed to find agent directory, using default workspace",
			map[string]any{"agent_type": agentType, "error": err.Error()})
		// Fall back to default behavior without specific agent directory
	}

	// Create sub-agent configuration dynamically
	// Note: Default is set to false because this is a sub-agent, not the main agent
	agentCfg := &config.AgentConfig{
		ID:      agentID,
		Name:    fmt.Sprintf("Sub-agent for zone %s", zoneID),
		Default: false, // Explicitly mark as sub-agent
	}

	// Set workspace to agent directory if found, so ContextBuilder loads AGENT.md from there
	if agentDir != "" {
		agentCfg.Workspace = agentDir
		logger.InfoCF("orchestrator", "Using agent directory as workspace",
			map[string]any{"agent_type": agentType, "workspace": agentDir})
	}

	// Add the sub-agent to the registry
	addedID, err := o.agentRegistry.AddAgent(agentCfg, &o.cfg.Agents.Defaults, o.cfg, o.provider)
	if err != nil {
		return fmt.Errorf("failed to add sub-agent: %w", err)
	}

	logger.InfoCF("orchestrator", "Main agent spawning sub-agent for zone",
		map[string]any{
			"main_agent_id": o.mainAgentID,
			"zone_id":       zoneID,
			"sub_agent_id":  addedID,
			"agent_type":    agentType,
			"workspace":     agentDir,
		})

	// Mark the zone as having an active sub-agent
	o.activeAgents[zoneID] = addedID

	return nil
}

// findAgentDirectory searches for the directory containing the agent's AGENT.md file
// It searches in the configured agents directory and supports multiple naming conventions
func (o *AgentOrchestrator) findAgentDirectory(agentType string) (string, error) {
	agentsDir := o.cfg.Agents.AgentsDir
	if agentsDir == "" {
		return "", fmt.Errorf("agents_dir not configured")
	}

	// Normalize agent type for matching
	normalizedType := strings.ToLower(strings.ReplaceAll(agentType, "-", "_"))

	// Search strategies:
	// 1. Direct match: agentsDir/{agentType}/AGENT.md
	// 2. Underscore variant: agentsDir/{agent_type}/AGENT.md
	// 3. Hyphen variant: agentsDir/{agent-type}/AGENT.md

	searchPaths := []string{
		filepath.Join(agentsDir, agentType),
		filepath.Join(agentsDir, normalizedType),
		filepath.Join(agentsDir, strings.ReplaceAll(normalizedType, "_", "-")),
	}

	for _, dirPath := range searchPaths {
		// Check for AGENT.md first (new standard)
		agentMdPath := filepath.Join(dirPath, "AGENT.md")
		if _, err := os.Stat(agentMdPath); err == nil {
			return dirPath, nil
		}

		// Fallback to main.md (legacy)
		mainMdPath := filepath.Join(dirPath, "main.md")
		if _, err := os.Stat(mainMdPath); err == nil {
			return dirPath, nil
		}
	}

	return "", fmt.Errorf("agent directory not found for type: %s", agentType)
}

// OnTaskCompleted handles task completion events from sub-agents
// This is called when a sub-agent reports that a task has been completed
func (o *AgentOrchestrator) OnTaskCompleted(zoneID, taskID string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	logger.InfoCF("orchestrator", "Main agent received task completion event from sub-agent",
		map[string]any{
			"main_agent_id": o.mainAgentID,
			"zone_id":       zoneID,
			"task_id":       taskID,
		})

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
		// Remove the sub-agent from active agents and release it
		if agentID, exists := o.activeAgents[zoneID]; exists {
			logger.InfoCF("orchestrator", "All tasks completed in zone, main agent releasing sub-agent",
				map[string]any{
					"main_agent_id": o.mainAgentID,
					"zone_id":       zoneID,
					"sub_agent_id":  agentID,
				})

			// Remove from active agents map
			delete(o.activeAgents, zoneID)

			// Actually remove the sub-agent from the registry (but never the main agent)
			if err := o.agentRegistry.RemoveAgent(agentID); err != nil {
				logger.WarnCF("orchestrator", "Failed to remove sub-agent from registry",
					map[string]any{"zone_id": zoneID, "sub_agent_id": agentID, "error": err.Error()})
			} else {
				logger.InfoCF("orchestrator", "Sub-agent successfully released from registry by main agent",
					map[string]any{"zone_id": zoneID, "sub_agent_id": agentID})
			}
		}
	}
}

// ReleaseAllAgents releases all sub-agents when the kanban board is empty
// Called by the Main Agent to clean up all delegated sub-agents
func (o *AgentOrchestrator) ReleaseAllAgents() {
	o.mu.Lock()
	defer o.mu.Unlock()

	logger.InfoCF("orchestrator", "Main agent releasing all sub-agents due to empty kanban board",
		map[string]any{
			"main_agent_id":       o.mainAgentID,
			"active_agents_count": len(o.activeAgents),
		})

	for zoneID, agentID := range o.activeAgents {
		// Remove the sub-agent from the registry (but never the main agent)
		if err := o.agentRegistry.RemoveAgent(agentID); err != nil {
			logger.WarnCF("orchestrator", "Failed to remove sub-agent from registry",
				map[string]any{
					"main_agent_id": o.mainAgentID,
					"zone_id":       zoneID,
					"sub_agent_id":  agentID,
					"error":         err.Error(),
				})
		} else {
			logger.InfoCF("orchestrator", "Sub-agent successfully released by main agent",
				map[string]any{
					"main_agent_id": o.mainAgentID,
					"zone_id":       zoneID,
					"sub_agent_id":  agentID,
				})
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
