package scanner

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/gomarkdown/markdown"
	"github.com/gomarkdown/markdown/ast"
	"github.com/gomarkdown/markdown/parser"
	"gopkg.in/yaml.v3"

	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
)

// AgentScanner scans the agents directory for AGENT.md files and extracts agent metadata
type AgentScanner struct {
	agentsDir string
}

// AgentMetadata represents metadata extracted from an AGENT.md file
type AgentMetadata struct {
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Model       *config.AgentModelConfig `json:"model,omitempty"`
	Path        string              `json:"path"`
	Directory   string              `json:"directory"`
}

// NewAgentScanner creates a new agent scanner
func NewAgentScanner(agentsDir string) *AgentScanner {
	return &AgentScanner{
		agentsDir: agentsDir,
	}
}

// ScanAgents scans the agents directory for all agent directories containing main.md
func (s *AgentScanner) ScanAgents() ([]AgentMetadata, error) {
	if s.agentsDir == "" {
		return nil, fmt.Errorf("agents directory not specified")
	}

	// Check if agents directory exists
	if _, err := os.Stat(s.agentsDir); os.IsNotExist(err) {
		logger.InfoCF("agent_scanner", "Agents directory does not exist, skipping scan",
			map[string]any{"path": s.agentsDir})
		return []AgentMetadata{}, nil
	}

	entries, err := os.ReadDir(s.agentsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read agents directory: %w", err)
	}

	var agents []AgentMetadata
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(s.agentsDir, entry.Name())
		// Support both AGENT.md (new standard) and main.md (legacy)
		agentMdPath := filepath.Join(dirPath, "AGENT.md")
		mainMdPath := filepath.Join(dirPath, "main.md")
		
		var agentFilePath string
		if _, err := os.Stat(agentMdPath); err == nil {
			agentFilePath = agentMdPath
		} else if _, err := os.Stat(mainMdPath); err == nil {
			agentFilePath = mainMdPath
		} else {
			continue
		}

		// Extract metadata from agent file
		metadata, err := s.extractMetadata(agentFilePath)
		if err != nil {
			logger.WarnCF("agent_scanner", "Failed to extract metadata from agent file",
				map[string]any{"path": agentFilePath, "error": err.Error()})
			continue
		}

		metadata.Path = agentFilePath
		metadata.Directory = dirPath
		agents = append(agents, *metadata)
	}

	logger.InfoCF("agent_scanner", "Scanned agents directory",
		map[string]any{
			"path":   s.agentsDir,
			"count":  len(agents),
			"agents": getAgentNames(agents),
		})

	return agents, nil
}

// extractMetadata extracts name, description and model from an agent file
func (s *AgentScanner) extractMetadata(path string) (*AgentMetadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	metadata := &AgentMetadata{}

	// Parse YAML frontmatter
	frontmatter, bodyContent := splitFrontmatter(string(content))
	
	// Extract metadata from frontmatter if present
	if frontmatter != "" {
		var fmData struct {
			Name        string              `yaml:"name"`
			Description string              `yaml:"description"`
			Model       interface{}         `yaml:"model,omitempty"`
		}
		if err := yaml.Unmarshal([]byte(frontmatter), &fmData); err == nil {
			if fmData.Name != "" {
				metadata.Name = fmData.Name
			}
			if fmData.Description != "" {
				metadata.Description = fmData.Description
			}
			// Parse model config (supports both string and object format)
			if fmData.Model != nil {
				modelConfig, err := parseModelConfig(fmData.Model)
				if err == nil && modelConfig != nil {
					metadata.Model = modelConfig
				}
			}
		}
	}

	// Use directory name as fallback for name
	dirName := filepath.Base(filepath.Dir(path))
	if metadata.Name == "" {
		metadata.Name = dirName
	}

	// If no description found, extract from markdown body
	if metadata.Description == "" {
		directory := filepath.Base(filepath.Dir(path))
		title, bodyDescription := extractMarkdownMetadata(bodyContent)
		if title != "" && metadata.Name == directory {
			metadata.Name = title
		}
		if bodyDescription != "" {
			metadata.Description = bodyDescription
		} else {
			metadata.Description = extractFirstParagraph(string(content))
		}
	}

	return metadata, nil
}

// parseModelConfig parses model configuration from YAML frontmatter
// Supports both string format ("openai/gpt-4o") and object format ({primary: "openai/gpt-4o", fallbacks: [...]})
func parseModelConfig(modelData interface{}) (*config.AgentModelConfig, error) {
	switch v := modelData.(type) {
	case string:
		// String format: just the primary model name
		if v == "" {
			return nil, nil
		}
		return &config.AgentModelConfig{
			Primary:   v,
			Fallbacks: nil,
		}, nil
	case map[string]interface{}:
		// Object format: {primary: "...", fallbacks: [...]}
		modelConfig := &config.AgentModelConfig{}
		if primary, ok := v["primary"].(string); ok {
			modelConfig.Primary = primary
		}
		if fallbacks, ok := v["fallbacks"].([]interface{}); ok {
			for _, f := range fallbacks {
				if fs, ok := f.(string); ok {
					modelConfig.Fallbacks = append(modelConfig.Fallbacks, fs)
				}
			}
		}
		if modelConfig.Primary == "" {
			return nil, nil
		}
		return modelConfig, nil
	default:
		return nil, fmt.Errorf("unsupported model format: %T", modelData)
	}
}

// splitFrontmatter splits markdown content into frontmatter and body
func splitFrontmatter(content string) (frontmatter, body string) {
	normalized := string(parser.NormalizeNewlines([]byte(content)))
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return "", content
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if lines[i] == "---" {
			end = i
			break
		}
	}

	if end == -1 {
		return "", content
	}

	frontmatter = strings.Join(lines[1:end], "\n")
	body = strings.Join(lines[end+1:], "\n")
	return frontmatter, strings.TrimLeft(body, "\n")
}

// extractMarkdownMetadata extracts title and first paragraph from markdown body
func extractMarkdownMetadata(content string) (title, description string) {
	p := parser.NewWithExtensions(parser.CommonExtensions)
	doc := markdown.Parse([]byte(content), p)
	if doc == nil {
		return "", ""
	}

	ast.WalkFunc(doc, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch n := node.(type) {
		case *ast.Heading:
			if title == "" && n.Level == 1 {
				title = nodeText(n)
			}
		case *ast.Paragraph:
			if description == "" {
				text := nodeText(n)
				// Skip if it looks like a role definition or too short
				if text != "" && len(text) > 20 && !strings.HasPrefix(strings.ToLower(text), "role") {
					description = text
				}
			}
		}

		if title != "" && description != "" {
			return ast.Terminate
		}

		return ast.GoToNext
	})

	return title, description
}

// nodeText extracts text content from an AST node
func nodeText(n ast.Node) string {
	var b strings.Builder
	ast.WalkFunc(n, func(node ast.Node, entering bool) ast.WalkStatus {
		if !entering {
			return ast.GoToNext
		}

		switch t := node.(type) {
		case *ast.Text:
			b.Write(t.Literal)
		case *ast.Code:
			b.Write(t.Literal)
		case *ast.Softbreak, *ast.Hardbreak, *ast.NonBlockingSpace:
			b.WriteByte(' ')
		}
		return ast.GoToNext
	})
	return strings.TrimSpace(b.String())
}

// extractFirstParagraph extracts the first paragraph from markdown content
func extractFirstParagraph(content string) string {
	lines := strings.Split(content, "\n")
	var paragraphLines []string
	inParagraph := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip frontmatter
		if trimmed == "---" {
			continue
		}

		// Skip headings
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Skip empty lines at the beginning
		if trimmed == "" && !inParagraph {
			continue
		}

		// End of paragraph
		if trimmed == "" && inParagraph {
			break
		}

		inParagraph = true
		paragraphLines = append(paragraphLines, trimmed)
	}

	paragraph := strings.Join(paragraphLines, " ")
	// Limit description length
	if len(paragraph) > 500 {
		paragraph = paragraph[:500] + "..."
	}

	return paragraph
}

// getAgentNames returns a list of agent names for logging
func getAgentNames(agents []AgentMetadata) []string {
	names := make([]string, len(agents))
	for i, a := range agents {
		names[i] = a.Name
	}
	return names
}

// BuildAgentConfigsFromScannedAgents builds agent configurations from scanned agents
// Agents with model configuration use the specified model; others use default model
func BuildAgentConfigsFromScannedAgents(scannedAgents []AgentMetadata, existingConfigs []config.AgentConfig) []config.AgentConfig {
	var configs []config.AgentConfig
	existingIDs := make(map[string]bool)

	// Track existing agent IDs to avoid duplicates
	for _, cfg := range existingConfigs {
		existingIDs[strings.ToLower(cfg.ID)] = true
		configs = append(configs, cfg)
	}

	// Create agent configs from scanned agents
	for _, agent := range scannedAgents {
		// Generate agent ID from directory name
		agentID := generateAgentID(agent.Directory)

		// Skip if already exists in config
		if existingIDs[strings.ToLower(agentID)] {
			logger.DebugCF("agent_scanner", "Agent already configured, skipping",
				map[string]any{"agent_id": agentID, "name": agent.Name})
			continue
		}

		// Create agent config with model if specified in frontmatter
		cfg := config.AgentConfig{
			ID:      agentID,
			Name:    agent.Name,
			Default: false,
			Model:   agent.Model, // Will be nil if not specified in frontmatter
		}

		configs = append(configs, cfg)
		
		modelInfo := "default model"
		if agent.Model != nil {
			modelInfo = agent.Model.Primary
			if len(agent.Model.Fallbacks) > 0 {
				modelInfo += fmt.Sprintf(" (+%d fallbacks)", len(agent.Model.Fallbacks))
			}
		}
		
		logger.InfoCF("agent_scanner", "Added auto-scanned agent config",
			map[string]any{
				"agent_id":    agentID,
				"name":        agent.Name,
				"description": truncateString(agent.Description, 100),
				"path":        agent.Path,
				"model":       modelInfo,
			})
	}

	return configs
}

// generateAgentID generates a normalized agent ID from a directory path
func generateAgentID(dirPath string) string {
	dirName := filepath.Base(dirPath)
	// Normalize: lowercase, replace spaces/special chars with hyphens
	re := regexp.MustCompile(`[^a-zA-Z0-9]+`)
	id := re.ReplaceAllString(strings.ToLower(dirName), "-")
	id = strings.Trim(id, "-")
	if id == "" {
		id = "agent"
	}
	return id
}

// truncateString truncates a string to maxLen characters
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// LoadAgentPrompt loads the system prompt from agent file (AGENT.md or main.md) for agent execution.
// The entire markdown body (after frontmatter) is used as the system prompt.
func LoadAgentPrompt(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read agent prompt: %w", err)
	}

	// Remove YAML frontmatter if present
	contentStr := string(content)
	if strings.HasPrefix(contentStr, "---") {
		parts := strings.SplitN(contentStr[4:], "---", 2)
		if len(parts) == 2 {
			contentStr = strings.TrimLeft(parts[1], "\n")
		}
	}

	return contentStr, nil
}

// IsSkillBasedAgent checks if an agent config represents a skill-based agent
// (i.e., no model specified)
func IsSkillBasedAgent(cfg *config.AgentConfig) bool {
	return cfg.Model == nil || cfg.Model.Primary == ""
}
