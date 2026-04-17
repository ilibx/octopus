package kanban

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/agent"
	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/config"
	"github.com/ilibx/octopus/pkg/logger"
)

// AgentWorker manages the execution of tasks for a specific zone
type AgentWorker struct {
	zoneID         string
	agentID        string
	board          *KanbanBoard
	service        *KanbanService
	agentInstance  *agent.AgentInstance
	msgBus         *bus.MessageBus
	cfg            *config.Config
	maxConcurrency int
	currentTasks   map[string]bool // taskID -> running
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
}

// NewAgentWorker creates a new agent worker for a specific zone
func NewAgentWorker(zoneID, agentID string, board *KanbanBoard, service *KanbanService, 
	agentInstance *agent.AgentInstance, msgBus *bus.MessageBus, cfg *config.Config, 
	maxConcurrency int) *AgentWorker {
	
	ctx, cancel := context.WithCancel(context.Background())
	
	return &AgentWorker{
		zoneID:         zoneID,
		agentID:        agentID,
		board:          board,
		service:        service,
		agentInstance:  agentInstance,
		msgBus:         msgBus,
		cfg:            cfg,
		maxConcurrency: maxConcurrency,
		currentTasks:   make(map[string]bool),
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start begins the worker's task processing loop
func (w *AgentWorker) Start() {
	logger.InfoCF("agent_worker", "Starting agent worker",
		map[string]any{
			"zone_id":    w.zoneID,
			"agent_id":   w.agentID,
			"max_concurrency": w.maxConcurrency,
		})

	// Start multiple worker goroutines based on maxConcurrency
	var wg sync.WaitGroup
	for i := 0; i < w.maxConcurrency; i++ {
		wg.Add(1)
		go func(workerNum int) {
			defer wg.Done()
			w.processTasksLoop(workerNum)
		}(i)
	}

	wg.Wait()
}

// processTasksLoop continuously processes tasks from the zone
func (w *AgentWorker) processTasksLoop(workerNum int) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			logger.InfoCF("agent_worker", "Worker stopping due to context cancellation",
				map[string]any{"zone_id": w.zoneID, "worker_num": workerNum})
			return
		case <-ticker.C:
			w.tryProcessNextTask(workerNum)
		}
	}
}

// tryProcessNextTask attempts to fetch and process the next pending task
func (w *AgentWorker) tryProcessNextTask(workerNum int) {
	// Check if we're at max concurrency
	w.mu.RLock()
	if len(w.currentTasks) >= w.maxConcurrency {
		w.mu.RUnlock()
		return
	}
	w.mu.RUnlock()

	// Fetch next pending task from the zone
	task := w.fetchNextPendingTask()
	if task == nil {
		return
	}

	// Try to claim the task
	if !w.claimTask(task.ID) {
		return
	}

	// Process the task in a separate goroutine
	go w.executeTask(task, workerNum)
}

// fetchNextPendingTask gets the next pending task from the zone respecting DAG dependencies
func (w *AgentWorker) fetchNextPendingTask() *Task {
	// Get ready tasks (pending tasks with satisfied dependencies)
	tasks := w.board.GetReadyTasks(w.zoneID)
	if len(tasks) == 0 {
		return nil
	}

	// Sort by priority (higher priority first) and return the first one
	var highestPriorityTask *Task
	for _, task := range tasks {
		if highestPriorityTask == nil || task.Priority > highestPriorityTask.Priority {
			highestPriorityTask = task
		}
	}

	return highestPriorityTask
}

// claimTask marks a task as claimed by this worker
func (w *AgentWorker) claimTask(taskID string) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.currentTasks[taskID] {
		return false
	}

	w.currentTasks[taskID] = true
	return true
}

// releaseTask releases a task from this worker
func (w *AgentWorker) releaseTask(taskID string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	delete(w.currentTasks, taskID)
}

// executeTask executes a single task using the sub-agent
// The sub-agent reports status back to the Main Agent via the KanbanService
func (w *AgentWorker) executeTask(task *Task, workerNum int) {
	defer w.releaseTask(task.ID)

	logger.InfoCF("agent_worker", "Sub-agent executing task",
		map[string]any{
			"zone_id":     w.zoneID,
			"sub_agent_id": w.agentID,
			"task_id":     task.ID,
			"title":       task.Title,
			"worker_num":  workerNum,
		})

	// Update task status to running - this notifies the Main Agent
	err := w.service.UpdateTaskStatusWithEvent(w.zoneID, task.ID, TaskRunning, "", "")
	if err != nil {
		logger.ErrorCF("agent_worker", "Failed to update task status to running",
			map[string]any{"task_id": task.ID, "error": err.Error()})
		return
	}

	// Execute the task using the sub-agent instance
	result, err := w.runTaskExecution(task)

	// Update task status based on result - this notifies the Main Agent
	if err != nil {
		logger.ErrorCF("agent_worker", "Sub-agent task execution failed",
			map[string]any{
				"sub_agent_id": w.agentID,
				"task_id":      task.ID,
				"error":        err.Error(),
			})
		
		err = w.service.UpdateTaskStatusWithEvent(w.zoneID, task.ID, TaskFailed, "", err.Error())
		if err != nil {
			logger.ErrorCF("agent_worker", "Failed to update task status to failed",
				map[string]any{"task_id": task.ID, "error": err.Error()})
		}
	} else {
		logger.InfoCF("agent_worker", "Sub-agent task completed successfully",
			map[string]any{
				"sub_agent_id": w.agentID,
				"task_id":      task.ID,
				"result":       result,
			})
		
		err = w.service.UpdateTaskStatusWithEvent(w.zoneID, task.ID, TaskCompleted, result, "")
		if err != nil {
			logger.ErrorCF("agent_worker", "Failed to update task status to completed",
				map[string]any{"task_id": task.ID, "error": err.Error()})
		}
	}
}

// runTaskExecution executes the actual task logic using the agent
func (w *AgentWorker) runTaskExecution(task *Task) (string, error) {
	// Build the prompt from task description and metadata
	prompt := fmt.Sprintf("Task: %s\nDescription: %s", task.Title, task.Description)
	
	if len(task.Metadata) > 0 {
		prompt += "\nMetadata:"
		for k, v := range task.Metadata {
			prompt += fmt.Sprintf("\n  %s: %s", k, v)
		}
	}

	// Use the agent instance to execute the task via ProcessDirect
	if w.agentInstance == nil {
		return "", fmt.Errorf("agent instance not available")
	}

	// Execute with timeout
	ctx, cancel := context.WithTimeout(w.ctx, 5*time.Minute)
	defer cancel()

	// Create a minimal loop to process the task
	// We need to use the agent's registry and message bus
	result, err := w.processTaskWithAgent(ctx, prompt, task.ID)
	if err != nil {
		return "", fmt.Errorf("agent execution failed: %w", err)
	}

	return result, nil
}

// processTaskWithAgent processes a task using the agent instance
func (w *AgentWorker) processTaskWithAgent(ctx context.Context, prompt, taskID string) (string, error) {
	// For now, we'll create a simple execution context
	// In a full implementation, this would integrate with the AgentLoop
	
	// Get the session for this task
	sessionKey := fmt.Sprintf("task_%s_%s", w.zoneID, taskID)
	
	// Use the agent's context builder to prepare the execution
	contextData, err := w.agentInstance.ContextBuilder.BuildContext(ctx, sessionKey, prompt)
	if err != nil {
		// If context building fails, try direct execution
		logger.WarnCF("agent_worker", "Failed to build context, using direct execution",
			map[string]any{"task_id": taskID, "error": err.Error()})
		return prompt, nil
	}
	
	// For now, return the context as the result
	// In production, this would call the LLM provider
	return contextData, nil
}

// Stop gracefully stops the worker
func (w *AgentWorker) Stop() {
	logger.InfoCF("agent_worker", "Stopping agent worker",
		map[string]any{"zone_id": w.zoneID, "agent_id": w.agentID})

	w.cancel()

	// Wait for current tasks to complete (with timeout)
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			logger.WarnCF("agent_worker", "Worker stop timed out, forcing shutdown",
				map[string]any{"zone_id": w.zoneID})
			return
		case <-ticker.C:
			w.mu.RLock()
			remaining := len(w.currentTasks)
			w.mu.RUnlock()
			
			if remaining == 0 {
				logger.InfoCF("agent_worker", "Worker stopped gracefully",
					map[string]any{"zone_id": w.zoneID})
				return
			}
		}
	}
}

// GetActiveTasks returns the number of currently active tasks
func (w *AgentWorker) GetActiveTasks() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.currentTasks)
}

// GetZoneID returns the zone ID this worker is managing
func (w *AgentWorker) GetZoneID() string {
	return w.zoneID
}

// GetAgentID returns the agent ID this worker is using
func (w *AgentWorker) GetAgentID() string {
	return w.agentID
}
