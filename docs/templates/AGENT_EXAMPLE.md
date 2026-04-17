---
name: data-analyst
description: "Specialized agent for data analysis, report generation, and business intelligence tasks."
metadata: {"octopus":{"emoji":"📊","requires":{"bins":["python3"]},"install":[{"id":"pip","kind":"pip","package":"pandas,matplotlib,seaborn","bins":["python3"],"label":"Install data analysis libraries"}]}}
---

# Data Analyst Agent

The data analyst agent specializes in processing structured data, generating insights, creating visualizations, and producing comprehensive reports. It works under the coordination of the main agent and can be spawned for data-intensive tasks.

## Core Responsibilities

### 1. Data Processing & Analysis
- Load and clean data from various sources (CSV, JSON, databases, APIs)
- Perform statistical analysis and identify patterns
- Handle missing values and data quality issues
- Execute complex queries and aggregations

### 2. Report Generation
- Create structured reports with key findings
- Generate executive summaries for stakeholders
- Format output for different audiences (technical vs business)
- Export reports in multiple formats (PDF, HTML, Markdown)

### 3. Visualization Creation
- Build charts and graphs using matplotlib/seaborn
- Create interactive dashboards when needed
- Select appropriate chart types for data characteristics
- Ensure visualizations are accessible and clear

## Model Configuration

```yaml
agents:
  - id: data-analyst
    name: Data Analyst
    # Use models with strong reasoning capabilities for analysis
    model: anthropic/claude-sonnet-4-5-20250929
    
    # Higher thinking level for complex analytical tasks
    thinking_level: high
    
    # Model parameters optimized for precision
    temperature: 0.5  # Lower temperature for accurate analysis
    max_tokens: 16384  # Large context for data and code
    
    # Disable routing for consistency in analysis
    routing:
      enabled: false
    
    # Fallback to other capable models
    fallbacks:
      - openai/gpt-4o
      - google/gemini-2.5-pro
    
    # Skills specific to data analysis
    skills:
      - exec
      - summarize
      - file_operations
```

## Tools & Skills

### Built-in Tools
- `read_file` / `write_file` / `edit_file` - Data file operations
- `list_dir` - Browse data directories
- `exec` - Execute Python scripts for analysis
- `append_file` - Append results to reports

### Required Skills

#### Python Data Stack
The agent uses Python with these libraries:
- **pandas**: Data manipulation and analysis
- **matplotlib/seaborn**: Static visualizations
- **numpy**: Numerical computations
- **scipy**: Statistical tests (optional)

### Skill Metadata Format

Example skill for data analysis:

```markdown
---
name: data-analysis
description: Execute Python code for data analysis and visualization
metadata: {
  "octopus": {
    "emoji": "📊",
    "requires": {"bins": ["python3"]},
    "install": [
      {"id": "pip", "kind": "pip", "packages": ["pandas", "matplotlib", "seaborn"]}
    ]
  }
}
---
```

## Execution Flow

```
Task Received from Main Agent
    ↓
Parse Task Requirements
    ├── Data source identification
    ├── Analysis type determination
    └── Output format specification
    ↓
Load & Validate Data
    ├── Check file existence
    ├── Validate schema
    └── Handle missing values
    ↓
Execute Analysis
    ├── Write Python script
    ├── Run analysis code
    └── Capture results/errors
    ↓
Generate Insights
    ├── Identify key findings
    ├── Calculate metrics
    └── Draw conclusions
    ↓
Create Visualizations (if needed)
    ├── Select chart types
    ├── Generate plots
    └── Save image files
    ↓
Format Report
    ├── Structure findings
    ├── Add visualizations
    └── Export in requested format
    ↓
Return Results to Main Agent
```

### Step-by-Step Process

1. **Task Understanding**
   - Analyze the request from main agent
   - Identify required data sources and analysis type
   - Clarify ambiguities by asking main agent if needed
   
2. **Data Preparation**
   - Load data from specified sources
   - Clean and preprocess (handle missing values, outliers)
   - Validate data quality and schema
   
3. **Analysis Execution**
   - Write and execute Python code for analysis
   - Iterate based on intermediate results
   - Document methodology and assumptions
   
4. **Result Synthesis**
   - Summarize key findings
   - Create visualizations if requested
   - Format output according to requirements
   
5. **Delivery**
   - Return structured results to main agent
   - Include raw data, code, and narrative summary
   - Highlight any limitations or caveats

## Session Management

### Configuration

```yaml
sessions:
  persist: true  # Keep analysis context for follow-up questions
  max_history: 30  # Retain sufficient context for iterative analysis
  summarize_threshold: 15  # Summarize after 15 messages
  token_limit_percent: 70  # Conserve tokens for large datasets
```

### Session Lifecycle

For data analysis tasks, sessions typically follow this pattern:

```
Initial Request (e.g., "Analyze sales data")
    ↓
Load Dataset + Context
    ↓
Iterative Analysis Loop
    ├── Write code → Execute → Review results
    ├── Refine approach based on findings
    └── Ask clarifying questions if needed
    ↓
Final Report Generation
    ↓
Session Archive (for future reference)
```

## Error Handling & Recovery

### Fallback Strategy

1. **Code Execution Errors**
   - Parse error message and identify root cause
   - Attempt fix (e.g., handle missing columns, adjust data types)
   - Retry with corrected code (max 3 attempts)
   - Report persistent issues to main agent

2. **Data Quality Issues**
   - Detect and report data problems (missing values, inconsistencies)
   - Suggest remediation strategies
   - Proceed with available data if acceptable

3. **Resource Constraints**
   - Handle large datasets with chunking
   - Optimize memory usage
   - Fall back to sampling if dataset too large

### Logging & Debugging

Example structured log:

```json
{
  "timestamp": "2025-01-15T14:22:00Z",
  "agent_id": "data-analyst",
  "action": "execute_analysis",
  "context": {
    "task_id": "sales-report-q4",
    "dataset": "sales_2024.csv",
    "rows_processed": 15420,
    "execution_time_ms": 3200,
    "status": "success",
    "output_files": ["summary.md", "trends.png"]
  }
}
```

## Performance Tuning

### Recommended Settings

| Workload | Model | Temperature | Max Tokens | Notes |
|----------|-------|-------------|------------|-------|
| Exploratory analysis | claude-sonnet | 0.5 | 16384 | Precision over creativity |
| Quick summaries | gpt-4o-mini | 0.7 | 4096 | Fast turnaround |
| Complex modeling | claude-sonnet | 0.3 | 32768 | Maximum accuracy |
| Visualization design | gpt-4o | 0.6 | 8192 | Balance creativity/clarity |

### Resource Limits

```yaml
defaults:
  max_tool_iterations: 15  # Prevent infinite analysis loops
  max_tokens: 16384
  temperature: 0.5
  restrict_to_workspace: true
  allow_read_outside_workspace: false
  max_data_size_mb: 100  # Limit dataset size
```

## Monitoring & Observability

### Lightweight Monitoring

Track these metrics via log parsing:
- **Analysis success rate**: % of tasks completed without errors
- **Average execution time**: Time from task receipt to delivery
- **Code iteration count**: Number of code revisions per task
- **Data volume processed**: Average rows/columns handled

### Key Metrics to Track

1. Task completion rate (target: >95%)
2. Average analysis duration (target: <2 minutes for standard reports)
3. Code execution error rate (target: <5%)
4. User satisfaction (via main agent feedback)

## Security Considerations

### Workspace Isolation

- Only access data within `/workspace/data/` directory
- No network access except through approved API tools
- Executed code runs in sandboxed Python environment
- Generated files stored in designated output directory

### Credential Management

- Database credentials via environment variables
- API keys stored securely, never hardcoded
- Temporary files cleaned up after analysis
- No logging of sensitive data values

## Example Workflows

### Workflow 1: Monthly Sales Report

```
1. Main agent receives cron trigger: "Generate monthly sales report"
   ↓
2. Spawns data-analyst agent with task details
   ↓
3. Data analyst:
   - Loads sales data from CSV/database
   - Calculates MoM growth, top products, regional performance
   - Creates trend charts and breakdown visualizations
   ↓
4. Generates Markdown report with embedded charts
   ↓
5. Returns report to main agent
   ↓
6. Main agent routes to Slack (summary) + Email (full PDF)
   ↓
7. Marks task as COMPLETED
```

### Workflow 2: Ad-hoc Data Query

```
1. User asks via Slack: "What were our top 10 products last quarter?"
   ↓
2. Channel handler creates kanban task
   ↓
3. Main agent spawns data-analyst
   ↓
4. Data analyst:
   - Queries database for Q3 sales
   - Aggregates by product
   - Returns top 10 with revenue figures
   ↓
5. Main agent formats response for Slack
   ↓
6. Delivers answer to user within seconds
```

## Troubleshooting

### Common Issues

**Issue**: "ModuleNotFoundError: No module named 'pandas'"
- Verify Python environment has required packages installed
- Check `skill.metadata.install` configuration
- Install manually: `pip install pandas matplotlib seaborn`
- Restart agent to reload skill definitions

**Issue**: "MemoryError when loading large dataset"
- Implement chunked processing: `pd.read_csv(..., chunksize=10000)`
- Sample dataset for exploratory work
- Use more efficient data types (categorical, int32 vs int64)
- Request additional resources if needed

**Issue**: "Analysis produces unexpected results"
- Validate data source and schema
- Check for data quality issues (duplicates, nulls)
- Review code logic and assumptions
- Add debug output to trace calculations

## Best Practices

1. **Start with data exploration**: Always inspect data before analysis to understand structure and quality
2. **Document assumptions**: Clearly state any assumptions made during analysis
3. **Validate results**: Cross-check calculations with manual spot checks
4. **Optimize for readability**: Write clear, commented code that others can understand
5. **Handle edge cases**: Account for empty datasets, missing columns, unusual values
6. **Progressive disclosure**: Provide summary first, offer detailed breakdown on request

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 1.0.0 | 2025-01-15 | Initial version |
| 1.0.1 | 2025-01-17 | Added chunked processing for large datasets |
| 1.1.0 | 2025-01-20 | Enhanced visualization capabilities |
