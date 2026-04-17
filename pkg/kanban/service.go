package kanban

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ilibx/octopus/pkg/bus"
	"github.com/ilibx/octopus/pkg/logger"
)

// TaskEvent represents an event published when task status changes
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

// KanbanService manages the kanban board and publishes events
type KanbanService struct {
	board   *KanbanBoard
	msgBus  *bus.MessageBus
	mu      sync.RWMutex
	subject string
}

// NewKanbanService creates a new kanban service
func NewKanbanService(board *KanbanBoard, msgBus *bus.MessageBus) *KanbanService {
	return &KanbanService{
		board:   board,
		msgBus:  msgBus,
		subject: "kanban.events",
	}
}

// PublishTaskEvent publishes a task event to the message bus
func (s *KanbanService) PublishTaskEvent(eventType, zoneID, taskID string, status TaskStatus, title, result, errMsg string) {
	event := TaskEvent{
		Type:      eventType,
		BoardID:   s.board.ID,
		ZoneID:    zoneID,
		TaskID:    taskID,
		Status:    status,
		Title:     title,
		Result:    result,
		Error:     errMsg,
		Timestamp: time.Now().Unix(),
	}

	data, err := json.Marshal(event)
	if err != nil {
		logger.ErrorCF("kanban", "Failed to marshal task event",
			map[string]any{"error": err.Error()})
		return
	}

	s.msgBus.Publish(s.subject, string(data))
	logger.DebugCF("kanban", "Published task event",
		map[string]any{
			"event_type": eventType,
			"zone_id":    zoneID,
			"task_id":    taskID,
		})
}

// CreateTaskWithEvent creates a task and publishes an event
func (s *KanbanService) CreateTaskWithEvent(zoneID, taskID, title, description string, priority int, metadata map[string]string) (*Task, error) {
	task, err := s.board.AddTask(zoneID, taskID, title, description, priority, metadata)
	if err != nil {
		return nil, err
	}

	s.PublishTaskEvent("task_created", zoneID, taskID, TaskPending, title, "", "")
	return task, nil
}

// UpdateTaskStatusWithEvent updates task status and publishes an event
func (s *KanbanService) UpdateTaskStatusWithEvent(zoneID, taskID string, status TaskStatus, result, errorMsg string) error {
	err := s.board.UpdateTaskStatus(zoneID, taskID, status, result, errorMsg)
	if err != nil {
		return err
	}

	eventType := "task_updated"
	if status == TaskCompleted || status == TaskFailed {
		eventType = "task_completed"
	}

	var title string
	zone, err := s.board.GetZone(zoneID)
	if err == nil {
		for _, task := range zone.Tasks {
			if task.ID == taskID {
				title = task.Title
				break
			}
		}
	}

	s.PublishTaskEvent(eventType, zoneID, taskID, status, title, result, errorMsg)
	return nil
}

// SubscribeToEvents subscribes to kanban events
func (s *KanbanService) SubscribeToEvents(handler func(string)) {
	s.msgBus.Subscribe(s.subject, handler)
}

// GetBoard returns the underlying kanban board
func (s *KanbanService) GetBoard() *KanbanBoard {
	return s.board
}

// HTTPHandler returns an HTTP handler for kanban API endpoints
func (s *KanbanService) HTTPHandler() http.Handler {
	mux := http.NewServeMux()

	// GET /kanban - Get board status
	mux.HandleFunc("/kanban", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		s.mu.RLock()
		board := s.board
		s.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(board)
	})

	// GET /kanban/zones/{zoneID} - Get zone details
	mux.HandleFunc("/kanban/zones/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		zoneID := r.URL.Path[len("/kanban/zones/"):]
		if zoneID == "" {
			http.Error(w, "Zone ID required", http.StatusBadRequest)
			return
		}

		zone, err := s.board.GetZone(zoneID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(zone)
	})

	// GET /kanban/zones/{zoneID}/tasks - Get tasks in a zone
	mux.HandleFunc("/kanban/tasks/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse zoneID and optional status from query params
		zoneID := r.URL.Query().Get("zone")
		statusStr := r.URL.Query().Get("status")

		if zoneID == "" {
			http.Error(w, "Zone ID required", http.StatusBadRequest)
			return
		}

		var tasks []*Task
		if statusStr != "" {
			tasks = s.board.GetTasksByStatus(zoneID, TaskStatus(statusStr))
		} else {
			zone, err := s.board.GetZone(zoneID)
			if err != nil {
				http.Error(w, err.Error(), http.StatusNotFound)
				return
			}
			tasks = zone.Tasks
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tasks)
	})

	return mux
}

// StartStatusReporter starts a goroutine that reports task status changes to channels
func (s *KanbanService) StartStatusReporter(ctx context.Context) {
	go func() {
		handler := func(msg string) {
			var event TaskEvent
			if err := json.Unmarshal([]byte(msg), &event); err != nil {
				logger.ErrorCF("kanban", "Failed to unmarshal task event",
					map[string]any{"error": err.Error()})
				return
			}

			// Format status report message
			report := fmt.Sprintf("📋 Task Update: %s\nZone: %s\nStatus: %s",
				event.Title, event.ZoneID, event.Status)

			if event.Result != "" {
				report += fmt.Sprintf("\nResult: %s", event.Result)
			}
			if event.Error != "" {
				report += fmt.Sprintf("\nError: %s", event.Error)
			}

			// Publish to message bus for channels to pick up
			s.msgBus.Publish("channel.broadcast", report)
			logger.InfoCF("kanban", "Status reported to channels",
				map[string]any{"task_id": event.TaskID, "status": event.Status})
		}

		s.SubscribeToEvents(handler)

		<-ctx.Done()
	}()
}
