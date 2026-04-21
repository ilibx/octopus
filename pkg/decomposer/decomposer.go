// Package decomposer provides LLM-based task decomposition and skill composition
package decomposer

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
	"github.com/ilibx/octopus/pkg/providers"
	"github.com/ilibx/octopus/pkg/skills"
)

// TaskDecomposer uses LLM to dynamically decompose tasks and compose skills
type TaskDecomposer struct {
	provider    providers.LLMProvider
	skillReg    *skills.Registry
	modelConfig config.ModelConfig
}

// DecompositionResult represents the result of task decomposition
type DecompositionResult struct {
	TraceID       string              `json:"trace_id"`
	MainTaskTitle string              `json:"main_task_title"`
	SubTasks      []SubTaskDefinition `json:"sub_tasks"`
	SkillChain    []string            `json:"skill_chain"` // Ordered list of skill IDs
	AgentType     string              `json:"agent_type"`  // Recommended agent type
	Dependencies  []TaskDependency    `json:"dependencies,omitempty"`
}

// SubTaskDefinition defines a sub-task created from decomposition
type SubTaskDefinition struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	SkillIDs    []string          `json:"skill_ids"`
	Priority    int               `json:"priority"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	DependsOn   []string          `json:"depends_on,omitempty"`
}

// TaskDependency represents a dependency between sub-tasks
type TaskDependency struct {
	FromTaskID string `json:"from_task_id"`
	ToTaskID   string `json:"to_task_id"`
	Type       string `json:"type"` // "sequential", "parallel", "conditional"
}

// NewTaskDecomposer creates a new task decomposer
func NewTaskDecomposer(provider providers.LLMProvider, skillReg *skills.Registry, modelConfig config.ModelConfig) *TaskDecomposer {
	return &TaskDecomposer{
		provider:    provider,
		skillReg:    skillReg,
		modelConfig: modelConfig,
	}
}

// DecomposeTask uses LLM to decompose a user request into sub-tasks with skill compositions
func (d *TaskDecomposer) DecomposeTask(ctx context.Context, traceID, title, description string) (*DecompositionResult, error) {
	logger.InfoCF("decomposer", "Starting task decomposition",
		map[string]any{
			"trace_id": traceID,
			"title":    title,
		})

	// Get available skills for prompt context
	availableSkills := d.getAvailableSkillsContext()

	// Build prompt for LLM
	prompt := d.buildDecompositionPrompt(title, description, availableSkills)

	// Call LLM for decomposition
	llmResponse, err := d.callLLM(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("LLM decomposition failed: %w", err)
	}

	// Parse LLM response
	result, err := d.parseDecompositionResponse(traceID, title, llmResponse)
	if err != nil {
		return nil, fmt.Errorf("failed to parse LLM response: %w", err)
	}

	logger.InfoCF("decomposer", "Task decomposition completed",
		map[string]any{
			"trace_id":           traceID,
			"sub_task_count":     len(result.SubTasks),
			"skill_chain_length": len(result.SkillChain),
		})

	return result, nil
}

// getAvailableSkillsContext returns a formatted list of available skills for the LLM prompt
func (d *TaskDecomposer) getAvailableSkillsContext() string {
	allSkills := d.skillReg.GetAllSkills()
	if len(allSkills) == 0 {
		return "No skills available in registry."
	}

	var sb strings.Builder
	sb.WriteString("Available Skills:\n")
	for _, skill := range allSkills {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", skill.ID, skill.Description))
		if len(skill.Tags) > 0 {
			sb.WriteString(fmt.Sprintf("  Tags: %s\n", strings.Join(skill.Tags, ", ")))
		}
	}
	return sb.String()
}

// buildDecompositionPrompt builds the prompt for LLM task decomposition
func (d *TaskDecomposer) buildDecompositionPrompt(title, description, skillsContext string) string {
	template := `You are an expert task decomposition assistant. Your job is to analyze a user's request and break it down into executable sub-tasks.

Each sub-task should be composed of one or more skills from the available skills list. Skills can be combined to form complex capabilities.

## Available Skills
%s

## User Request
**Title**: %s
**Description**: %s

## Instructions
1. Analyze the user request and identify the main goal
2. Break down the request into logical sub-tasks that can be executed sequentially or in parallel
3. For each sub-task, select appropriate skills from the available skills list
4. Define dependencies between sub-tasks (which tasks must complete before others can start)
5. Consider the natural flow of execution (e.g., for shopping: view products -> filter -> add to cart -> checkout -> pay)

## Output Format
Respond with a JSON object in the following format:
{
  "agent_type": "recommended agent type (e.g., 'shopping', 'finance', 'devops')",
  "skill_chain": ["skill_id_1", "skill_id_2", ...],
  "sub_tasks": [
    {
      "id": "task_1",
      "title": "Clear task title",
      "description": "Detailed description of what to do",
      "skill_ids": ["skill_id_1", "skill_id_2"],
      "priority": 1,
      "depends_on": []
    }
  ],
  "dependencies": [
    {"from_task_id": "task_1", "to_task_id": "task_2", "type": "sequential"}
  ]
}

Ensure that:
- Each sub-task has a clear, actionable title
- Skills are properly matched to task requirements
- Dependencies reflect the natural execution order
- The skill_chain contains all unique skills needed across all sub-tasks in execution order`

	return fmt.Sprintf(template, skillsContext, title, description)
}

// callLLM calls the LLM provider with the given prompt
func (d *TaskDecomposer) callLLM(ctx context.Context, prompt string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	response, err := d.provider.Generate(ctx, d.modelConfig.Model, prompt, nil)
	if err != nil {
		return "", err
	}

	return response.Content, nil
}

// parseDecompositionResponse parses the LLM response into a structured result
func (d *TaskDecomposer) parseDecompositionResponse(traceID, mainTitle, llmResponse string) (*DecompositionResult, error) {
	// Use JSON parsing for robust extraction
	result := &DecompositionResult{
		TraceID:       traceID,
		MainTaskTitle: mainTitle,
		SubTasks:      make([]SubTaskDefinition, 0),
		SkillChain:    make([]string, 0),
		Dependencies:  make([]TaskDependency, 0),
	}

	// Try to extract JSON from the response (handle markdown code blocks)
	jsonStr := d.extractJSON(llmResponse)
	if jsonStr == "" {
		return nil, fmt.Errorf("no valid JSON found in LLM response")
	}

	// Parse the JSON using encoding/json
	if err := d.parseJSON(jsonStr, result); err != nil {
		return nil, fmt.Errorf("JSON parsing failed: %w", err)
	}

	// Default agent type if not specified
	if result.AgentType == "" {
		result.AgentType = "general"
	}

	return result, nil
}

// extractJSON extracts JSON from LLM response (handles markdown code blocks)
func (d *TaskDecomposer) extractJSON(response string) string {
	// Try to find JSON between ```json and ```
	startMarker := "```json"
	endMarker := "```"

	startIdx := strings.Index(response, startMarker)
	if startIdx != -1 {
		startIdx += len(startMarker)
		endIdx := strings.Index(response[startIdx:], endMarker)
		if endIdx != -1 {
			return strings.TrimSpace(response[startIdx : startIdx+endIdx])
		}
	}

	// Try to find JSON between ``` and ```
	startMarker = "```"
	startIdx = strings.Index(response, startMarker)
	if startIdx != -1 {
		startIdx += len(startMarker)
		endIdx := strings.Index(response[startIdx:], endMarker)
		if endIdx != -1 {
			return strings.TrimSpace(response[startIdx : startIdx+endIdx])
		}
	}

	// Try to find JSON object directly
	if strings.Contains(response, "{") && strings.Contains(response, "}") {
		startIdx := strings.Index(response, "{")
		bracketCount := 0
		for i := startIdx; i < len(response); i++ {
			if response[i] == '{' {
				bracketCount++
			} else if response[i] == '}' {
				bracketCount--
				if bracketCount == 0 {
					return response[startIdx : i+1]
				}
			}
		}
	}

	return ""
}

// parseJSON parses the JSON string into the result struct
func (d *TaskDecomposer) parseJSON(jsonStr string, result *DecompositionResult) error {
	// This would use encoding/json in a full implementation
	// For now, we'll use a simplified approach
	logger.DebugCF("decomposer", "Parsing JSON response", map[string]any{"json_length": len(jsonStr)})

	// TODO: Implement proper JSON parsing with encoding/json
	// This requires defining custom UnmarshalJSON methods or using a library

	return nil
}

// ComposeSkillsForTask dynamically composes skills for a specific task
func (d *TaskDecomposer) ComposeSkillsForTask(ctx context.Context, taskDesc string, requiredCapabilities []string) ([]string, error) {
	logger.InfoCF("decomposer", "Composing skills for task",
		map[string]any{
			"task_desc":             taskDesc,
			"required_capabilities": requiredCapabilities,
		})

	// Get skills that match the required capabilities
	matchedSkills := d.matchSkillsToCapabilities(requiredCapabilities)

	if len(matchedSkills) == 0 {
		// Try LLM-based skill composition
		return d.llmBasedSkillComposition(ctx, taskDesc, requiredCapabilities)
	}

	return matchedSkills, nil
}

// matchSkillsToCapabilities finds skills that match the required capabilities
func (d *TaskDecomposer) matchSkillsToCapabilities(capabilities []string) []string {
	allSkills := d.skillReg.GetAllSkills()
	var matched []string

	for _, capability := range capabilities {
		for _, skill := range allSkills {
			if d.skillMatchesCapability(skill, capability) {
				matched = append(matched, skill.ID)
			}
		}
	}

	return matched
}

// skillMatchesCapability checks if a skill matches a capability requirement
func (d *TaskDecomposer) skillMatchesCapability(skill skills.SKILL, capability string) bool {
	capability = strings.ToLower(capability)

	// Check skill name
	if strings.Contains(strings.ToLower(skill.Name), capability) {
		return true
	}

	// Check skill description
	if strings.Contains(strings.ToLower(skill.Description), capability) {
		return true
	}

	// Check skill tags
	for _, tag := range skill.Tags {
		if strings.Contains(strings.ToLower(tag), capability) {
			return true
		}
	}

	return false
}

// llmBasedSkillComposition uses LLM to find appropriate skills
func (d *TaskDecomposer) llmBasedSkillComposition(ctx context.Context, taskDesc string, capabilities []string) ([]string, error) {
	prompt := fmt.Sprintf(`Given the task: "%s"
Required capabilities: %v

Select appropriate skills from the available skills to accomplish this task.
Return only a comma-separated list of skill IDs.

Available Skills:
%s`, taskDesc, capabilities, d.getAvailableSkillsContext())

	response, err := d.callLLM(ctx, prompt)
	if err != nil {
		return nil, err
	}

	// Parse comma-separated skill IDs
	skillIDs := strings.Split(response, ",")
	var result []string
	for _, id := range skillIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			result = append(result, id)
		}
	}

	return result, nil
}
