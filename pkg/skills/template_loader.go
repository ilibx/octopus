package skills

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"

	"github.com/ilibx/octopus/pkg/logger"
)

// SkillTemplate represents a parsed skill template ready for injection
type SkillTemplate struct {
	Name        string
	Description string
	Template    *template.Template
	RawContent  string
}

// TemplateLoader handles dynamic loading and rendering of skill templates
type TemplateLoader struct {
	loader   *SkillsLoader
	cache    map[string]*SkillTemplate
	mu       sync.RWMutex
	baseVars map[string]interface{} // Global variables available to all templates
}

// TemplateContext contains all variables available during template rendering
type TemplateContext struct {
	SkillName       string            `json:"skill_name"`
	SkillContent    string            `json:"skill_content"`
	TaskDescription string            `json:"task_description"`
	InputFrom       map[string]string `json:"input_from"`  // Results from previous tasks
	UserVars        map[string]string `json:"user_vars"`   // User-provided variables
	SystemVars      map[string]string `json:"system_vars"` // System-provided variables
}

// NewTemplateLoader creates a new template loader
func NewTemplateLoader(loader *SkillsLoader) *TemplateLoader {
	return &TemplateLoader{
		loader:   loader,
		cache:    make(map[string]*SkillTemplate),
		baseVars: make(map[string]interface{}),
	}
}

// SetBaseVar sets a global variable available to all template renderings
func (tl *TemplateLoader) SetBaseVar(key string, value interface{}) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.baseVars[key] = value
}

// LoadTemplate loads and parses a skill template
func (tl *TemplateLoader) LoadTemplate(skillName string) (*SkillTemplate, error) {
	// Check cache first
	tl.mu.RLock()
	cached, exists := tl.cache[skillName]
	tl.mu.RUnlock()

	if exists {
		return cached, nil
	}

	// Load skill content
	content, ok := tl.loader.LoadSkill(skillName)
	if !ok {
		return nil, fmt.Errorf("skill not found: %s", skillName)
	}

	// Parse as template
	tmpl, err := template.New(skillName).Funcs(tl.buildFuncMap()).Parse(content)
	if err != nil {
		return nil, fmt.Errorf("parse template failed: %w", err)
	}

	// Extract metadata
	metadata := tl.loader.getSkillMetadataByPath(skillName)
	name := skillName
	description := ""
	if metadata != nil {
		if metadata.Name != "" {
			name = metadata.Name
		}
		description = metadata.Description
	}

	skillTmpl := &SkillTemplate{
		Name:        name,
		Description: description,
		Template:    tmpl,
		RawContent:  content,
	}

	// Cache the template
	tl.mu.Lock()
	tl.cache[skillName] = skillTmpl
	tl.mu.Unlock()

	logger.InfoCF("skill_template", "Template loaded",
		map[string]any{
			"skill_name": skillName,
			"cached":     true,
		})

	return skillTmpl, nil
}

// RenderTemplate renders a skill template with the given context
func (tl *TemplateLoader) RenderTemplate(skillName string, context TemplateContext) (string, error) {
	skillTmpl, err := tl.LoadTemplate(skillName)
	if err != nil {
		return "", err
	}

	// Build render data
	renderData := tl.buildRenderData(context)

	// Execute template
	var buf bytes.Buffer
	if err := skillTmpl.Template.Execute(&buf, renderData); err != nil {
		return "", fmt.Errorf("execute template failed: %w", err)
	}

	return buf.String(), nil
}

// RenderSkillsForTask renders multiple skill templates for a task
func (tl *TemplateLoader) RenderSkillsForTask(skillNames []string, context TemplateContext) (map[string]string, error) {
	results := make(map[string]string)

	for _, skillName := range skillNames {
		rendered, err := tl.RenderTemplate(skillName, context)
		if err != nil {
			logger.WarnCF("skill_template", "Failed to render skill",
				map[string]any{
					"skill_name": skillName,
					"error":      err.Error(),
				})
			continue
		}
		results[skillName] = rendered
	}

	return results, nil
}

// buildRenderData combines context with base variables
func (tl *TemplateLoader) buildRenderData(context TemplateContext) map[string]interface{} {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	data := make(map[string]interface{})

	// Add base variables
	for k, v := range tl.baseVars {
		data[k] = v
	}

	// Add context variables
	data["skill_name"] = context.SkillName
	data["skill_content"] = context.SkillContent
	data["task_description"] = context.TaskDescription
	data["input_from"] = context.InputFrom
	data["user_vars"] = context.UserVars
	data["system_vars"] = context.SystemVars

	// Helper functions available in templates
	data["join"] = strings.Join
	data["has_input"] = func(key string) bool {
		_, exists := context.InputFrom[key]
		return exists
	}
	data["get_input"] = func(key string) string {
		if val, exists := context.InputFrom[key]; exists {
			return val
		}
		return ""
	}

	return data
}

// buildFuncMap returns custom template functions
func (tl *TemplateLoader) buildFuncMap() template.FuncMap {
	return template.FuncMap{
		"upper": strings.ToUpper,
		"lower": strings.ToLower,
		"title": strings.Title,
		"trim":  strings.TrimSpace,
		"split": func(sep, s string) []string { return strings.Split(s, sep) },
		"join":  func(sep string, elems []string) string { return strings.Join(elems, sep) },
		"contains": func(substr, s string) bool {
			return strings.Contains(s, substr)
		},
		"replace": func(old, new, s string) string {
			return strings.ReplaceAll(s, old, new)
		},
		"default": func(def, val string) string {
			if val == "" {
				return def
			}
			return val
		},
	}
}

// ClearCache clears the template cache
func (tl *TemplateLoader) ClearCache() {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	tl.cache = make(map[string]*SkillTemplate)
	logger.InfoCF("skill_template", "Template cache cleared", nil)
}

// GetCacheStats returns cache statistics
func (tl *TemplateLoader) GetCacheStats() map[string]interface{} {
	tl.mu.RLock()
	defer tl.mu.RUnlock()

	totalSize := 0
	for _, tmpl := range tl.cache {
		totalSize += len(tmpl.RawContent)
	}

	return map[string]interface{}{
		"cached_templates": len(tl.cache),
		"total_size_bytes": totalSize,
	}
}

// InjectSkillsContext builds a formatted skills context string for prompt injection
func (tl *TemplateLoader) InjectSkillsContext(skillNames []string, context TemplateContext) (string, error) {
	var sb strings.Builder

	sb.WriteString("## 🔧 已加载的 SKILL 能力清单\n\n")

	rendered, err := tl.RenderSkillsForTask(skillNames, context)
	if err != nil {
		return "", err
	}

	for skillName, content := range rendered {
		sb.WriteString(fmt.Sprintf("### SKILL: %s\n", skillName))
		sb.WriteString(content)
		sb.WriteString("\n\n---\n\n")
	}

	return sb.String(), nil
}

// getSkillMetadataByPath retrieves metadata for a skill by name/path
func (sl *SkillsLoader) getSkillMetadataByPath(skillName string) *SkillMetadata {
	// Try to find the skill file
	paths := []string{
		filepath.Join(sl.workspaceSkills, skillName, "SKILL.md"),
		filepath.Join(sl.globalSkills, skillName, "SKILL.md"),
		filepath.Join(sl.builtinSkills, skillName, "SKILL.md"),
	}

	for _, path := range paths {
		if _, err := os.Stat(path); err == nil {
			return sl.getSkillMetadata(path)
		}
	}

	return nil
}
