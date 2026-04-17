package agent

import (
	"fmt"
	"sync"

	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
	"github.com/ilibx/octopus/pkg/providers"
	"github.com/ilibx/octopus/pkg/routing"
	"github.com/ilibx/octopus/pkg/scanner"
	"github.com/ilibx/octopus/pkg/tools"
)

// AgentRegistry manages multiple agent instances and routes messages to them.
type AgentRegistry struct {
	agents   map[string]*AgentInstance
	resolver *routing.RouteResolver
	mu       sync.RWMutex
}

// NewAgentRegistry creates a registry from config, instantiating all agents.
func NewAgentRegistry(
	cfg *config.Config,
	provider providers.LLMProvider,
) *AgentRegistry {
	registry := &AgentRegistry{
		agents:   make(map[string]*AgentInstance),
		resolver: routing.NewRouteResolver(cfg),
	}

	// Scan agents directory for auto-discovered agents
	var scannedAgents []scanner.AgentMetadata
	if cfg.Agents.AgentsDir != "" {
		sc := scanner.NewAgentScanner(cfg.Agents.AgentsDir)
		var err error
		scannedAgents, err = sc.ScanAgents()
		if err != nil {
			logger.WarnCF("agent", "Failed to scan agents directory",
				map[string]any{"path": cfg.Agents.AgentsDir, "error": err.Error()})
		}
	}

	// Build final agent configs from scanned agents only (no manual config)
	agentConfigs := scanner.BuildAgentConfigsFromScannedAgents(scannedAgents, nil)

	if len(agentConfigs) == 0 {
		implicitAgent := &config.AgentConfig{
			ID:      "main",
			Default: true,
		}
		instance := NewAgentInstance(implicitAgent, &cfg.Agents.Defaults, cfg, provider)
		registry.agents["main"] = instance
		logger.InfoCF("agent", "Created implicit main agent (no agents found in directory)", nil)
	} else {
		for i := range agentConfigs {
			ac := &agentConfigs[i]
			id := routing.NormalizeAgentID(ac.ID)
			instance := NewAgentInstance(ac, &cfg.Agents.Defaults, cfg, provider)
			registry.agents[id] = instance
			logger.InfoCF("agent", "Registered agent",
				map[string]any{
					"agent_id":  id,
					"name":      ac.Name,
					"workspace": instance.Workspace,
					"model":     instance.Model,
				})
		}
	}

	// Update resolver with registered agent list for routing decisions
	agentIDs := registry.ListAgentIDs()
	registry.resolver.SetAgentRegistry(&routing.AgentRegistryCache{
		agents: agentIDs,
	})

	return registry
}

// AddAgent dynamically adds a new agent instance to the registry.
// Returns error if agent already exists.
func (r *AgentRegistry) AddAgent(
	agentCfg *config.AgentConfig,
	defaults *config.AgentDefaults,
	cfg *config.Config,
	provider providers.LLMProvider,
) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := routing.NormalizeAgentID(agentCfg.ID)
	if _, exists := r.agents[id]; exists {
		return "", fmt.Errorf("agent %s already exists", id)
	}

	instance := NewAgentInstance(agentCfg, defaults, cfg, provider)
	r.agents[id] = instance
	logger.InfoCF("agent", "Dynamically added agent",
		map[string]any{
			"agent_id":  id,
			"name":      agentCfg.Name,
			"workspace": instance.Workspace,
			"model":     instance.Model,
		})

	return id, nil
}

// RemoveAgent dynamically removes an agent instance from the registry.
// Returns error if agent is the main agent or doesn't exist.
func (r *AgentRegistry) RemoveAgent(agentID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	id := routing.NormalizeAgentID(agentID)

	// Never allow removal of the main agent
	if id == "main" {
		return fmt.Errorf("cannot remove main agent")
	}

	agent, exists := r.agents[id]
	if !exists {
		return fmt.Errorf("agent %s not found", id)
	}

	// Close the agent to release resources
	if err := agent.Close(); err != nil {
		logger.WarnCF("agent", "Failed to close agent during removal",
			map[string]any{"agent_id": id, "error": err.Error()})
	}

	delete(r.agents, id)
	logger.InfoCF("agent", "Removed agent",
		map[string]any{"agent_id": id})

	return nil
}

// IsMainAgent checks if the given agent ID is the main agent
func (r *AgentRegistry) IsMainAgent(agentID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := routing.NormalizeAgentID(agentID)
	return id == "main"
}

// GetAgent returns the agent instance for a given ID.
func (r *AgentRegistry) GetAgent(agentID string) (*AgentInstance, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id := routing.NormalizeAgentID(agentID)
	agent, ok := r.agents[id]
	return agent, ok
}

// ResolveRoute determines which agent handles the message.
func (r *AgentRegistry) ResolveRoute(input routing.RouteInput) routing.ResolvedRoute {
	return r.resolver.ResolveRoute(input)
}

// ListAgentIDs returns all registered agent IDs.
func (r *AgentRegistry) ListAgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ids := make([]string, 0, len(r.agents))
	for id := range r.agents {
		ids = append(ids, id)
	}
	return ids
}

// CanSpawnSubagent checks if parentAgentID is allowed to spawn targetAgentID.
func (r *AgentRegistry) CanSpawnSubagent(parentAgentID, targetAgentID string) bool {
	parent, ok := r.GetAgent(parentAgentID)
	if !ok {
		return false
	}
	if parent.Subagents == nil || parent.Subagents.AllowAgents == nil {
		return false
	}
	targetNorm := routing.NormalizeAgentID(targetAgentID)
	for _, allowed := range parent.Subagents.AllowAgents {
		if allowed == "*" {
			return true
		}
		if routing.NormalizeAgentID(allowed) == targetNorm {
			return true
		}
	}
	return false
}

// ForEachTool calls fn for every tool registered under the given name
// across all agents. This is useful for propagating dependencies (e.g.
// MediaStore) to tools after registry construction.
func (r *AgentRegistry) ForEachTool(name string, fn func(tools.Tool)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agent := range r.agents {
		if t, ok := agent.Tools.Get(name); ok {
			fn(t)
		}
	}
}

// Close releases resources held by all registered agents.
func (r *AgentRegistry) Close() {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, agent := range r.agents {
		if err := agent.Close(); err != nil {
			logger.WarnCF("agent", "Failed to close agent",
				map[string]any{"agent_id": agent.ID, "error": err.Error()})
		}
	}
}

// GetDefaultAgent returns the default agent instance.
func (r *AgentRegistry) GetDefaultAgent() *AgentInstance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if agent, ok := r.agents["main"]; ok {
		return agent
	}
	for _, agent := range r.agents {
		return agent
	}
	return nil
}
