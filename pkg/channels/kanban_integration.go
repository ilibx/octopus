package channels

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ilibx/octopus/pkg/bus"
	kanbantypes "github.com/ilibx/octopus/pkg/kanban/types"
	"github.com/ilibx/octopus/pkg/logger"
)

// KanbanIntegration handles integration between channels and kanban board
// This is the ONLY way for channels to create tasks on the kanban board
type KanbanIntegration struct {
	kanbanService kanbantypes.KanbanBoardService
	msgBus        *bus.MessageBus
}

// NewKanbanIntegration creates a new channel-kanban integration instance
func NewKanbanIntegration(kanbanService kanbantypes.KanbanBoardService, msgBus *bus.MessageBus) *KanbanIntegration {
	return &KanbanIntegration{
		kanbanService: kanbanService,
		msgBus:        msgBus,
	}
}

// HandleCommand processes channel commands to create kanban tasks
// This is the entry point for users to create tasks via chat commands
func (ki *KanbanIntegration) HandleCommand(ctx context.Context, channel, chatID, command string, args []string) (string, error) {
	logger.InfoCF("channel_kanban", "Processing channel command for kanban",
		map[string]any{
			"channel": channel,
			"chat_id": chatID,
			"command": command,
		})

	switch strings.ToLower(command) {
	case "create", "add":
		return ki.handleCreateTask(channel, chatID, args)
	case "list", "tasks":
		return ki.handleListTasks(channel, chatID, args)
	case "status":
		return ki.handleTaskStatus(channel, chatID, args)
	default:
		return "", fmt.Errorf("unknown kanban command: %s. Available: create, list, status", command)
	}
}

// handleCreateTask creates a new task from a channel command
// Example: /kanban create "Fix bug" "High priority bug" high
func (ki *KanbanIntegration) handleCreateTask(channel, chatID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("task title required. Usage: /kanban create <title> [description] [priority]")
	}

	title := args[0]
	description := ""
	priority := 5 // default priority

	if len(args) >= 2 {
		description = args[1]
	}

	if len(args) >= 3 {
		switch strings.ToLower(args[2]) {
		case "high", "h", "1":
			priority = 10
		case "medium", "m", "5":
			priority = 5
		case "low", "l", "9":
			priority = 1
		default:
			// Try to parse as number
			fmt.Sscanf(args[2], "%d", &priority)
		}
	}

	// Create task ID with channel and chatID for traceability
	taskID := fmt.Sprintf("channel_%s_%s_%d", channel, chatID, getTimestamp())

	// Use default zone for now - in production this could be determined by channel/chatID
	zoneID := "default"

	// Create the task - THIS IS THE ONLY WAY CHANNELS CAN CREATE TASKS
	task, err := ki.kanbanService.CreateTaskWithEvent(
		zoneID,
		taskID,
		title,
		description,
		priority,
		map[string]string{
			"source":     "channel",
			"channel":    channel,
			"chat_id":    chatID,
			"created_by": "user",
		},
	)

	if err != nil {
		logger.ErrorCF("channel_kanban", "Failed to create task from channel command",
			map[string]any{
				"channel": channel,
				"chat_id": chatID,
				"error":   err.Error(),
			})
		return "", fmt.Errorf("failed to create task: %w", err)
	}

	logger.InfoCF("channel_kanban", "Task created from channel command",
		map[string]any{
			"channel": channel,
			"chat_id": chatID,
			"task_id": task.ID,
			"title":   task.Title,
		})

	return fmt.Sprintf("✅ Task created successfully!\n📋 **%s**\n🆔 ID: `%s`\n🎯 Zone: %s\n⚡ Priority: %d",
		task.Title, task.ID, zoneID, task.Priority), nil
}

// handleListTasks lists tasks in a zone
func (ki *KanbanIntegration) handleListTasks(channel, chatID string, args []string) (string, error) {
	zoneID := "default"
	statusFilter := ""

	if len(args) >= 1 {
		zoneID = args[0]
	}
	if len(args) >= 2 {
		statusFilter = args[1]
	}

	board := ki.kanbanService.GetBoard()
	zone, err := board.GetZone(zoneID)
	if err != nil {
		return "", fmt.Errorf("zone not found: %s", zoneID)
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Tasks in zone **%s**:\n\n", zone.Name))

	for _, task := range zone.Tasks {
		if statusFilter != "" && string(task.Status) != statusFilter {
			continue
		}

		statusEmoji := getStatusEmoji(task.Status)
		priorityStr := getPriorityStr(task.Priority)

		sb.WriteString(fmt.Sprintf("%s [%s] **%s**\n", statusEmoji, priorityStr, task.Title))
		sb.WriteString(fmt.Sprintf("   └─ ID: `%s` | Updated: %s\n", task.ID, task.UpdatedAt.Format("2006-01-02 15:04")))
		if task.Result != "" {
			sb.WriteString(fmt.Sprintf("   └─ Result: %s\n", truncateString(task.Result, 50)))
		}
		sb.WriteString("\n")
	}

	return sb.String(), nil
}

// handleTaskStatus shows status of a specific task
func (ki *KanbanIntegration) handleTaskStatus(channel, chatID string, args []string) (string, error) {
	if len(args) < 1 {
		return "", fmt.Errorf("task ID required. Usage: /kanban status <task-id>")
	}

	taskID := args[0]
	board := ki.kanbanService.GetBoard()

	// Search across all zones
	for zoneID, zone := range board.Zones {
		for _, task := range zone.Tasks {
			if task.ID == taskID {
				statusEmoji := getStatusEmoji(task.Status)
				return fmt.Sprintf("%s Task Status\n"+
					"━━━━━━━━━━━━━━━━━━━━━━\n"+
					"📋 **%s**\n"+
					"🆔 ID: `%s`\n"+
					"🎯 Zone: %s\n"+
					"📊 Status: %s\n"+
					"⚡ Priority: %d\n"+
					"🕐 Created: %s\n"+
					"🔄 Updated: %s\n",
					statusEmoji,
					task.Title,
					task.ID,
					zoneID,
					task.Status,
					task.Priority,
					task.CreatedAt.Format("2006-01-02 15:04"),
					task.UpdatedAt.Format("2006-01-02 15:04"),
				), nil
			}
		}
	}

	return "", fmt.Errorf("task not found: %s", taskID)
}

// SubscribeToEvents subscribes to kanban events and broadcasts to channels
func (ki *KanbanIntegration) SubscribeToEvents(ctx context.Context) {
	handler := func(msg string) {
		// Parse the event
		event, err := parseTaskEvent(msg)
		if err != nil {
			logger.ErrorCF("channel_kanban", "Failed to parse task event",
				map[string]any{"error": err.Error()})
			return
		}

		// Format notification message
		notification := formatTaskNotification(event)

		// Publish to channel broadcast
		ki.msgBus.Publish("channel.broadcast", notification)

		logger.DebugCF("channel_kanban", "Broadcasted task event to channels",
			map[string]any{
				"task_id": event.TaskID,
				"status":  event.Status,
			})
	}

	// Subscribe to kanban events
	ki.kanbanService.SubscribeToEvents(handler)

	logger.InfoCF("channel_kanban", "Subscribed to kanban events for channel broadcasting", nil)

	<-ctx.Done()
	logger.InfoCF("channel_kanban", "Channel-Kanban event subscription stopped", nil)
}

// Helper functions

func getStatusEmoji(status kanbantypes.TaskStatus) string {
	switch status {
	case kanbantypes.TaskPending:
		return "⏳"
	case kanbantypes.TaskRunning, kanbantypes.TaskInProgress:
		return "🔄"
	case kanbantypes.TaskCompleted:
		return "✅"
	case kanbantypes.TaskFailed:
		return "❌"
	case kanbantypes.TaskBlocked:
		return "🚫"
	default:
		return "❓"
	}
}

func getPriorityStr(priority int) string {
	switch {
	case priority >= 8:
		return "🔴 High"
	case priority >= 4:
		return "🟡 Medium"
	default:
		return "🟢 Low"
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func getTimestamp() int64 {
	return time.Now().UnixNano() / 1e6
}

func parseTaskEvent(msg string) (*kanbantypes.TaskEvent, error) {
	// In production, this would properly unmarshal JSON
	// For now, return a simple implementation
	event := &kanbantypes.TaskEvent{}
	// TODO: Implement proper JSON unmarshaling
	return event, nil
}

func formatTaskNotification(event *kanbantypes.TaskEvent) string {
	statusEmoji := getStatusEmoji(event.Status)

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("%s Task Update\n", statusEmoji))
	sb.WriteString("━━━━━━━━━━━━━━━━━━━━━━\n")
	sb.WriteString(fmt.Sprintf("📋 **%s**\n", event.Title))
	sb.WriteString(fmt.Sprintf("🎯 Zone: %s\n", event.ZoneID))
	sb.WriteString(fmt.Sprintf("📊 Status: %s\n", event.Status))

	if event.Result != "" {
		sb.WriteString(fmt.Sprintf("✨ Result: %s\n", truncateString(event.Result, 100)))
	}
	if event.Error != "" {
		sb.WriteString(fmt.Sprintf("⚠️ Error: %s\n", truncateString(event.Error, 100)))
	}

	return sb.String()
}
