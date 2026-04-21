package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ilibx/octopus/pkg/logger"
)

// TokenEstimator handles token counting and context truncation
type TokenEstimator struct {
	mu                sync.RWMutex
	warningThreshold  float64 // 0.8 = 80%
	criticalThreshold float64 // 0.95 = 95%
	avgCharsPerToken  int     // Average characters per token (typically 4 for English, 2 for Chinese)
}

// TokenEstimateResult contains the result of token estimation
type TokenEstimateResult struct {
	EstimatedTokens int
	MaxTokens       int
	UsageRatio      float64
	NeedsTruncation bool
	Mode            string // "normal", "warning", "critical"
}

// NewTokenEstimator creates a new token estimator
func NewTokenEstimator(warningThreshold, criticalThreshold float64, avgCharsPerToken int) *TokenEstimator {
	if warningThreshold <= 0 || warningThreshold >= 1 {
		warningThreshold = 0.8
	}
	if criticalThreshold <= warningThreshold || criticalThreshold > 1 {
		criticalThreshold = 0.95
	}
	if avgCharsPerToken <= 0 {
		avgCharsPerToken = 4 // Default for English
	}

	return &TokenEstimator{
		warningThreshold:  warningThreshold,
		criticalThreshold: criticalThreshold,
		avgCharsPerToken:  avgCharsPerToken,
	}
}

// EstimateTokens estimates the number of tokens in a text
func (te *TokenEstimator) EstimateTokens(text string) int {
	te.mu.RLock()
	defer te.mu.RUnlock()

	// Simple estimation: character count / avg chars per token
	// For production, use tiktoken or similar library
	charCount := len([]rune(text))
	return charCount / te.avgCharsPerToken
}

// EstimateAndTruncate checks token usage and truncates if necessary
// Returns the (possibly truncated) prompt and whether truncation occurred
func (te *TokenEstimator) EstimateAndTruncate(prompt string, maxTokens int) (string, bool) {
	estimated := te.EstimateTokens(prompt)
	usageRatio := float64(estimated) / float64(maxTokens)

	te.mu.RLock()
	warningThreshold := te.warningThreshold
	criticalThreshold := te.criticalThreshold
	te.mu.RUnlock()

	result := &TokenEstimateResult{
		EstimatedTokens: estimated,
		MaxTokens:       maxTokens,
		UsageRatio:      usageRatio,
	}

	if usageRatio < warningThreshold {
		result.NeedsTruncation = false
		result.Mode = "normal"
		logger.DebugCF("token_estimator", "Token usage within normal range",
			map[string]any{
				"estimated": estimated,
				"max":       maxTokens,
				"ratio":     fmt.Sprintf("%.2f%%", usageRatio*100),
			})
		return prompt, false
	}

	if usageRatio >= criticalThreshold {
		// Critical: switch to schema-only mode
		result.NeedsTruncation = true
		result.Mode = "critical"
		truncated := te.extractSchemaOnly(prompt)
		logger.WarnCF("token_estimator", "Token usage critical - switched to schema-only mode",
			map[string]any{
				"original":  estimated,
				"truncated": te.EstimateTokens(truncated),
				"max":       maxTokens,
				"ratio":     fmt.Sprintf("%.2f%%", usageRatio*100),
			})
		return truncated, true
	}

	// Warning: compress history
	result.NeedsTruncation = true
	result.Mode = "warning"
	truncated := te.compressHistory(prompt)
	logger.WarnCF("token_estimator", "Token usage high - compressed history",
		map[string]any{
			"original":  estimated,
			"truncated": te.EstimateTokens(truncated),
			"max":       maxTokens,
			"ratio":     fmt.Sprintf("%.2f%%", usageRatio*100),
		})
	return truncated, true
}

// extractSchemaOnly extracts only the core schema from a prompt
// This is used in critical mode when context window is nearly full
func (te *TokenEstimator) extractSchemaOnly(prompt string) string {
	// Look for JSON schema markers
	schemaStart := strings.Index(prompt, "{")
	schemaEnd := strings.LastIndex(prompt, "}")

	if schemaStart == -1 || schemaEnd == -1 || schemaEnd <= schemaStart {
		// No schema found, return minimal instruction
		return "Execute task and output JSON with status, result, and error_message fields."
	}

	// Extract schema section
	schema := prompt[schemaStart : schemaEnd+1]

	// Add minimal instruction
	return fmt.Sprintf("Output strict JSON matching this schema:\n%s", schema)
}

// compressHistory compresses conversation history by summarizing older messages
func (te *TokenEstimator) compressHistory(prompt string) string {
	// Strategy: Keep recent content, summarize older sections
	// Look for common section markers

	sections := []string{
		"# 🎯 角色定义与职责边界",
		"# 🔧 SKILL 调用协议",
		"# 🔄 标准执行流程",
		"# 🛡️ 约束与熔断机制",
		"# 📝 输出规范",
	}

	lines := strings.Split(prompt, "\n")
	var keptLines []string
	inCompressibleSection := false
	sectionDepth := 0

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check if entering a compressible section
		for _, section := range sections[1:] { // Skip first section (keep role definition)
			if strings.HasPrefix(trimmed, section) {
				inCompressibleSection = true
				sectionDepth++
				// Keep section header but add compression note
				keptLines = append(keptLines, fmt.Sprintf("%s [已压缩]", section))
				continue
			}
		}

		if inCompressibleSection {
			// Check if exiting section (new top-level header)
			if strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "##") {
				inCompressibleSection = false
				sectionDepth = 0
			} else if sectionDepth > 0 && strings.HasPrefix(trimmed, "##") {
				// Keep subsection headers
				keptLines = append(keptLines, line)
			} else {
				// Skip detailed content in compressible sections
				continue
			}
		}

		keptLines = append(keptLines, line)
	}

	return strings.Join(keptLines, "\n")
}

// GetThresholds returns the current threshold settings
func (te *TokenEstimator) GetThresholds() (warning, critical float64) {
	te.mu.RLock()
	defer te.mu.RUnlock()
	return te.warningThreshold, te.criticalThreshold
}

// SetThresholds updates the threshold settings
func (te *TokenEstimator) SetThresholds(warning, critical float64) error {
	if warning <= 0 || warning >= 1 {
		return fmt.Errorf("warning threshold must be between 0 and 1")
	}
	if critical <= warning || critical > 1 {
		return fmt.Errorf("critical threshold must be greater than warning and <= 1")
	}

	te.mu.Lock()
	te.warningThreshold = warning
	te.criticalThreshold = critical
	te.mu.Unlock()

	logger.InfoCF("token_estimator", "Thresholds updated",
		map[string]any{
			"warning":  warning,
			"critical": critical,
		})
	return nil
}

// SetAvgCharsPerToken updates the average characters per token estimate
func (te *TokenEstimator) SetAvgCharsPerToken(chars int) {
	if chars <= 0 {
		return
	}
	te.mu.Lock()
	te.avgCharsPerToken = chars
	te.mu.Unlock()
}

// GetStats returns statistics about token estimation
func (te *TokenEstimator) GetStats() map[string]interface{} {
	te.mu.RLock()
	defer te.mu.RUnlock()

	return map[string]interface{}{
		"warning_threshold":   te.warningThreshold,
		"critical_threshold":  te.criticalThreshold,
		"avg_chars_per_token": te.avgCharsPerToken,
	}
}
