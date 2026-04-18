// Package types provides shared types for kanban integration
// This package is designed to be imported by both pkg/kanban and pkg/channels
// without creating circular dependencies
package types

import "time"

// TaskStatus represents the current state of a task
type TaskStatus string

const (
	TaskPending    TaskStatus = "pending"
	TaskRunning    TaskStatus = "running"
	TaskCompleted  TaskStatus = "completed"
	TaskFailed     TaskStatus = "failed"
	TaskBlocked    TaskStatus = "blocked"
	TaskInProgress TaskStatus = "in_progress"
)

// TaskEvent represents an event published when task status changes
// This is used for communication between kanban and channels
type TaskEvent struct {
	Type      string     `json:"type"` // "task_created", "task_updated", "task_completed"
	BoardID   string     `json:"board_id"`
	ZoneID    string     `json:"zone_id"`
	TaskID    string     `json:"task_id"`
	Status    TaskStatus `json:"status,omitempty"`
	Title     string     `json:"title,omitempty"`
	Result    string     `json:"result,omitempty"`
	Error     string     `json:"error,omitempty"`
	Timestamp int64      `json:"timestamp"`
}

// Task represents a single task in the kanban system
type Task struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	Priority    int               `json:"priority"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	AssignedTo  string            `json:"assigned_to,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	Result      string            `json:"result,omitempty"`
	Error       string            `json:"error,omitempty"`
}

// Zone represents an independent area in the kanban board
type Zone struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Tasks       []*Task           `json:"tasks"`
	AgentType   string            `json:"agent_type"`
	Active      bool              `json:"active"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// KanbanBoardService defines the interface for kanban operations
// This allows channels to interact with kanban without importing pkg/kanban
type KanbanBoardService interface {
	// CreateTask creates a new task and returns it
	CreateTask(zoneID, taskID, title, description string, priority int, metadata map[string]string) (*Task, error)
	
	// GetBoard returns the current board state
	GetBoard() *KanbanBoardView
	
	// SubscribeToEvents subscribes to task events
	SubscribeToEvents(handler func(string))
}

// KanbanBoardView is a read-only view of the kanban board
type KanbanBoardView struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Zones       map[string]*Zone `json:"zones"`
	MainAgentID string         `json:"main_agent_id"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}
