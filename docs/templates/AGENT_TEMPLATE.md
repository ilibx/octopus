---
name: ${AGENT_NAME}
description: "${BRIEF_DESCRIPTION}"
metadata: {"octopus":{"emoji":"${EMOJI}","requires":{"bins":[${REQUIRED_BINS}]},"install":[${INSTALL_STEPS}]}}
---

# ${AGENT_NAME} Agent

${AGENT_DESCRIPTION}

## Core Responsibilities

### 1. ${RESPONSIBILITY_1_TITLE}
- ${responsibility_1_detail_1}
- ${responsibility_1_detail_2}
- ${responsibility_1_detail_3}

### 2. ${RESPONSIBILITY_2_TITLE}
- ${responsibility_2_detail_1}
- ${responsibility_2_detail_2}

### 3. ${RESPONSIBILITY_3_TITLE}
- ${responsibility_3_detail_1}
- ${responsibility_3_detail_2}

## Model Configuration

```yaml
agents:
  - id: ${AGENT_ID}
    name: ${AGENT_DISPLAY_NAME}
    # Model specification (format: provider/model-name)
    model: ${PRIMARY_MODEL}  # e.g., openai/gpt-4o, anthropic/claude-sonnet-4-5-20250929
    
    # Optional: specify thinking level for complex reasoning
    # thinking_level: ${THINKING_LEVEL}  # Options: low, medium, high
    
    # Model parameters
    temperature: ${TEMPERATURE}  # Recommended: 0.5-0.8
    max_tokens: ${MAX_TOKENS}    # Context window size
    
    # Model routing for cost optimization (optional)
    routing:
      enabled: ${ROUTING_ENABLED}
      light_model: ${LIGHT_MODEL}  # e.g., openai/gpt-4o-mini
      threshold: ${ROUTING_THRESHOLD}  # Messages scoring below this use light model
    
    # Fallback models if primary fails (optional)
    fallbacks:
      - ${FALLBACK_MODEL_1}
      - ${FALLBACK_MODEL_2}
    
    # Sub-agent spawning permissions (optional)
    subagents:
      allow_agents:
        - "*"  # Allow spawning any configured sub-agent
        # Or specify explicitly:
        # - github
        # - data-analyst
    
    # Skills filter (optional - restrict available skills)
    skills:
      - ${SKILL_1}
      - ${SKILL_2}
```

## Tools & Skills

### Built-in Tools
- `read_file` / `write_file` / `edit_file` - File operations
- `list_dir` - Directory listing
- `exec` - Command execution (sandboxed)
- `append_file` - Append to files

### Required Skills

${SKILL_DETAILS}

### Skill Metadata Format

Skills are loaded from `/workspace/workspace/skills/*/SKILL.md`:

```markdown
# Example skill metadata
---
name: ${SKILL_NAME}
description: ${SKILL_DESCRIPTION}
metadata: {
  "octopus": {
    "emoji": "${SKILL_EMOJI}",
    "requires": {"bins": [${SKILL_BINS}]},
    "install": [
      {"id": "brew", "kind": "brew", "formula": "${SKILL_FORMULA}"}
    ]
  }
}
---
```

## Execution Flow

```
${EXECUTION_FLOW_DIAGRAM}
```

### Step-by-Step Process

1. **${STEP_1_TITLE}**
   - ${step_1_description}
   
2. **${STEP_2_TITLE}**
   - ${step_2_description}
   
3. **${STEP_3_TITLE}**
   - ${step_3_description}

## Session Management

### Configuration

```yaml
sessions:
  persist: ${SESSION_PERSIST}
  max_history: ${MAX_HISTORY}  # Messages to keep in context
  summarize_threshold: ${SUMMARIZE_THRESHOLD}  # Summarize after N messages
  token_limit_percent: ${TOKEN_LIMIT_PERCENT}  # Use X% of context window before summarizing
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

## Error Handling & Recovery

### Fallback Strategy

1. **Primary Model Failure** → Try fallback models in order
2. **Tool Execution Error** → Retry with adjusted parameters (max 3 attempts)
3. **Dependency Failure** → Gracefully degrade or queue for retry
4. **Context Overflow** → Automatic summarization and trimming

### Logging & Debugging

All agent actions are logged with structured context:

```json
{
  "timestamp": "2025-01-15T10:30:00Z",
  "agent_id": "${AGENT_ID}",
  "action": "${EXAMPLE_ACTION}",
  "context": {
    "task_id": "${EXAMPLE_TASK_ID}",
    "duration_ms": 245,
    "status": "success"
  }
}
```

## Performance Tuning

### Recommended Settings

| Workload | Model | Temperature | Max Tokens | Notes |
|----------|-------|-------------|------------|-------|
| ${WORKLOAD_1} | ${MODEL_1} | ${TEMP_1} | ${TOKENS_1} | ${NOTES_1} |
| ${WORKLOAD_2} | ${MODEL_2} | ${TEMP_2} | ${TOKENS_2} | ${NOTES_2} |

### Resource Limits

```yaml
defaults:
  max_tool_iterations: ${MAX_TOOL_ITERATIONS}  # Prevent infinite tool loops
  max_tokens: ${DEFAULT_MAX_TOKENS}
  temperature: ${DEFAULT_TEMPERATURE}
  restrict_to_workspace: ${RESTRICT_WORKSPACE}
  allow_read_outside_workspace: ${ALLOW_READ_OUTSIDE}
```

## Monitoring & Observability

### Lightweight Monitoring

- **Log-based metrics**: Parse logs for KPIs (tasks/hour, success rate, avg duration)
- **Health checks**: Periodic ping to verify agent responsiveness
- **Alert thresholds**: Configure alerts for failure rates > 5% or latency > 30s

### Key Metrics to Track

1. Task completion rate
2. Average task duration
3. Tool usage frequency
4. Token usage per task type
5. Error rate by category

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

### Workflow 1: ${WORKFLOW_1_NAME}

```
${WORKFLOW_1_STEPS}
```

### Workflow 2: ${WORKFLOW_2_NAME}

```
${WORKFLOW_2_STEPS}
```

## Troubleshooting

### Common Issues

**Issue**: ${COMMON_ISSUE_1}
- ${solution_1}
- ${solution_2}
- ${solution_3}

**Issue**: ${COMMON_ISSUE_2}
- ${solution_1}
- ${solution_2}

## Best Practices

1. **${BEST_PRACTICE_1}**: ${best_practice_1_detail}
2. **${BEST_PRACTICE_2}**: ${best_practice_2_detail}
3. **${BEST_PRACTICE_3}**: ${best_practice_3_detail}
4. **${BEST_PRACTICE_4}**: ${best_practice_4_detail}

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | ${CURRENT_DATE} | Initial version |
