package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/logger"
)

// SnapshotManager handles periodic state persistence and recovery
type SnapshotManager struct {
	board        *KanbanBoard
	snapshotPath string
	interval     time.Duration
	mu           sync.RWMutex
	lastSave     time.Time
}

// NewSnapshotManager creates a new snapshot manager
func NewSnapshotManager(board *KanbanBoard, snapshotPath string, interval time.Duration) *SnapshotManager {
	if snapshotPath == "" {
		snapshotPath = "state_snapshot.json"
	}
	if interval == 0 {
		interval = 60 * time.Second // Default: 60 seconds
	}

	return &SnapshotManager{
		board:        board,
		snapshotPath: snapshotPath,
		interval:     interval,
	}
}

// StartPeriodicSnapshot begins the periodic snapshot loop
func (sm *SnapshotManager) StartPeriodicSnapshot(ctx context.Context) {
	ticker := time.NewTicker(sm.interval)
	defer ticker.Stop()

	logger.InfoCF("snapshot", "Starting periodic snapshot manager",
		map[string]any{
			"interval": sm.interval.String(),
			"path":     sm.snapshotPath,
		})

	for {
		select {
		case <-ctx.Done():
			logger.InfoCF("snapshot", "Stopping snapshot manager, saving final snapshot", nil)
			sm.saveSnapshot() // Save on exit
			return
		case <-ticker.C:
			sm.saveSnapshot()
		}
	}
}

// saveSnapshot persists the current board state to disk
func (sm *SnapshotManager) saveSnapshot() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	startTime := time.Now()

	// Serialize board state
	data, err := json.MarshalIndent(sm.board, "", "  ")
	if err != nil {
		logger.ErrorCF("snapshot", "Failed to marshal board state",
			map[string]any{"error": err.Error()})
		return fmt.Errorf("marshal failed: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(sm.snapshotPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		logger.ErrorCF("snapshot", "Failed to create snapshot directory",
			map[string]any{"dir": dir, "error": err.Error()})
		return fmt.Errorf("mkdir failed: %w", err)
	}

	// Write to temporary file first (atomic write)
	tmpPath := sm.snapshotPath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		logger.ErrorCF("snapshot", "Failed to write snapshot file",
			map[string]any{"path": tmpPath, "error": err.Error()})
		return fmt.Errorf("write failed: %w", err)
	}

	// Atomic rename
	if err := os.Rename(tmpPath, sm.snapshotPath); err != nil {
		logger.ErrorCF("snapshot", "Failed to rename snapshot file",
			map[string]any{"from": tmpPath, "to": sm.snapshotPath, "error": err.Error()})
		return fmt.Errorf("rename failed: %w", err)
	}

	sm.lastSave = time.Now()
	duration := time.Since(startTime)

	logger.InfoCF("snapshot", "Snapshot saved successfully",
		map[string]any{
			"path":     sm.snapshotPath,
			"size":     len(data),
			"duration": duration.String(),
		})

	return nil
}

// RestoreFromSnapshot loads board state from disk
func (sm *SnapshotManager) RestoreFromSnapshot() error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	// Check if snapshot file exists
	if _, err := os.Stat(sm.snapshotPath); os.IsNotExist(err) {
		logger.InfoCF("snapshot", "No snapshot file found, starting fresh", nil)
		return nil // Not an error, just no snapshot yet
	}

	// Read snapshot file
	data, err := os.ReadFile(sm.snapshotPath)
	if err != nil {
		logger.ErrorCF("snapshot", "Failed to read snapshot file",
			map[string]any{"path": sm.snapshotPath, "error": err.Error()})
		return fmt.Errorf("read failed: %w", err)
	}

	// Deserialize into board
	// Note: We need to preserve the locks, so we copy data fields only
	var snapshotData map[string]interface{}
	if err := json.Unmarshal(data, &snapshotData); err != nil {
		logger.ErrorCF("snapshot", "Failed to unmarshal snapshot",
			map[string]any{"error": err.Error()})
		return fmt.Errorf("unmarshal failed: %w", err)
	}

	// Re-marshall with proper types to avoid interface{} issues
	cleanData, err := json.Marshal(snapshotData)
	if err != nil {
		logger.ErrorCF("snapshot", "Failed to re-marshal snapshot",
			map[string]any{"error": err.Error()})
		return fmt.Errorf("re-marshal failed: %w", err)
	}

	// Create a temporary board for unmarshaling
	tempBoard := &KanbanBoard{
		Zones:     make(map[string]*Zone),
		zoneLocks: make(map[string]*sync.RWMutex),
	}

	if err := json.Unmarshal(cleanData, tempBoard); err != nil {
		logger.ErrorCF("snapshot", "Failed to unmarshal into temp board",
			map[string]any{"error": err.Error()})
		return fmt.Errorf("unmarshal into board failed: %w", err)
	}

	// Copy data to actual board while preserving locks
	sm.board.mu.Lock()
	sm.board.ID = tempBoard.ID
	sm.board.Name = tempBoard.Name
	sm.board.MainAgentID = tempBoard.MainAgentID
	sm.board.CreatedAt = tempBoard.CreatedAt
	sm.board.UpdatedAt = tempBoard.UpdatedAt

	// Copy zones
	sm.board.Zones = tempBoard.Zones

	// Reinitialize zone locks
	sm.board.zoneLocks = make(map[string]*sync.RWMutex)
	for zoneID := range sm.board.Zones {
		sm.board.zoneLocks[zoneID] = &sync.RWMutex{}
	}
	sm.board.mu.Unlock()

	logger.InfoCF("snapshot", "Snapshot restored successfully",
		map[string]any{
			"path":        sm.snapshotPath,
			"zones_count": len(tempBoard.Zones),
		})

	return nil
}

// GetLastSaveTime returns the time of the last successful save
func (sm *SnapshotManager) GetLastSaveTime() time.Time {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.lastSave
}

// SaveOnDemand triggers an immediate snapshot save (e.g., on critical state changes)
func (sm *SnapshotManager) SaveOnDemand() error {
	return sm.saveSnapshot()
}
