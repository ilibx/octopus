# Octopus Agents

This directory contains all agent definitions and configurations for the Octopus multi-agent system.

## Directory Structure

```
agents/
├── AGENT.md.template          # Template for creating new agents
├── main/
│   └── AGENT.md              # Main orchestrator agent documentation
├── {{agent-id}}/             # Template for sub-agent directories
│   └── AGENT.md              # Agent-specific documentation
└── README.md                 # This file
```

## Available Agents

### Main Agent
- **ID**: `main`
- **Role**: Central orchestrator, intelligent channel routing, task coordination
- **Documentation**: [`main/AGENT.md`](./main/AGENT.md)
- **Can Spawn**: Any configured sub-agent (configurable)

### Sub-Agents

Sub-agents are specialized agents spawned by the main agent for specific tasks:

| Agent ID | Purpose | Status | Documentation |
|----------|---------|--------|---------------|
| `github` | GitHub operations (PRs, issues, CI) | 🟢 Active | [skills/github/SKILL.md](../skills/github/SKILL.md) |
| `data-analyst` | Data analysis and reporting | 🟡 Planned | - |
| `devops` | Infrastructure and deployment | 🟡 Planned | - |

> **Note**: Sub-agents are defined in skills directory and spawned dynamically by the main agent based on task requirements.

## Agent Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Incoming Tasks                          │
│            (Channels, Cron, API, Webhooks)                   │
└────────────────────────┬────────────────────────────────────┘
                         │
                         ▼
                ┌─────────────────┐
                │   Main Agent    │
                │  (Orchestrator) │
                └────────┬────────┘
                         │
         ┌───────────────┼───────────────┐
         │               │               │
         ▼               ▼               ▼
   ┌──────────┐   ┌──────────┐   ┌──────────┐
   │ GitHub   │   │  Data    │   │  DevOps  │
   │  Agent   │   │ Analyst  │   │  Agent   │
   │ (Skill)  │   │ (Skill)  │   │ (Skill)  │
   └──────────┘   └──────────┘   └──────────┘
```

## Creating a New Agent

### Step 1: Create Agent Directory

```bash
mkdir -p /workspace/workspace/agents/{agent-id}
cp /workspace/workspace/agents/AGENT.md.template \
   /workspace/workspace/agents/{agent-id}/AGENT.md
```

### Step 2: Configure Agent

Edit the new `AGENT.md` file:
1. Replace all `{{placeholders}}` with actual values
2. Define model configuration and fallbacks
3. Document workflows and examples
4. Specify tool dependencies

### Step 3: Register Agent

Add to your `config.yaml`:

```yaml
agents:
  - id: {agent-id}
    name: {Agent Display Name}
    model: {provider/model-name}
    # ... additional configuration
```

### Step 4: Test Agent

```bash
# Verify agent loads correctly
octopus agents list

# Test agent functionality
octopus agents test {agent-id}
```

## Agent Configuration Reference

### Basic Configuration

```yaml
agents:
  - id: unique-identifier
    name: Display Name
    model: openai/gpt-4o
    temperature: 0.7
    max_tokens: 8192
```

### Advanced Configuration

```yaml
agents:
  - id: advanced-agent
    name: Advanced Agent
    model: openai/gpt-4o
    
    # Model routing for cost optimization
    routing:
      enabled: true
      light_model: openai/gpt-4o-mini
      threshold: 0.6
    
    # Fallback models
    fallbacks:
      - anthropic/claude-sonnet-4-5-20250929
      - google/gemini-2.5-pro
    
    # Sub-agent permissions
    subagents:
      allow_agents:
        - "*"  # Or list specific agents
    
    # Skills filter
    skills:
      - github
      - summarize
```

### Session Management

```yaml
defaults:
  sessions:
    persist: true
    max_history: 50
    summarize_threshold: 20
    token_limit_percent: 75
```

## Agent Communication

### Main → Sub-Agent

When the main agent spawns a sub-agent:

1. **Context Transfer**: Main agent passes relevant context
2. **Isolated Execution**: Sub-agent works in separate workspace
3. **Result Return**: Structured results sent back to main
4. **Integration**: Main agent integrates results into workflow

### Sub-Agent → Channel

Sub-agents do NOT send notifications directly:
- All notifications route through main agent
- Main agent selects optimal channel(s)
- Ensures consistent formatting and delivery tracking

## Model Selection Guide

| Use Case | Recommended Model | Temperature | Max Tokens |
|----------|------------------|-------------|------------|
| General chat | gpt-4o | 0.7 | 8192 |
| Code review | claude-sonnet | 0.5 | 16384 |
| Quick queries | gpt-4o-mini | 0.8 | 4096 |
| Complex reasoning | o1-preview | 1.0 | 32768 |
| Cost-sensitive | gpt-4o-mini + routing | 0.7 | 4096 |

## Monitoring Agents

### Health Checks

```bash
# List all active agents
octopus agents list

# Check agent status
octopus agents status {agent-id}

# View agent logs
octopus logs --agent {agent-id}
```

### Key Metrics

Monitor these metrics per agent:
- Task completion rate
- Average task duration
- Token consumption
- Error rate
- Sub-agent spawn frequency

### Alerting

Set up alerts for:
- Failure rate > 5%
- Average latency > 30s
- Token quota exceeded
- Repeated sub-agent failures

## Security Best Practices

1. **Workspace Isolation**: Each agent has dedicated workspace
2. **Minimal Permissions**: Grant only required tool access
3. **Credential Rotation**: Regularly rotate API keys
4. **Audit Logging**: Log all agent actions
5. **Rate Limiting**: Prevent abuse via tool limits

## Troubleshooting

### Agent Not Starting

```bash
# Check configuration syntax
octopus config validate

# Verify model availability
octopus models list

# Check workspace permissions
ls -la /workspace/workspace/agents/{agent-id}/
```

### High Token Usage

1. Enable model routing for simple queries
2. Reduce session history length
3. Lower summarization threshold
4. Review prompts for efficiency

### Sub-Agent Spawning Issues

1. Verify `subagents.allow_agents` configuration
2. Check sub-agent skill exists in `/workspace/workspace/skills/`
3. Ensure sufficient API quota
4. Review main agent logs for rejection reasons

## Related Documentation

- [Architecture Overview](../../docs/architecture.md)
- [Data Flow](../../docs/data-flow.md)
- [Skills Documentation](../skills/)
- [Channel Routing](../../docs/channels/)
- [Configuration Guide](../../docs/configuration.md)

## Contributing

When adding or modifying agents:

1. Update this README with new agent info
2. Follow the template structure exactly
3. Include comprehensive examples
4. Document failure modes and recovery
5. Add monitoring and alerting guidance
6. Test with real workloads before merging

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2025-01-15 | Initial agent framework |
| 1.1.0 | 2025-01-17 | Added model routing support |
| 1.2.0 | 2025-01-17 | Intelligent channel routing |

---

**Maintained by**: Octopus Team  
**Last Updated**: 2025-01-17  
**Contact**: See [CONTRIBUTING.md](../../CONTRIBUTING.md)
