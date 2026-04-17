---
name: main
description: "Main agent responsible for task orchestration, intelligent channel routing, and coordinating sub-agents."
metadata: {"octopus":{"emoji":"🐙","requires":{"bins":[]},"install":[]}}
---

# Main Agent

The main agent is the central orchestrator in the Octopus system. It handles incoming tasks from all sources (channels, cron jobs, API) and intelligently routes notifications through appropriate channels based on task nature and context.

## Core Responsibilities

### 1. Task Orchestration
- Receive tasks from kanban board, channels, or cron scheduler
- Analyze task requirements and determine execution strategy
- Spawn specialized sub-agents when needed (based on `subagents.allow_agents` configuration)
- Coordinate multi-step workflows across different tools and services

### 2. Intelligent Channel Routing
- Automatically select optimal notification channel(s) based on:
  - Task urgency and priority
  - Content type (text, media, files)
  - Target audience (individual, group, broadcast)
  - Time sensitivity and user preferences
- Support multi-channel broadcasting for critical alerts
- Handle channel-specific formatting and limitations

### 3. Context Management
- Maintain conversation history and session state
- Manage short-term memory for active tasks
- Persist important context across sessions
- Implement automatic summarization for long conversations

## Model Configuration

```yaml
agents:
  - id: main
    name: Main Orchestrator
    model: openai/gpt-4o
    # Optional: specify thinking level for complex reasoning
    # thinking_level: high  # Options: low, medium, high
    temperature: 0.7
    max_tokens: 8192
    
    # Model routing for cost optimization
    routing:
      enabled: true
      light_model: openai/gpt-4o-mini
      threshold: 0.6  # Messages scoring below this use light model
    
    # Fallback models if primary fails
    fallbacks:
      - anthropic/claude-sonnet-4-5-20250929
      - google/gemini-2.5-pro
    
    # Sub-agent spawning permissions
    subagents:
      allow_agents:
        - "*"  # Allow spawning any configured sub-agent
        # Or specify explicitly:
        # - github
        # - data-analyst
    
    # Skills filter (optional - restrict available skills)
    skills:
      - github
      - summarize
      - exec
```

## Channel Routing Logic

The main agent automatically determines notification channels using this decision tree:

```
Task Received
    ↓
Analyze Task Metadata
    ├── Priority: CRITICAL → All configured channels
    ├── Priority: HIGH → Primary channel + backup
    ├── Priority: NORMAL → Primary channel only
    └── Priority: LOW → Batch with other notifications
    
Content Type Analysis:
    ├── Text-only → Slack/Discord/Telegram
    ├── Rich media → Discord/Matrix
    ├── Files/Documents → Email/Feishu
    └── Interactive → Slack/Discord (buttons/actions)
    
Recipient Analysis:
    ├── Individual → Direct message
    ├── Team → Group channel
    └── Broadcast → All subscribed channels
```

### Example: Cron Task Routing

When a cron task triggers:

```go
// Cron payload example
{
  "task_id": "daily-report",
  "metadata": {
    "priority": "normal",
    "content_type": "report",
    "requires_ack": false,
    "channels": ["slack", "email"]  // Optional: force specific channels
  }
}

// Main agent receives task and:
// 1. Analyzes metadata
// 2. Checks channel availability
// 3. Formats message per channel
// 4. Sends notifications
// 5. Updates kanban with delivery status
```

## Sub-agent Coordination

### Spawning Sub-agents

The main agent can spawn specialized sub-agents for specific tasks:

```yaml
# Example: GitHub PR review workflow
main_agent receives: "Review PR #123 in octopus repo"
    ↓
Spawns github sub-agent
    ↓
Sub-agent:
  - Fetches PR details via gh CLI
  - Runs code analysis
  - Posts review comments
    ↓
Returns summary to main agent
    ↓
Main agent notifies via Slack
```

### Sub-agent Communication Protocol

1. **Task Delegation**: Main agent passes structured context to sub-agent
2. **Execution**: Sub-agent operates in isolated workspace
3. **Result Return**: Sub-agent returns structured results
4. **Integration**: Main agent integrates results into broader workflow

## Session Management

### Configuration

```yaml
sessions:
  persist: true
  max_history: 50  # Messages to keep in context
  summarize_threshold: 20  # Summarize after N messages
  token_limit_percent: 75  # Use 75% of context window before summarizing
```

### Session Lifecycle

```
New Message
    ↓
Load/Create Session
    ↓
Check Context Length
    ├── Under threshold → Process normally
    └── Over threshold → Summarize old messages
        ↓
    Trimmed Context Ready
        ↓
Process with LLM
        ↓
Update Session State
```

## Tools & Skills

### Built-in Tools
- `read_file` / `write_file` / `edit_file` - File operations
- `list_dir` - Directory listing
- `exec` - Command execution (sandboxed)
- `append_file` - Append to files

### Skill Integration

Skills are loaded from `/workspace/workspace/skills/*/SKILL.md`:

```markdown
# Example skill metadata
---
name: github
description: Interact with GitHub
metadata: {
  "octopus": {
    "emoji": "🐙",
    "requires": {"bins": ["gh"]},
    "install": [
      {"id": "brew", "kind": "brew", "formula": "gh"}
    ]
  }
}
---
```

## Error Handling & Recovery

### Fallback Strategy

1. **Primary Model Failure** → Try fallback models in order
2. **Tool Execution Error** → Retry with adjusted parameters (max 3 attempts)
3. **Channel Delivery Failure** → Queue for retry, notify via backup channel
4. **Sub-agent Crash** → Restart sub-agent, redelegate task

### Logging & Debugging

All agent actions are logged with structured context:

```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "agent_id": "main",
  "action": "route_notification",
  "context": {
    "task_id": "cron-daily-001",
    "selected_channels": ["slack", "email"],
    "routing_reason": "priority=high,content_type=report",
    "duration_ms": 245
  }
}
```

## Performance Tuning

### Recommended Settings

| Workload | Model | Temperature | Max Tokens | Routing |
|----------|-------|-------------|------------|---------|
| General purpose | gpt-4o | 0.7 | 8192 | Enabled |
| Code analysis | claude-sonnet | 0.5 | 16384 | Disabled |
| Quick responses | gpt-4o-mini | 0.8 | 4096 | N/A |
| Complex reasoning | o1-preview | 1.0 | 32768 | Disabled |

### Resource Limits

```yaml
defaults:
  max_tool_iterations: 20  # Prevent infinite tool loops
  max_tokens: 8192
  temperature: 0.7
  restrict_to_workspace: true
  allow_read_outside_workspace: false
```

## Monitoring & Observability

### Lightweight Monitoring

- **Log-based metrics**: Parse logs for KPIs (tasks/hour, success rate, avg duration)
- **Health checks**: Periodic ping to verify agent responsiveness
- **Alert thresholds**: Configure alerts for failure rates > 5% or latency > 30s

### Key Metrics to Track

1. Task completion rate
2. Average task duration
3. Channel delivery success rate
4. Sub-agent spawn frequency
5. Token usage per task type

## Security Considerations

### Workspace Isolation

- Each agent operates in isolated workspace directory
- File access restricted to workspace by default
- Exec tool runs in sandboxed environment
- No network access except through configured tools

### Credential Management

- API keys stored in environment variables or secret manager
- Never log sensitive credentials
- Rotate credentials periodically
- Use minimal permission scopes

## Example Workflows

### Workflow 1: Daily Report Generation

```
1. Cron triggers at 9:00 AM
   ↓
2. Main agent receives task with metadata:
   {type: "report", priority: "normal", channels: ["slack", "email"]}
   ↓
3. Spawns data-analyst sub-agent to generate report
   ↓
4. Sub-agent queries database, generates charts
   ↓
5. Main agent formats report for each channel
   ↓
6. Sends to Slack (rich format) and Email (PDF attachment)
   ↓
7. Marks task as COMPLETED in kanban
```

### Workflow 2: Critical Alert

```
1. Monitoring system posts to webhook
   ↓
2. Main agent analyzes alert severity
   ↓
3. Routes to ALL channels (Slack, SMS, Email, PagerDuty)
   ↓
4. Spawns incident-response sub-agent
   ↓
5. Sub-agent gathers diagnostic data
   ↓
6. Main agent provides real-time updates until resolved
```

## Troubleshooting

### Common Issues

**Issue**: Agent not routing to expected channel
- Check channel configuration in `config.yaml`
- Verify channel credentials are valid
- Review routing logic in agent prompt
- Check task metadata for channel hints

**Issue**: Sub-agent spawning fails
- Verify `subagents.allow_agents` includes target agent
- Check sub-agent configuration exists
- Ensure sufficient resources (memory, API quota)
- Review sub-agent logs for initialization errors

**Issue**: High token usage
- Enable model routing for simple queries
- Reduce `max_history` in session config
- Lower `summarize_threshold`
- Use more efficient prompts

## Best Practices

1. **Keep prompts focused**: Clear, concise instructions reduce token usage
2. **Use sub-agents wisely**: Only spawn when specialization adds value
3. **Batch notifications**: Group low-priority updates to reduce channel noise
4. **Monitor costs**: Track token usage per agent and optimize model selection
5. **Test routing logic**: Validate channel selection with diverse scenarios
6. **Implement circuit breakers**: Prevent cascade failures in dependent services
