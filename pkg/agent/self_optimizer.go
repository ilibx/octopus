// Package agent provides self-optimization capabilities for the multi-agent system
// SelfOptimizer analyzes execution logs and performance metrics to suggest or apply
// safe optimizations to agent prompts, SKILL configurations, and execution policies
package agent

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/logger"
	"github.com/ilibx/octopus/pkg/observability"
)

// OptimizationType represents the type of optimization
type OptimizationType string

const (
	OptimizePrompt     OptimizationType = "prompt"
	OptimizeSkill      OptimizationType = "skill"
	OptimizePolicy     OptimizationType = "policy"
	OptimizeRetry      OptimizationType = "retry"
	OptimizeTimeout    OptimizationType = "timeout"
	OptimizeMaxSteps   OptimizationType = "max_steps"
)

// ConfidenceLevel represents the confidence level of an optimization suggestion
type ConfidenceLevel string

const (
	ConfidenceHigh   ConfidenceLevel = "high"
	ConfidenceMedium ConfidenceLevel = "medium"
	ConfidenceLow    ConfidenceLevel = "low"
)

// OptimizationSuggestion represents a single optimization recommendation
type OptimizationSuggestion struct {
	ID              string            `json:"id"`
	Type            OptimizationType  `json:"type"`
	AgentType       string            `json:"agent_type"`
	Description     string            `json:"description"`
	CurrentValue    string            `json:"current_value"`
	SuggestedValue  string            `json:"suggested_value"`
	Reason          string            `json:"reason"`
	Evidence        []string          `json:"evidence"`
	Confidence      ConfidenceLevel   `json:"confidence"`
	ImpactScore     float64           `json:"impact_score"` // 0.0 - 1.0
	RiskLevel       string            `json:"risk_level"`  // "low", "medium", "high"
	CreatedAt       time.Time         `json:"created_at"`
	Metrics         map[string]float64 `json:"metrics,omitempty"`
}

// ExecutionRecord represents a single task execution record for analysis
type ExecutionRecord struct {
	TaskID        string                 `json:"task_id"`
	AgentType     string                 `json:"agent_type"`
	Status        string                 `json:"status"`
	Duration      time.Duration          `json:"duration"`
	Error         string                 `json:"error,omitempty"`
	RetryCount    int                    `json:"retry_count"`
	StepsExecuted int                    `json:"steps_executed"`
	TokensUsed    int                    `json:"tokens_used"`
	Timestamp     time.Time              `json:"timestamp"`
	Metadata      map[string]interface{} `json:"metadata,omitempty"`
}

// PatternMatch represents a detected pattern in execution logs
type PatternMatch struct {
	Pattern     string    `json:"pattern"`
	Description string    `json:"description"`
	Occurrences int       `json:"occurrences"`
	Examples    []string  `json:"examples"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// SelfOptimizerConfig holds configuration for the self-optimizer
type SelfOptimizerConfig struct {
	AnalysisWindow      time.Duration // Time window for analysis
	MinSamples          int           // Minimum samples required for analysis
	ConfidenceThreshold float64       // Minimum confidence to generate suggestion
	AutoApplyEnabled    bool          // Enable automatic application of low-risk optimizations
	MaxSuggestions      int           // Maximum pending suggestions
	LogRetentionDays    int           // Days to retain execution logs
}

// DefaultSelfOptimizerConfig returns default configuration
func DefaultSelfOptimizerConfig() SelfOptimizerConfig {
	return SelfOptimizerConfig{
		AnalysisWindow:      24 * time.Hour,
		MinSamples:          10,
		ConfidenceThreshold: 0.7,
		AutoApplyEnabled:    false,
		MaxSuggestions:      50,
		LogRetentionDays:    7,
	}
}

// SelfOptimizer analyzes execution patterns and suggests optimizations
type SelfOptimizer struct {
	mu               sync.RWMutex
	config           SelfOptimizerConfig
	executionLogs    []ExecutionRecord
	suggestions      []*OptimizationSuggestion
	appliedChanges   []*OptimizationSuggestion
	patternDetectors []*patternDetector
	metricsCollector *observability.Metrics
	lastAnalysisTime time.Time
	running          bool
	stopCh           chan struct{}
}

// patternDetector detects specific patterns in execution logs
type patternDetector struct {
	name        string
	pattern     *regexp.Regexp
	description string
	detectFunc  func([]ExecutionRecord) []PatternMatch
}

// NewSelfOptimizer creates a new self-optimizer instance
func NewSelfOptimizer(config SelfOptimizerConfig) *SelfOptimizer {
	if config.MinSamples == 0 {
		config.MinSamples = 10
	}
	if config.ConfidenceThreshold == 0 {
		config.ConfidenceThreshold = 0.7
	}
	if config.MaxSuggestions == 0 {
		config.MaxSuggestions = 50
	}

	so := &SelfOptimizer{
		config:         config,
		executionLogs:  make([]ExecutionRecord, 0),
		suggestions:    make([]*OptimizationSuggestion, 0),
		appliedChanges: make([]*OptimizationSuggestion, 0),
		stopCh:         make(chan struct{}),
	}

	// Initialize pattern detectors
	so.initializePatternDetectors()

	return so
}

// initializePatternDetectors sets up pattern detection rules
func (so *SelfOptimizer) initializePatternDetectors() {
	so.patternDetectors = []*patternDetector{
		{
			name:        "repeated_failures",
			description: "Detects repeated failures for same task type",
			detectFunc:  so.detectRepeatedFailures,
		},
		{
			name:        "timeout_patterns",
			description: "Detects tasks consistently hitting timeout limits",
			detectFunc:  so.detectTimeoutPatterns,
		},
		{
			name:        "excessive_retries",
			description: "Detects tasks requiring excessive retries",
			detectFunc:  so.detectExcessiveRetries,
		},
		{
			name:        "step_limit_issues",
			description: "Detects tasks hitting max step limits",
			detectFunc:  so.detectStepLimitIssues,
		},
		{
			name:        "error_message_patterns",
			description: "Detects common error message patterns",
			detectFunc:  so.detectErrorMessagePatterns,
		},
	}
}

// RecordExecution records a task execution for later analysis
func (so *SelfOptimizer) RecordExecution(record ExecutionRecord) {
	so.mu.Lock()
	defer so.mu.Unlock()

	so.executionLogs = append(so.executionLogs, record)

	// Trim old logs based on retention policy
	so.trimOldLogs()
}

// trimOldLogs removes logs older than retention period
func (so *SelfOptimizer) trimOldLogs() {
	cutoff := time.Now().AddDate(0, 0, -so.config.LogRetentionDays)
	
	validLogs := make([]ExecutionRecord, 0)
	for _, log := range so.executionLogs {
		if log.Timestamp.After(cutoff) {
			validLogs = append(validLogs, log)
		}
	}
	
	// Keep only recent logs within analysis window as well
	windowStart := time.Now().Add(-so.config.AnalysisWindow)
	filteredLogs := make([]ExecutionRecord, 0)
	for _, log := range validLogs {
		if log.Timestamp.After(windowStart) {
			filteredLogs = append(filteredLogs, log)
		}
	}
	
	so.executionLogs = filteredLogs
}

// Analyze performs analysis on collected execution logs and generates suggestions
func (so *SelfOptimizer) Analyze(ctx context.Context) []*OptimizationSuggestion {
	so.mu.Lock()
	defer so.mu.Unlock()

	logger.InfoCF("self_optimizer", "Starting analysis",
		map[string]any{
			"log_count": len(so.executionLogs),
			"window":    so.config.AnalysisWindow.String(),
		})

	so.lastAnalysisTime = time.Now()
	newSuggestions := make([]*OptimizationSuggestion, 0)

	// Check if we have enough samples
	if len(so.executionLogs) < so.config.MinSamples {
		logger.WarnCF("self_optimizer", "Insufficient samples for analysis",
			map[string]any{
				"current":  len(so.executionLogs),
				"required": so.config.MinSamples,
			})
		return newSuggestions
	}

	// Run all pattern detectors
	for _, detector := range so.patternDetectors {
		select {
		case <-ctx.Done():
			return newSuggestions
		default:
		}

		patterns := detector.detectFunc(so.executionLogs)
		for _, pattern := range patterns {
			suggestion := so.generateSuggestionFromPattern(detector.name, pattern)
			if suggestion != nil && suggestion.Confidence >= so.getConfidenceLevel(so.config.ConfidenceThreshold) {
				newSuggestions = append(newSuggestions, suggestion)
			}
		}
	}

	// Also analyze metrics if available
	if so.metricsCollector != nil {
		metricSuggestions := so.analyzeMetrics()
		newSuggestions = append(newSuggestions, metricSuggestions...)
	}

	// Sort suggestions by impact score
	sort.Slice(newSuggestions, func(i, j int) bool {
		return newSuggestions[i].ImpactScore > newSuggestions[j].ImpactScore
	})

	// Limit number of suggestions
	if len(newSuggestions) > so.config.MaxSuggestions {
		newSuggestions = newSuggestions[:so.config.MaxSuggestions]
	}

	so.suggestions = append(so.suggestions, newSuggestions...)

	logger.InfoCF("self_optimizer", "Analysis complete",
		map[string]any{
			"suggestions_generated": len(newSuggestions),
			"total_pending":         len(so.suggestions),
		})

	return newSuggestions
}

// generateSuggestionFromPattern creates an optimization suggestion from a detected pattern
func (so *SelfOptimizer) generateSuggestionFromPattern(detectorName string, pattern PatternMatch) *OptimizationSuggestion {
	var optType OptimizationType
	var description string
	var suggestedValue string
	var riskLevel string
	var impactScore float64
	var confidence ConfidenceLevel

	switch detectorName {
	case "repeated_failures":
		optType = OptimizeSkill
		description = fmt.Sprintf("High failure rate detected for pattern: %s", pattern.Pattern)
		suggestedValue = "Review and update SKILL implementation or add error handling"
		riskLevel = "medium"
		impactScore = 0.8
		confidence = ConfidenceHigh
	case "timeout_patterns":
		optType = OptimizeTimeout
		description = fmt.Sprintf("Tasks consistently timing out: %s", pattern.Description)
		suggestedValue = "Increase timeout limit by 50% or optimize SKILL execution"
		riskLevel = "low"
		impactScore = 0.6
		confidence = ConfidenceHigh
	case "excessive_retries":
		optType = OptimizeRetry
		description = fmt.Sprintf("Excessive retries detected: %s", pattern.Description)
		suggestedValue = "Increase max retries or fix underlying failure cause"
		riskLevel = "low"
		impactScore = 0.5
		confidence = ConfidenceMedium
	case "step_limit_issues":
		optType = OptimizeMaxSteps
		description = fmt.Sprintf("Tasks hitting step limits: %s", pattern.Description)
		suggestedValue = "Increase max_steps or refactor task decomposition"
		riskLevel = "medium"
		impactScore = 0.7
		confidence = ConfidenceHigh
	case "error_message_patterns":
		optType = OptimizePrompt
		description = fmt.Sprintf("Common error pattern: %s", pattern.Pattern)
		suggestedValue = "Update prompt to handle edge cases"
		riskLevel = "low"
		impactScore = 0.4
		confidence = ConfidenceMedium
	default:
		return nil
	}

	return &OptimizationSuggestion{
		ID:             generateSuggestionID(detectorName, pattern.Pattern),
		Type:           optType,
		AgentType:      "auto-detected",
		Description:    description,
		CurrentValue:   fmt.Sprintf("%d occurrences", pattern.Occurrences),
		SuggestedValue: suggestedValue,
		Reason:         pattern.Description,
		Evidence:       pattern.Examples,
		Confidence:     confidence,
		ImpactScore:    impactScore,
		RiskLevel:      riskLevel,
		CreatedAt:      time.Now(),
		Metrics: map[string]float64{
			"occurrences": float64(pattern.Occurrences),
		},
	}
}

// analyzeMetrics analyzes observability metrics for optimization opportunities
func (so *SelfOptimizer) analyzeMetrics() []*OptimizationSuggestion {
	if so.metricsCollector == nil {
		return nil
	}

	suggestions := make([]*OptimizationSuggestion, 0)
	stats := so.metricsCollector.GetAllStats()

	// Check error rate
	if errorRate, ok := stats["error_rate"].(float64); ok && errorRate > 0.3 {
		suggestions = append(suggestions, &OptimizationSuggestion{
			ID:             "high_error_rate",
			Type:           OptimizePrompt,
			Description:    "High overall error rate detected",
			CurrentValue:   fmt.Sprintf("%.2f%%", errorRate*100),
			SuggestedValue: "Review prompt templates and SKILL definitions",
			Reason:         "Error rate exceeds 30% threshold",
			Confidence:     ConfidenceHigh,
			ImpactScore:    0.9,
			RiskLevel:      "medium",
			CreatedAt:      time.Now(),
		})
	}

	// Check P99 latency
	if p99, ok := stats["p99_latency_ms"].(float64); ok && p99 > 10000 {
		suggestions = append(suggestions, &OptimizationSuggestion{
			ID:             "high_latency",
			Type:           OptimizeTimeout,
			Description:    "High P99 latency detected",
			CurrentValue:   fmt.Sprintf("%.0f ms", p99),
			SuggestedValue: "Increase timeout or optimize SKILL chain",
			Reason:         "P99 latency exceeds 10 seconds",
			Confidence:     ConfidenceMedium,
			ImpactScore:    0.6,
			RiskLevel:      "low",
			CreatedAt:      time.Now(),
		})
	}

	return suggestions
}

// GetSuggestions returns all pending optimization suggestions
func (so *SelfOptimizer) GetSuggestions() []*OptimizationSuggestion {
	so.mu.RLock()
	defer so.mu.RUnlock()

	result := make([]*OptimizationSuggestion, len(so.suggestions))
	copy(result, so.suggestions)
	return result
}

// GetSuggestionByID returns a specific suggestion by ID
func (so *SelfOptimizer) GetSuggestionByID(id string) *OptimizationSuggestion {
	so.mu.RLock()
	defer so.mu.RUnlock()

	for _, s := range so.suggestions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// ApplySuggestion applies an optimization suggestion
func (so *SelfOptimizer) ApplySuggestion(ctx context.Context, suggestionID string) error {
	so.mu.Lock()
	defer so.mu.Unlock()

	suggestion := so.getSuggestionByIDUnsafe(suggestionID)
	if suggestion == nil {
		return fmt.Errorf("suggestion not found: %s", suggestionID)
	}

	logger.InfoCF("self_optimizer", "Applying optimization",
		map[string]any{
			"suggestion_id": suggestionID,
			"type":          suggestion.Type,
			"risk_level":    suggestion.RiskLevel,
		})

	// In production, this would integrate with:
	// - Prompt template storage
	// - SKILL configuration management
	// - Agent policy updates
	// For now, we just mark it as applied

	suggestion.CreatedAt = time.Now()
	so.appliedChanges = append(so.appliedChanges, suggestion)

	// Remove from pending suggestions
	filtered := make([]*OptimizationSuggestion, 0)
	for _, s := range so.suggestions {
		if s.ID != suggestionID {
			filtered = append(filtered, s)
		}
	}
	so.suggestions = filtered

	logger.InfoCF("self_optimizer", "Optimization applied successfully",
		map[string]any{
			"suggestion_id": suggestionID,
		})

	return nil
}

// RejectSuggestion rejects an optimization suggestion
func (so *SelfOptimizer) RejectSuggestion(suggestionID string, reason string) error {
	so.mu.Lock()
	defer so.mu.Unlock()

	suggestion := so.getSuggestionByIDUnsafe(suggestionID)
	if suggestion == nil {
		return fmt.Errorf("suggestion not found: %s", suggestionID)
	}

	logger.InfoCF("self_optimizer", "Rejecting optimization",
		map[string]any{
			"suggestion_id": suggestionID,
			"reason":        reason,
		})

	// Remove from pending suggestions
	filtered := make([]*OptimizationSuggestion, 0)
	for _, s := range so.suggestions {
		if s.ID != suggestionID {
			filtered = append(filtered, s)
		}
	}
	so.suggestions = filtered

	return nil
}

// GetAppliedChanges returns all applied optimization changes
func (so *SelfOptimizer) GetAppliedChanges() []*OptimizationSuggestion {
	so.mu.RLock()
	defer so.mu.RUnlock()

	result := make([]*OptimizationSuggestion, len(so.appliedChanges))
	copy(result, so.appliedChanges)
	return result
}

// SetMetricsCollector sets the metrics collector for analysis
func (so *SelfOptimizer) SetMetricsCollector(metrics *observability.Metrics) {
	so.mu.Lock()
	defer so.mu.Unlock()
	so.metricsCollector = metrics
}

// StartBackgroundAnalysis starts periodic background analysis
func (so *SelfOptimizer) StartBackgroundAnalysis(interval time.Duration) {
	so.mu.Lock()
	if so.running {
		so.mu.Unlock()
		return
	}
	so.running = true
	so.mu.Unlock()

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-so.stopCh:
				return
			case <-ticker.C:
				ctx := context.Background()
				so.Analyze(ctx)
			}
		}
	}()

	logger.InfoCF("self_optimizer", "Background analysis started",
		map[string]any{
			"interval": interval.String(),
		})
}

// StopBackgroundAnalysis stops background analysis
func (so *SelfOptimizer) StopBackgroundAnalysis() {
	so.mu.Lock()
	defer so.mu.Unlock()

	if !so.running {
		return
	}

	close(so.stopCh)
	so.running = false
	so.stopCh = make(chan struct{})

	logger.InfoCF("self_optimizer", "Background analysis stopped", nil)
}

// detectRepeatedFailures detects tasks with repeated failures
func (so *SelfOptimizer) detectRepeatedFailures(logs []ExecutionRecord) []PatternMatch {
	failureCounts := make(map[string]int)
	examples := make(map[string][]string)
	firstSeen := make(map[string]time.Time)
	lastSeen := make(map[string]time.Time)

	for _, log := range logs {
		if log.Status == "failed" && log.Error != "" {
			// Extract error pattern (simplified)
			pattern := extractErrorPattern(log.Error)
			failureCounts[pattern]++
			
			if len(examples[pattern]) < 3 {
				examples[pattern] = append(examples[pattern], log.TaskID)
			}

			if _, exists := firstSeen[pattern]; !exists {
				firstSeen[pattern] = log.Timestamp
			}
			lastSeen[pattern] = log.Timestamp
		}
	}

	matches := make([]PatternMatch, 0)
	for pattern, count := range failureCounts {
		if count >= 3 { // Minimum threshold for pattern detection
			matches = append(matches, PatternMatch{
				Pattern:     pattern,
				Description: fmt.Sprintf("Repeated failure pattern: %s", pattern),
				Occurrences: count,
				Examples:    examples[pattern],
				FirstSeen:   firstSeen[pattern],
				LastSeen:    lastSeen[pattern],
			})
		}
	}

	return matches
}

// detectTimeoutPatterns detects tasks consistently hitting timeout limits
func (so *SelfOptimizer) detectTimeoutPatterns(logs []ExecutionRecord) []PatternMatch {
	timeoutCounts := make(map[string]int)
	examples := make(map[string][]string)

	for _, log := range logs {
		// Heuristic: if duration is very long (> 5 minutes), likely timeout-related
		if log.Duration > 5*time.Minute {
			key := log.AgentType
			timeoutCounts[key]++
			
			if len(examples[key]) < 3 {
				examples[key] = append(examples[key], log.TaskID)
			}
		}
	}

	matches := make([]PatternMatch, 0)
	for agentType, count := range timeoutCounts {
		if count >= 2 {
			matches = append(matches, PatternMatch{
				Pattern:     agentType,
				Description: fmt.Sprintf("Agent %s frequently timing out", agentType),
				Occurrences: count,
				Examples:    examples[agentType],
			})
		}
	}

	return matches
}

// detectExcessiveRetries detects tasks requiring excessive retries
func (so *SelfOptimizer) detectExcessiveRetries(logs []ExecutionRecord) []PatternMatch {
	retryPatterns := make(map[string][]int)
	examples := make(map[string][]string)

	for _, log := range logs {
		if log.RetryCount >= 2 {
			key := log.AgentType
			retryPatterns[key] = append(retryPatterns[key], log.RetryCount)
			
			if len(examples[key]) < 3 {
				examples[key] = append(examples[key], log.TaskID)
			}
		}
	}

	matches := make([]PatternMatch, 0)
	for agentType, retries := range retryPatterns {
		if len(retries) >= 2 {
			avgRetries := 0
			for _, r := range retries {
				avgRetries += r
			}
			avgRetries /= len(retries)

			matches = append(matches, PatternMatch{
				Pattern:     agentType,
				Description: fmt.Sprintf("Average %.1f retries per task", float64(avgRetries)),
				Occurrences: len(retries),
				Examples:    examples[agentType],
			})
		}
	}

	return matches
}

// detectStepLimitIssues detects tasks hitting max step limits
func (so *SelfOptimizer) detectStepLimitIssues(logs []ExecutionRecord) []PatternMatch {
	stepLimitHits := make(map[string]int)
	examples := make(map[string][]string)

	for _, log := range logs {
		// Heuristic: if steps executed >= 10, might be hitting limits
		if log.StepsExecuted >= 10 {
			key := log.AgentType
			stepLimitHits[key]++
			
			if len(examples[key]) < 3 {
				examples[key] = append(examples[key], log.TaskID)
			}
		}
	}

	matches := make([]PatternMatch, 0)
	for agentType, count := range stepLimitHits {
		if count >= 2 {
			matches = append(matches, PatternMatch{
				Pattern:     agentType,
				Description: fmt.Sprintf("Agent %s frequently hitting step limits", agentType),
				Occurrences: count,
				Examples:    examples[agentType],
			})
		}
	}

	return matches
}

// detectErrorMessagePatterns detects common error message patterns
func (so *SelfOptimizer) detectErrorMessagePatterns(logs []ExecutionRecord) []PatternMatch {
	errorPatterns := make(map[string]int)
	examples := make(map[string][]string)

	for _, log := range logs {
		if log.Error != "" {
			pattern := extractErrorPattern(log.Error)
			errorPatterns[pattern]++
			
			if len(examples[pattern]) < 3 {
				examples[pattern] = append(examples[pattern], log.TaskID)
			}
		}
	}

	matches := make([]PatternMatch, 0)
	for pattern, count := range errorPatterns {
		if count >= 2 {
			matches = append(matches, PatternMatch{
				Pattern:     pattern,
				Description: fmt.Sprintf("Common error: %s", pattern),
				Occurrences: count,
				Examples:    examples[pattern],
			})
		}
	}

	return matches
}

// getSuggestionByIDUnsafe returns a suggestion by ID without locking (caller must hold lock)
func (so *SelfOptimizer) getSuggestionByIDUnsafe(id string) *OptimizationSuggestion {
	for _, s := range so.suggestions {
		if s.ID == id {
			return s
		}
	}
	return nil
}

// getConfidenceLevel converts a threshold to ConfidenceLevel
func (so *SelfOptimizer) getConfidenceLevel(threshold float64) ConfidenceLevel {
	if threshold >= 0.8 {
		return ConfidenceHigh
	} else if threshold >= 0.5 {
		return ConfidenceMedium
	}
	return ConfidenceLow
}

// extractErrorPattern extracts a simplified pattern from an error message
func extractErrorPattern(errorMsg string) string {
	// Remove variable parts like IDs, timestamps, etc.
	pattern := errorMsg
	
	// Remove UUIDs
	uuidRegex := regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`)
	pattern = uuidRegex.ReplaceAllString(pattern, "<UUID>")
	
	// Remove numbers
	numRegex := regexp.MustCompile(`\b\d+\b`)
	pattern = numRegex.ReplaceAllString(pattern, "<NUM>")
	
	// Truncate if too long
	if len(pattern) > 100 {
		pattern = pattern[:100] + "..."
	}
	
	return strings.TrimSpace(pattern)
}

// generateSuggestionID generates a unique ID for a suggestion
func generateSuggestionID(detectorName, pattern string) string {
	// Simple ID generation - in production, use UUID
	hash := fmt.Sprintf("%x", []byte(detectorName+pattern))
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return fmt.Sprintf("opt_%s_%s", detectorName, hash)
}
