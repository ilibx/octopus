package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/cron"
	"github.com/ilibx/octopus/pkg/logger"
)

// CronKanbanIntegration handles integration between cron service and kanban board
type CronKanbanIntegration struct {
	board       *KanbanBoard
	service     *KanbanService
	cronService *cron.CronService
	msgBus      *bus.MessageBus
	mu          sync.RWMutex
}

// NewCronKanbanIntegration creates a new integration instance
func NewCronKanbanIntegration(board *KanbanBoard, service *KanbanService, cronService *cron.CronService, msgBus *bus.MessageBus) *CronKanbanIntegration {
	return &CronKanbanIntegration{
		board:       board,
		service:     service,
		cronService: cronService,
		msgBus:      msgBus,
	}
}

// SetupCronHandlers registers cron job handlers for kanban task creation
func (i *CronKanbanIntegration) SetupCronHandlers() {
	i.cronService.SetOnJob(i.handleCronJob)
	logger.InfoCF("cron_kanban", "Cron-Kanban integration handlers registered", nil)
}

// handleCronJob processes cron jobs and creates tasks on the kanban board
func (i *CronKanbanIntegration) handleCronJob(job *cron.CronJob) (string, error) {
	logger.InfoCF("cron_kanban", "Processing cron job for kanban",
		map[string]any{
			"job_id":   job.ID,
			"job_name": job.Name,
			"payload":  job.Payload,
		})

	// Parse the payload to extract task information
	var taskPayload struct {
		ZoneID      string            `json:"zone_id"`
		Title       string            `json:"title"`
		Description string            `json:"description"`
		Priority    int               `json:"priority"`
		Metadata    map[string]string `json:"metadata"`
	}

	if err := json.Unmarshal([]byte(job.Payload.Message), &taskPayload); err != nil {
		// Try simple message format
		taskPayload.Title = job.Payload.Message
		taskPayload.Description = fmt.Sprintf("Scheduled task from cron job: %s", job.Name)
		taskPayload.Priority = 5
		taskPayload.ZoneID = "default"
	}

	// Create task on kanban board
	taskID := fmt.Sprintf("cron_%s_%d", job.ID, time.Now().Unix())
	task, err := i.service.CreateTaskWithEvent(
		taskPayload.ZoneID,
		taskID,
		taskPayload.Title,
		taskPayload.Description,
		taskPayload.Priority,
		taskPayload.Metadata,
	)

	if err != nil {
		logger.ErrorCF("cron_kanban", "Failed to create task from cron job",
			map[string]any{"job_id": job.ID, "error": err.Error()})
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	logger.InfoCF("cron_kanban", "Task created from cron job",
		map[string]any{
			"job_id":   job.ID,
			"task_id":  task.ID,
			"zone_id":  taskPayload.ZoneID,
			"title":    task.Title,
		})

	return fmt.Sprintf("Task %s created successfully", task.ID), nil
}

// ScheduleRecurringTask schedules a recurring task on the kanban board
func (i *CronKanbanIntegration) ScheduleRecurringTask(zoneID, title, description string, priority int, cronExpr string, metadata map[string]string) (*cron.CronJob, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Create task payload
	taskPayload := map[string]interface{}{
		"zone_id":      zoneID,
		"title":        title,
		"description":  description,
		"priority":     priority,
		"metadata":     metadata,
	}

	payloadJSON, err := json.Marshal(taskPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create cron job
	schedule := cron.CronSchedule{
		Kind: "cron",
		Expr: cronExpr,
		TZ:   "UTC",
	}

	job, err := i.cronService.AddJob(
		fmt.Sprintf("Recurring: %s", title),
		schedule,
		string(payloadJSON),
		false, // deliver
		metadata,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to add cron job: %w", err)
	}

	logger.InfoCF("cron_kanban", "Recurring task scheduled",
		map[string]any{
			"job_id":    job.ID,
			"zone_id":   zoneID,
			"title":     title,
			"cron_expr": cronExpr,
		})

	return job, nil
}

// ScheduleOneTimeTask schedules a one-time task on the kanban board
func (i *CronKanbanIntegration) ScheduleOneTimeTask(zoneID, title, description string, priority int, executeAt time.Time, metadata map[string]string) (*cron.CronJob, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	// Create task payload
	taskPayload := map[string]interface{}{
		"zone_id":      zoneID,
		"title":        title,
		"description":  description,
		"priority":     priority,
		"metadata":     metadata,
	}

	payloadJSON, err := json.Marshal(taskPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create cron job with "at" schedule
	atMS := executeAt.UnixMilli()
	schedule := cron.CronSchedule{
		Kind: "at",
		AtMS: &atMS,
	}

	job, err := i.cronService.AddJob(
		fmt.Sprintf("One-time: %s", title),
		schedule,
		string(payloadJSON),
		false, // deliver
		metadata,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to add cron job: %w", err)
	}

	logger.InfoCF("cron_kanban", "One-time task scheduled",
		map[string]any{
			"job_id":     job.ID,
			"zone_id":    zoneID,
			"title":      title,
			"execute_at": executeAt.Format(time.RFC3339),
		})

	return job, nil
}

// GetScheduledTasks returns all scheduled tasks from cron service
func (i *CronKanbanIntegration) GetScheduledTasks(includeDisabled bool) []cron.CronJob {
	return i.cronService.ListJobs(includeDisabled)
}

// CancelScheduledTask cancels a scheduled task
func (i *CronKanbanIntegration) CancelScheduledTask(jobID string) bool {
	removed := i.cronService.RemoveJob(jobID)
	if removed {
		logger.InfoCF("cron_kanban", "Scheduled task cancelled",
			map[string]any{"job_id": jobID})
	}
	return removed
}

// StartBackgroundMonitor starts a background goroutine to monitor cron-kanban integration
func (i *CronKanbanIntegration) StartBackgroundMonitor(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.InfoCF("cron_kanban", "Stopping background monitor", nil)
				return
			case <-ticker.C:
				i.logIntegrationStats()
			}
		}
	}()

	logger.InfoCF("cron_kanban", "Background monitor started", nil)
}

// logIntegrationStats logs statistics about the integration
func (i *CronKanbanIntegration) logIntegrationStats() {
	i.mu.RLock()
	defer i.mu.RUnlock()

	cronStatus := i.cronService.Status()
	pendingTasks := i.board.GetPendingTasks()

	totalPending := 0
	for _, tasks := range pendingTasks {
		totalPending += len(tasks)
	}

	logger.DebugCF("cron_kanban", "Integration stats",
		map[string]any{
			"cron_enabled":      cronStatus["enabled"],
			"cron_jobs":         cronStatus["jobs"],
			"pending_tasks":     totalPending,
			"next_cron_wake":    cronStatus["nextWakeAtMS"],
		})
}

// ExportCronConfig exports current cron configuration as JSON
func (i *CronKanbanIntegration) ExportCronConfig() ([]byte, error) {
	jobs := i.GetScheduledTasks(true)
	return json.MarshalIndent(jobs, "", "  ")
}

// ImportCronConfig imports cron configuration from JSON
func (i *CronKanbanIntegration) ImportCronConfig(configData []byte) error {
	var jobs []cron.CronJob
	if err := json.Unmarshal(configData, &jobs); err != nil {
		return fmt.Errorf("failed to unmarshal config: %w", err)
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	imported := 0
	for _, job := range jobs {
		// Re-add each job
		_, err := i.cronService.AddJob(
			job.Name,
			job.Schedule,
			job.Payload.Message,
			job.Payload.Deliver,
			job.Payload.Metadata,
		)
		if err == nil {
			imported++
		}
	}

	logger.InfoCF("cron_kanban", "Cron config imported",
		map[string]any{"imported_count": imported})

	return nil
}

// HandleCronJobForTest exports handleCronJob for testing purposes
func (i *CronKanbanIntegration) HandleCronJobForTest(job *cron.CronJob) (string, error) {
	return i.handleCronJob(job)
}
