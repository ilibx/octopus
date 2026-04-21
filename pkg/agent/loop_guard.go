package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/logger"
)

// CallRecord represents a single SKILL/tool call for loop detection
type CallRecord struct {
	SkillID    string    `json:"skill_id"`
	ParamsHash string    `json:"params_hash"`
	Timestamp  time.Time `json:"timestamp"`
}

// LoopGuard tracks SKILL calls to detect and prevent infinite loops
type LoopGuard struct {
	callHistory map[string][]CallRecord // taskID -> recent calls
	maxHistory  int                     // Max history entries per task
	mu          sync.RWMutex
}

// NewLoopGuard creates a new loop guard instance
func NewLoopGuard(maxHistory int) *LoopGuard {
	if maxHistory <= 0 {
		maxHistory = 10 // Default: keep last 10 calls per task
	}

	return &LoopGuard{
		callHistory: make(map[string][]CallRecord),
		maxHistory:  maxHistory,
	}
}

// hashParams creates a deterministic hash of parameters for comparison
func hashParams(params interface{}) string {
	// Convert params to JSON string for hashing
	data := fmt.Sprintf("%v", params)
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:])
}

// CheckAndRecord checks for loop patterns and records the current call
// Returns error if a loop is detected
func (lg *LoopGuard) CheckAndRecord(taskID, skillID string, params interface{}) error {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	paramHash := hashParams(params)
	history := lg.callHistory[taskID]

	// Check for loop pattern: same skill called 3+ times with identical params
	// This matches the design doc requirement: "连续 2 次调用同一 SKILL 且参数未变化，视为死循环"
	if len(history) >= 2 {
		last1 := history[len(history)-1]
		last2 := history[len(history)-2]

		if last1.SkillID == skillID && last1.ParamsHash == paramHash &&
			last2.SkillID == skillID && last2.ParamsHash == paramHash {

			logger.WarnCF("loop_guard", "Loop detected - same skill called 3 times with identical params",
				map[string]any{
					"task_id":  taskID,
					"skill_id": skillID,
					"count":    3,
				})

			return fmt.Errorf("loop_detected: skill %s called 3 times with identical parameters", skillID)
		}
	}

	// Record this call
	record := CallRecord{
		SkillID:    skillID,
		ParamsHash: paramHash,
		Timestamp:  time.Now(),
	}

	history = append(history, record)

	// Trim history to max size
	if len(history) > lg.maxHistory {
		history = history[len(history)-lg.maxHistory:]
	}

	lg.callHistory[taskID] = history

	return nil
}

// GetCallHistory returns the recent call history for a task (for debugging/monitoring)
func (lg *LoopGuard) GetCallHistory(taskID string) []CallRecord {
	lg.mu.RLock()
	defer lg.mu.RUnlock()

	history, exists := lg.callHistory[taskID]
	if !exists {
		return nil
	}

	// Return a copy to avoid race conditions
	result := make([]CallRecord, len(history))
	copy(result, history)
	return result
}

// ClearTaskHistory clears the call history for a specific task
func (lg *LoopGuard) ClearTaskHistory(taskID string) {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	delete(lg.callHistory, taskID)
}

// Reset clears all call history
func (lg *LoopGuard) Reset() {
	lg.mu.Lock()
	defer lg.mu.Unlock()

	lg.callHistory = make(map[string][]CallRecord)
}

// GetStats returns statistics about tracked calls
func (lg *LoopGuard) GetStats() map[string]interface{} {
	lg.mu.RLock()
	defer lg.mu.RUnlock()

	totalCalls := 0
	for _, history := range lg.callHistory {
		totalCalls += len(history)
	}

	return map[string]interface{}{
		"total_tasks_tracked":  len(lg.callHistory),
		"total_calls_tracked":  totalCalls,
		"max_history_per_task": lg.maxHistory,
	}
}
