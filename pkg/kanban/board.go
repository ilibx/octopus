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

// TaskDependency represents a dependency relationship between tasks
type TaskDependency struct {
	TaskID       string `json:"task_id"`        // The task that this task depends on
	RequiredStatus TaskStatus `json:"required_status"` // Status that must be reached (default: completed)
}

// Task represents a single task in the kanban system
// Tasks can have dependencies forming a DAG/Workflow
type Task struct {
	ID           string             `json:"id"`
	Title        string             `json:"title"`
	Description  string             `json:"description"`
	Status       TaskStatus         `json:"status"`
	Priority     int                `json:"priority"`
	CreatedAt    time.Time          `json:"created_at"`
	UpdatedAt    time.Time          `json:"updated_at"`
	AssignedTo   string             `json:"assigned_to,omitempty"` // Sub-Agent ID assigned to execute this task
	Metadata     map[string]string  `json:"metadata,omitempty"`
	Result       string             `json:"result,omitempty"`
	Error        string             `json:"error,omitempty"`
	Dependencies []TaskDependency   `json:"dependencies,omitempty"` // DAG/Workflow dependencies
	DependsOn    []string           `json:"depends_on,omitempty"`   // Simple list of task IDs this task depends on
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
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Zones       map[string]*Zone         `json:"zones"`
	MainAgentID string                   `json:"main_agent_id"`
	CreatedAt   time.Time                `json:"created_at"`
	UpdatedAt   time.Time                `json:"updated_at"`
	mu          sync.RWMutex
	zoneLocks   map[string]*sync.RWMutex // Per-zone locks for fine-grained concurrency
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
		zoneLocks:   make(map[string]*sync.RWMutex),
	}
}

// getZoneLock returns the lock for a specific zone, creating it if necessary
func (k *KanbanBoard) getZoneLock(zoneID string) *sync.RWMutex {
	k.mu.Lock()
	defer k.mu.Unlock()

	if _, exists := k.zoneLocks[zoneID]; !exists {
		k.zoneLocks[zoneID] = &sync.RWMutex{}
	}
	return k.zoneLocks[zoneID]
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

// AddTask adds a task to a specific zone using per-zone lock
func (k *KanbanBoard) AddTask(zoneID, taskID, title, description string, priority int, metadata map[string]string) (*Task, error) {
	// First check if zone exists with read lock
	k.mu.RLock()
	zone, ok := k.Zones[zoneID]
	k.mu.RUnlock()

	if !ok {
		return nil, ErrZoneNotFound
	}

	// Use per-zone lock for fine-grained concurrency
	zoneLock := k.getZoneLock(zoneID)
	zoneLock.Lock()
	defer zoneLock.Unlock()

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

	// Update board timestamp with write lock
	k.mu.Lock()
	k.UpdatedAt = time.Now()
	k.mu.Unlock()

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

// UpdateTaskStatus updates the status of a task using per-zone lock
func (k *KanbanBoard) UpdateTaskStatus(zoneID, taskID string, status TaskStatus, result, errorMsg string) error {
	// First check if zone exists with read lock
	k.mu.RLock()
	zone, ok := k.Zones[zoneID]
	k.mu.RUnlock()

	if !ok {
		return ErrZoneNotFound
	}

	// Use per-zone lock for fine-grained concurrency
	zoneLock := k.getZoneLock(zoneID)
	zoneLock.Lock()
	defer zoneLock.Unlock()

	for _, task := range zone.Tasks {
		if task.ID == taskID {
			task.Status = status
			task.Result = result
			task.Error = errorMsg
			task.UpdatedAt = time.Now()
			zone.UpdatedAt = time.Now()

			// Update board timestamp with write lock
			k.mu.Lock()
			k.UpdatedAt = time.Now()
			k.mu.Unlock()

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

// GetTasksByStatus returns all tasks in a zone with a specific status with optimized memory allocation
func (k *KanbanBoard) GetTasksByStatus(zoneID string, status TaskStatus) []*Task {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return nil
	}

	// Pre-count matching tasks to avoid slice resizing
	count := 0
	for _, task := range zone.Tasks {
		if task.Status == status {
			count++
		}
	}

	if count == 0 {
		return nil
	}

	tasks := make([]*Task, 0, count)
	for _, task := range zone.Tasks {
		if task.Status == status {
			tasks = append(tasks, task)
		}
	}

	return tasks
}

// GetPendingTasks returns all pending tasks across all zones with optimized memory allocation
func (k *KanbanBoard) GetPendingTasks() map[string][]*Task {
	k.mu.RLock()
	defer k.mu.RUnlock()

	// Pre-count zones with pending tasks to avoid map resizing
	zonesWithPending := 0
	for _, zone := range k.Zones {
		for _, task := range zone.Tasks {
			if task.Status == TaskPending {
				zonesWithPending++
				break
			}
		}
	}

	result := make(map[string][]*Task, zonesWithPending)
	for zoneID, zone := range k.Zones {
		// Count pending tasks in this zone for pre-allocation
		pendingCount := 0
		for _, task := range zone.Tasks {
			if task.Status == TaskPending {
				pendingCount++
			}
		}
		
		if pendingCount > 0 {
			pending := make([]*Task, 0, pendingCount)
			for _, task := range zone.Tasks {
				if task.Status == TaskPending {
					pending = append(pending, task)
				}
			}
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

// AreTaskDependenciesSatisfied checks if all dependencies of a task are satisfied
func (k *KanbanBoard) AreTaskDependenciesSatisfied(zoneID, taskID string) bool {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return false
	}

	var targetTask *Task
	for _, task := range zone.Tasks {
		if task.ID == taskID {
			targetTask = task
			break
		}
	}

	if targetTask == nil {
		return false
	}

	// Check simple DependsOn list
	for _, depID := range targetTask.DependsOn {
		found := false
		for _, task := range zone.Tasks {
			if task.ID == depID && task.Status == TaskCompleted {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check detailed Dependencies with required status
	for _, dep := range targetTask.Dependencies {
		found := false
		for _, task := range zone.Tasks {
			if task.ID == dep.TaskID && task.Status == dep.RequiredStatus {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// GetReadyTasks returns all tasks that are ready to execute (pending + dependencies satisfied)
func (k *KanbanBoard) GetReadyTasks(zoneID string) []*Task {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return nil
	}

	var readyTasks []*Task
	for _, task := range zone.Tasks {
		if task.Status == TaskPending {
			// Check if task has any dependencies
			hasDependencies := len(task.DependsOn) > 0 || len(task.Dependencies) > 0
			
			if !hasDependencies {
				// No dependencies, task is ready
				readyTasks = append(readyTasks, task)
			} else if k.areTaskDependenciesSatisfiedLocked(zone, task.ID) {
				// Has dependencies but they are all satisfied
				readyTasks = append(readyTasks, task)
			}
		}
	}

	return readyTasks
}

// areTaskDependenciesSatisfiedLocked checks dependencies without acquiring lock (caller must hold lock)
func (k *KanbanBoard) areTaskDependenciesSatisfiedLocked(zone *Zone, taskID string) bool {
	var targetTask *Task
	for _, task := range zone.Tasks {
		if task.ID == taskID {
			targetTask = task
			break
		}
	}

	if targetTask == nil {
		return false
	}

	// Check simple DependsOn list
	for _, depID := range targetTask.DependsOn {
		found := false
		for _, task := range zone.Tasks {
			if task.ID == depID && task.Status == TaskCompleted {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check detailed Dependencies with required status
	for _, dep := range targetTask.Dependencies {
		found := false
		for _, task := range zone.Tasks {
			if task.ID == dep.TaskID && task.Status == dep.RequiredStatus {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// GetTaskExecutionOrder returns tasks in topological order for DAG execution
func (k *KanbanBoard) GetTaskExecutionOrder(zoneID string) ([]string, error) {
	k.mu.RLock()
	defer k.mu.RUnlock()

	zone, ok := k.Zones[zoneID]
	if !ok {
		return nil, ErrZoneNotFound
	}

	// Build adjacency list and in-degree count
	inDegree := make(map[string]int)
	adjList := make(map[string][]string)
	taskIDs := make(map[string]bool)

	for _, task := range zone.Tasks {
		taskIDs[task.ID] = true
		inDegree[task.ID] = 0
	}

	// Build graph based on dependencies
	for _, task := range zone.Tasks {
		for _, depID := range task.DependsOn {
			if _, exists := taskIDs[depID]; exists {
				adjList[depID] = append(adjList[depID], task.ID)
				inDegree[task.ID]++
			}
		}
		for _, dep := range task.Dependencies {
			if _, exists := taskIDs[dep.TaskID]; exists {
				adjList[dep.TaskID] = append(adjList[dep.TaskID], task.ID)
				inDegree[task.ID]++
			}
		}
	}

	// Kahn's algorithm for topological sort
	var queue []string
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	var result []string
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		for _, neighbor := range adjList[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Check for cycles
	if len(result) != len(taskIDs) {
		return nil, ErrCircularDependency
	}

	return result, nil
}
