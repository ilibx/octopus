package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/ilibx/octopus/pkg/logger"
)

// PromptContext contains all data needed to build a dynamic prompt
type PromptContext struct {
	AgentName       string            `json:"agent_name"`
	Description     string            `json:"description"`
	Model           string            `json:"model"`
	Role            string            `json:"role"`
	Skills          []string          `json:"skills"`
	SkillsContext   string            `json:"skills_context"` // Injected SKILL definitions
	Tools           []ToolDefinition  `json:"tools"`
	TaskDescription string            `json:"task_description"`
	InputFrom       map[string]string `json:"input_from"` // Results from previous tasks
	OutputSchema    string            `json:"output_schema"`
	MaxSteps        int               `json:"max_steps"`
	TimeoutSeconds  int               `json:"timeout_seconds"`
	RetryLimit      int               `json:"retry_limit"`
	LoopGuard       bool              `json:"loop_guard"`
}

// ToolDefinition represents a tool configuration
type ToolDefinition struct {
	Name     string `json:"name"`
	Type     string `json:"type"` // api|mcp|local
	Endpoint string `json:"endpoint"`
}

// PromptEngine handles dynamic prompt generation with template injection
type PromptEngine struct {
	templates map[string]*template.Template
	cache     sync.RWMutex
	baseDir   string
}

// NewPromptEngine creates a new prompt engine
func NewPromptEngine(agentsDir string) (*PromptEngine, error) {
	engine := &PromptEngine{
		templates: make(map[string]*template.Template),
		baseDir:   agentsDir,
	}

	// Pre-load templates if directory exists
	if err := engine.loadTemplates(); err != nil {
		logger.WarnCF("prompt_engine", "Failed to pre-load templates",
			map[string]any{"error": err.Error()})
	}

	return engine, nil
}

// loadTemplates loads all agent templates from the configured directory
func (pe *PromptEngine) loadTemplates() error {
	// Templates will be loaded on-demand via LoadTemplate method
	// This is a placeholder for future file system watching/hot-reload
	return nil
}

// BuildPrompt builds a complete prompt by rendering the template with context
func (pe *PromptEngine) BuildPrompt(agentType string, context PromptContext) (string, error) {
	pe.cache.RLock()
	tmpl, exists := pe.templates[agentType]
	pe.cache.RUnlock()

	if !exists {
		// Load template on-demand
		var err error
		tmpl, err = pe.loadTemplate(agentType)
		if err != nil {
			return "", fmt.Errorf("template not found and cannot load: %s - %w", agentType, err)
		}

		// Cache the template
		pe.cache.Lock()
		pe.templates[agentType] = tmpl
		pe.cache.Unlock()
	}

	// Execute template with context
	var buf strings.Builder
	if err := tmpl.Execute(&buf, context); err != nil {
		return "", fmt.Errorf("template execution failed: %w", err)
	}

	return buf.String(), nil
}

// loadTemplate loads a template from file or returns a default template
func (pe *PromptEngine) loadTemplate(agentType string) (*template.Template, error) {
	// Try to load from file system first
	// In production, this would read from workspace/agents/{agentType}/AGENT.md

	// For now, return the default sub-agent template
	defaultTemplate := `---
name: "{{.AgentName}}"
description: "{{.Description}}"
model: "{{.Model}}"
role: "{{.Role}}"
skills:
{{- range .Skills}}
  - "{{.}}"
{{- end}}
{{- if .Tools}}
tools:
{{- range .Tools}}
  - name: "{{.Name}}"
    type: "{{.Type}}"
    endpoint: "{{.Endpoint}}"
{{- end}}
{{- end}}
execution_policy:
  max_steps: {{.MaxSteps}}
  timeout_seconds: {{.TimeoutSeconds}}
  retry_limit: {{.RetryLimit}}
  loop_guard: {{.LoopGuard}}
output_schema: "{{.OutputSchema}}"
---
# 🎯 角色定义与职责边界

{{.Role}}

⚠️ 严格限制：你只能使用 skills 列表中声明的能力。禁止越权调用、编造数据或输出未授权内容。

# 🔧 SKILL 调用协议
## 可用能力清单

{{.SkillsContext}}

## 调用规范
1. 严格按看板传入的 TaskDescription 与 InputFrom 执行。
2. 每次调用后必须校验返回结果，失败则立即记录并进入重试/降级流程。
3. 不得缓存敏感数据，不得跨任务传递中间状态（仅通过看板读写）。

# 🔄 标准执行流程
1. **解析上下文**：读取前置任务输出与当前任务目标。
2. **能力路由**：根据当前阶段选择最匹配的 SKILL。
3. **执行与验证**：调用 → 校验 Schema → 记录中间状态。
4. **结果封装**：按 output_schema 严格格式化，准备回写看板。

# 🛡️ 约束与熔断机制
- **步数限制**：单任务最多 {{.MaxSteps}} 轮 LLM 交互，超限自动终止。
- **循环检测**：连续 2 次调用同一 SKILL 且参数未变化，视为死循环，上报 loop_detected。
- **超时控制**：总执行时间不得超过 {{.TimeoutSeconds}}s，超时标记 timeout。
- **重试策略**：失败最多重试 {{.RetryLimit}} 次，仍失败则输出错误详情。

# 📝 输出规范
最终必须输出以下 JSON 结构（严禁附加 Markdown、解释性文本或代码块标记）：
{
  "status": "success|failed|partial",
  "result": "{{.TaskDescription}}",
  "error_message": "",
  "steps_executed": [],
  "next_action": "complete"
}
`

	return template.New(agentType).Parse(defaultTemplate)
}

// LoadTemplateFromFile loads a template from a specific file path
func (pe *PromptEngine) LoadTemplateFromFile(agentType, filePath string) error {
	tmpl, err := template.ParseFiles(filePath)
	if err != nil {
		return fmt.Errorf("parse template failed: %w", err)
	}

	pe.cache.Lock()
	pe.templates[agentType] = tmpl
	pe.cache.Unlock()

	logger.InfoCF("prompt_engine", "Template loaded from file",
		map[string]any{
			"agent_type": agentType,
			"file_path":  filePath,
		})

	return nil
}

// ClearCache clears the template cache (useful for hot-reload scenarios)
func (pe *PromptEngine) ClearCache() {
	pe.cache.Lock()
	defer pe.cache.Unlock()
	pe.templates = make(map[string]*template.Template)
}

// BuildDefaultContext creates a default prompt context with sensible defaults
func BuildDefaultContext(agentName, description, role string, skills []string) PromptContext {
	return PromptContext{
		AgentName:      agentName,
		Description:    description,
		Model:          "gpt-4o-mini",
		Role:           role,
		Skills:         skills,
		SkillsContext:  "", // Will be injected at runtime
		Tools:          []ToolDefinition{},
		InputFrom:      make(map[string]string),
		OutputSchema:   "json",
		MaxSteps:       15,
		TimeoutSeconds: 300,
		RetryLimit:     2,
		LoopGuard:      true,
	}
}

// InjectSkillsContext injects SKILL definitions into the context
func (pc *PromptContext) InjectSkillsContext(skillsDefinitions map[string]string) {
	var sb strings.Builder

	sb.WriteString("## 已加载的 SKILL 能力:\n\n")
	for skillID, definition := range skillsDefinitions {
		sb.WriteString(fmt.Sprintf("### SKILL: %s\n", skillID))
		sb.WriteString(definition)
		sb.WriteString("\n\n")
	}

	pc.SkillsContext = sb.String()
}

// Validate checks if the context has all required fields
func (pc *PromptContext) Validate() error {
	if pc.AgentName == "" {
		return fmt.Errorf("agent_name is required")
	}
	if pc.Role == "" {
		return fmt.Errorf("role is required")
	}
	if len(pc.Skills) == 0 {
		return fmt.Errorf("at least one skill is required")
	}
	if pc.MaxSteps <= 0 {
		return fmt.Errorf("max_steps must be positive")
	}
	if pc.TimeoutSeconds <= 0 {
		return fmt.Errorf("timeout_seconds must be positive")
	}
	return nil
}

// GetTimeout returns the timeout duration
func (pc *PromptContext) GetTimeout() time.Duration {
	return time.Duration(pc.TimeoutSeconds) * time.Second
}

// CreateTimeoutContext creates a context with the configured timeout
func (pc *PromptContext) CreateTimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, pc.GetTimeout())
}
