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

	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
)

// AgentScanner scans the agents directory for main.md files and extracts agent metadata
type AgentScanner struct {
	agentsDir string
}

// AgentMetadata represents metadata extracted from a main.md file
type AgentMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Directory   string `json:"directory"`
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
		mainMdPath := filepath.Join(dirPath, "main.md")

		// Check if main.md exists
		if _, err := os.Stat(mainMdPath); os.IsNotExist(err) {
			continue
		}

		// Extract metadata from main.md
		metadata, err := s.extractMetadata(mainMdPath)
		if err != nil {
			logger.WarnCF("agent_scanner", "Failed to extract metadata from main.md",
				map[string]any{"path": mainMdPath, "error": err.Error()})
			continue
		}

		metadata.Path = mainMdPath
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

// extractMetadata extracts name and description from a main.md file
func (s *AgentScanner) extractMetadata(path string) (*AgentMetadata, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	metadata := &AgentMetadata{}

	// Parse markdown to extract title and first paragraph
	p := parser.NewWithExtensions(parser.CommonExtensions)
	doc := markdown.Parse(content, p)
	if doc == nil {
		return nil, fmt.Errorf("failed to parse markdown")
	}

	var title, description string
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

	// Use directory name as fallback for name
	dirName := filepath.Base(filepath.Dir(path))
	if title == "" {
		title = dirName
	}

	// If no description found, use a portion of the content
	if description == "" {
		description = extractFirstParagraph(string(content))
	}

	metadata.Name = title
	metadata.Description = description

	return metadata, nil
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
// Agents without model configuration are treated as skill-based agents
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

		// Create agent config - no model specified means it will use skill-based execution
		cfg := config.AgentConfig{
			ID:      agentID,
			Name:    agent.Name,
			Default: false,
			// Model is intentionally left nil - this indicates skill-based agent
			// The agent will be created dynamically when needed
		}

		configs = append(configs, cfg)
		logger.InfoCF("agent_scanner", "Added auto-scanned agent config",
			map[string]any{
				"agent_id":    agentID,
				"name":        agent.Name,
				"description": truncateString(agent.Description, 100),
				"path":        agent.Path,
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

// LoadAgentPrompt loads the prompt from main.md file for agent execution
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
