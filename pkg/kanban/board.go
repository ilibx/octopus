package kanban

import (
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/logger"
)

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

// Task represents a single task in the kanban system
type Task struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	Status      TaskStatus        `json:"status"`
	Priority    int               `json:"priority"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	AssignedTo  string            `json:"assigned_to,omitempty"` // Agent ID
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
	AgentType   string            `json:"agent_type"` // Type of agent needed for this zone
	Active      bool              `json:"active"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

// KanbanBoard represents the main kanban board for task management
type KanbanBoard struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Zones       map[string]*Zone  `json:"zones"`
	MainAgentID string            `json:"main_agent_id"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	mu          sync.RWMutex
}

// NewKanbanBoard creates a new kanban board
func NewKanbanBoard(id, name, mainAgentID string) *KanbanBoard {
	return &KanbanBoard{
		ID:          id,
		Name:        name,
		Zones:       make(map[string]*Zone),
		MainAgentID: mainAgentID,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
}

// CreateZone creates a new zone in the kanban board
func (k *KanbanBoard) CreateZone(id, name, description, agentType string) *Zone {
	k.mu.Lock()
	defer k.mu.Unlock()

	zone := &Zone{
		ID:          id,
		Name:        name,
		Description: description,
		Tasks:       make([]*Task, 0),
		AgentType:   agentType,
		Active:      true,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    make(map[string]string),
	}

	k.Zones[id] = zone
	logger.InfoCF("kanban", "Zone created",
		map[string]any{
			"board_id":   k.ID,
			"zone_id":    id,
			"zone_name":  name,
			"agent_type": agentType,
		})

	return zone
}

// AddTask adds a task to a specific zone
func (k *KanbanBoard) AddTask(zoneID, taskID, title, description string, priority int, metadata map[string]string) (*Task, error) {
	k.mu.Lock()
	defer k.mu.Unlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return nil, ErrZoneNotFound
	}

	task := &Task{
		ID:          taskID,
		Title:       title,
		Description: description,
		Status:      TaskPending,
		Priority:    priority,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Metadata:    metadata,
	}

	zone.Tasks = append(zone.Tasks, task)
	zone.UpdatedAt = time.Now()
	k.UpdatedAt = time.Now()

	logger.InfoCF("kanban", "Task added",
		map[string]any{
			"board_id": k.ID,
			"zone_id":  zoneID,
			"task_id":  taskID,
			"title":    title,
			"priority": priority,
		})

	return task, nil
}

// UpdateTaskStatus updates the status of a task
func (k *KanbanBoard) UpdateTaskStatus(zoneID, taskID string, status TaskStatus, result, errorMsg string) error {
	k.mu.Lock()
	defer k.mu.Unlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return ErrZoneNotFound
	}

	for _, task := range zone.Tasks {
		if task.ID == taskID {
			task.Status = status
			task.Result = result
			task.Error = errorMsg
			task.UpdatedAt = time.Now()
			zone.UpdatedAt = time.Now()
			k.UpdatedAt = time.Now()

			logger.InfoCF("kanban", "Task status updated",
				map[string]any{
					"board_id": k.ID,
					"zone_id":  zoneID,
					"task_id":  taskID,
					"status":   status,
				})

			return nil
		}
	}

	return ErrTaskNotFound
}

// GetZone returns a zone by ID
func (k *KanbanBoard) GetZone(zoneID string) (*Zone, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return nil, ErrZoneNotFound
	}

	return zone, nil
}

// GetTasksByStatus returns all tasks in a zone with a specific status
func (k *KanbanBoard) GetTasksByStatus(zoneID string, status TaskStatus) []*Task {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return nil
	}

	var tasks []*Task
	for _, task := range zone.Tasks {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// GetPendingTasks returns all pending tasks across all zones
func (k *KanbanBoard) GetPendingTasks() map[string][]*Task {
	k.mu.RLock()
	defer k.mu.RUnlock()

	result := make(map[string][]*Task)
	for zoneID, zone := range k.Zones {
		var pending []*Task
		for _, task := range zone.Tasks {
			if task.Status == TaskPending {
				pending = append(pending, task)
			}
		}
		if len(pending) > 0 {
			result[zoneID] = pending
		}
	}

	return result
}

// HasActiveAgent checks if a zone has an active agent assigned
func (k *KanbanBoard) HasActiveAgent(zoneID string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return false
	}

	// Check if any task in the zone is being processed
	for _, task := range zone.Tasks {
		if task.Status == TaskRunning || task.Status == TaskInProgress {
			return true
		}
	}

	return false
}

// GetZoneAgentType returns the agent type required for a zone
func (k *KanbanBoard) GetZoneAgentType(zoneID string) (string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return "", ErrZoneNotFound
	}

	return zone.AgentType, nil
}
