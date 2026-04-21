package kanban

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/template"
	"time"

	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
)

// AgentTemplate represents the template for creating an agent
type AgentTemplate struct {
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Prompt      string            `json:"prompt"`
	SystemRole  string            `json:"system_role"`
	Parameters  map[string]string `json:"parameters"`
}

// TemplateLoader handles loading and caching of agent templates
type TemplateLoader struct {
	templateDir    string
	cache          map[string]*CachedTemplate
	mu             sync.RWMutex
	reloadInterval time.Duration
	lastReload     time.Time
}

// CachedTemplate holds a loaded template with metadata
type CachedTemplate struct {
	Template   *AgentTemplate
	LoadedAt   time.Time
	ZoneID     string
	RawContent string
}

// TemplateContext holds the context data for template rendering
type TemplateContext struct {
	ZoneName     string
	ZoneID       string
	TaskContext  string
	GlobalConfig *config.Config
	AgentID      string
	Timestamp    int64
	CustomData   map[string]string
}

// NewTemplateLoader creates a new template loader
func NewTemplateLoader(templateDir string, reloadInterval time.Duration) *TemplateLoader {
	return &TemplateLoader{
		templateDir:    templateDir,
		cache:          make(map[string]*CachedTemplate),
		reloadInterval: reloadInterval,
		lastReload:     time.Now(),
	}
}

// LoadTemplate loads a template from file or cache
func (l *TemplateLoader) LoadTemplate(zoneID string) (*AgentTemplate, error) {
	l.mu.RLock()
	cached, exists := l.cache[zoneID]
	l.mu.RUnlock()

	// Check if we should reload based on interval
	shouldReload := time.Since(l.lastReload) > l.reloadInterval

	if exists && !shouldReload {
		logger.DebugCF("template_loader", "Using cached template",
			map[string]any{"zone_id": zoneID})
		return cached.Template, nil
	}

	// Load from file system
	templatePath := filepath.Join(l.templateDir, fmt.Sprintf("%s.md", zoneID))

	// Try default template if zone-specific doesn't exist
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		templatePath = filepath.Join(l.templateDir, "agent.md")
	}

	logger.InfoCF("template_loader", "Loading template from file",
		map[string]any{"path": templatePath, "zone_id": zoneID})

	content, err := os.ReadFile(templatePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read template file: %w", err)
	}

	// Parse the template
	tmpl, err := l.parseTemplate(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	// Cache the result
	l.mu.Lock()
	l.cache[zoneID] = &CachedTemplate{
		Template:   tmpl,
		LoadedAt:   time.Now(),
		ZoneID:     zoneID,
		RawContent: string(content),
	}
	l.lastReload = time.Now()
	l.mu.Unlock()

	logger.InfoCF("template_loader", "Template loaded successfully",
		map[string]any{"zone_id": zoneID, "name": tmpl.Name})

	return tmpl, nil
}

// parseTemplate parses raw template content into an AgentTemplate
func (l *TemplateLoader) parseTemplate(content string) (*AgentTemplate, error) {
	// Simple parsing strategy: look for YAML frontmatter or markdown headers
	// Format expected:
	// ---
	// name: Agent Name
	// description: Description here
	// system_role: You are a helpful assistant
	// ---
	//
	// # Prompt
	// Your prompt template here...

	lines := bytes.Split([]byte(content), []byte("\n"))
	tmpl := &AgentTemplate{
		Parameters: make(map[string]string),
	}

	inFrontmatter := false
	frontmatterEnded := false
	promptLines := []string{}
	currentSection := ""

	for i, line := range lines {
		trimmed := bytes.TrimSpace(line)

		// Check for frontmatter delimiters
		if bytes.Equal(trimmed, []byte("---")) {
			if !inFrontmatter && i == 0 {
				inFrontmatter = true
				continue
			} else if inFrontmatter {
				frontmatterEnded = true
				continue
			}
		}

		if inFrontmatter {
			// Parse key: value pairs
			parts := bytes.SplitN(trimmed, []byte(":"), 2)
			if len(parts) == 2 {
				key := string(bytes.TrimSpace(parts[0]))
				value := string(bytes.TrimSpace(parts[1]))

				switch key {
				case "name":
					tmpl.Name = value
				case "description":
					tmpl.Description = value
				case "system_role":
					tmpl.SystemRole = value
				default:
					tmpl.Parameters[key] = value
				}
			}
		} else if frontmatterEnded {
			// Parse markdown sections
			if bytes.HasPrefix(trimmed, []byte("# ")) {
				currentSection = string(bytes.TrimPrefix(trimmed, []byte("# ")))
				continue
			}

			if currentSection == "Prompt" || currentSection == "prompt" {
				promptLines = append(promptLines, string(line))
			}
		}
	}

	tmpl.Prompt = joinLines(promptLines)

	// Validate required fields
	if tmpl.Name == "" {
		tmpl.Name = "Default Agent"
	}

	if tmpl.Prompt == "" {
		return nil, fmt.Errorf("template must have a prompt section")
	}

	return tmpl, nil
}

// RenderTemplate renders a template with the given context
func (l *TemplateLoader) RenderTemplate(tmpl *AgentTemplate, ctx *TemplateContext) (string, error) {
	if tmpl.Prompt == "" {
		return "", fmt.Errorf("empty template prompt")
	}

	// Create a text template from the prompt
	t, err := template.New("prompt").Parse(tmpl.Prompt)
	if err != nil {
		return "", fmt.Errorf("failed to parse prompt template: %w", err)
	}

	var buf bytes.Buffer
	err = t.Execute(&buf, ctx)
	if err != nil {
		return "", fmt.Errorf("failed to execute template: %w", err)
	}

	return buf.String(), nil
}

// ReloadCache forces a cache reload
func (l *TemplateLoader) ReloadCache() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	logger.InfoCF("template_loader", "Forcing cache reload", nil)

	// Clear cache
	l.cache = make(map[string]*CachedTemplate)
	l.lastReload = time.Time{}

	return nil
}

// HotReload watches for file changes and reloads templates
func (l *TemplateLoader) HotReload(ctx context.Context) {
	ticker := time.NewTicker(l.reloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.InfoCF("template_loader", "Stopping hot reload watcher", nil)
			return
		case <-ticker.C:
			l.checkForChanges()
		}
	}
}

// checkForChanges checks if template files have been modified
func (l *TemplateLoader) checkForChanges() {
	l.mu.RLock()
	cachedZones := make([]string, 0, len(l.cache))
	for zoneID := range l.cache {
		cachedZones = append(cachedZones, zoneID)
	}
	l.mu.RUnlock()

	for _, zoneID := range cachedZones {
		templatePath := filepath.Join(l.templateDir, fmt.Sprintf("%s.md", zoneID))
		if _, err := os.Stat(templatePath); os.IsNotExist(err) {
			templatePath = filepath.Join(l.templateDir, "agent.md")
		}

		info, err := os.Stat(templatePath)
		if err != nil {
			continue
		}

		l.mu.RLock()
		cached := l.cache[zoneID]
		l.mu.RUnlock()

		if cached != nil && info.ModTime().After(cached.LoadedAt) {
			logger.InfoCF("template_loader", "Template file changed, reloading",
				map[string]any{"zone_id": zoneID, "path": templatePath})

			// Remove from cache to force reload on next access
			l.mu.Lock()
			delete(l.cache, zoneID)
			l.mu.Unlock()
		}
	}
}

// GetCacheStats returns cache statistics
func (l *TemplateLoader) GetCacheStats() map[string]any {
	l.mu.RLock()
	defer l.mu.RUnlock()

	stats := map[string]any{
		"cached_templates": len(l.cache),
		"last_reload":      l.lastReload.Format(time.RFC3339),
		"zones":            []string{},
	}

	zones := make([]string, 0, len(l.cache))
	for zoneID := range l.cache {
		zones = append(zones, zoneID)
	}
	stats["zones"] = zones

	return stats
}

// ClearCache clears all cached templates
func (l *TemplateLoader) ClearCache() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.cache = make(map[string]*CachedTemplate)
	logger.InfoCF("template_loader", "Cache cleared", nil)
}

// joinLines joins lines into a single string
func joinLines(lines []string) string {
	var result string
	for i, line := range lines {
		if i > 0 {
			result += "\n"
		}
		result += line
	}
	return result
}

// LoadTemplateWithInjection loads and injects context into a template
func (l *TemplateLoader) LoadTemplateWithInjection(zoneID, taskContext string, cfg *config.Config, agentID string) (string, error) {
	tmpl, err := l.LoadTemplate(zoneID)
	if err != nil {
		return "", err
	}

	ctx := &TemplateContext{
		ZoneID:       zoneID,
		ZoneName:     zoneID,
		TaskContext:  taskContext,
		GlobalConfig: cfg,
		AgentID:      agentID,
		Timestamp:    time.Now().Unix(),
		CustomData:   make(map[string]string),
	}

	return l.RenderTemplate(tmpl, ctx)
}
