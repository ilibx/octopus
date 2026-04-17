package kanban

import (
"testing"
"time"
)

func TestNewKanbanBoard(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")

if board.ID != "test-board" {
t.Errorf("Expected ID 'test-board', got '%s'", board.ID)
}

if board.Name != "Test Board" {
t.Errorf("Expected Name 'Test Board', got '%s'", board.Name)
}

if board.MainAgentID != "agent-1" {
t.Errorf("Expected MainAgentID 'agent-1', got '%s'", board.MainAgentID)
}

if len(board.Zones) != 0 {
t.Errorf("Expected empty zones, got %d zones", len(board.Zones))
}
}

func TestCreateZone(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")

zone := board.CreateZone("zone-1", "Development", "Development tasks", "developer")

if zone == nil {
t.Fatal("Expected zone to be created, got nil")
}

if zone.ID != "zone-1" {
t.Errorf("Expected zone ID 'zone-1', got '%s'", zone.ID)
}

if zone.AgentType != "developer" {
t.Errorf("Expected AgentType 'developer', got '%s'", zone.AgentType)
}

if !zone.Active {
t.Error("Expected zone to be active")
}
}

func TestAddTask(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")

metadata := map[string]string{"priority_label": "high"}
task, err := board.AddTask("zone-1", "task-1", "Implement feature", "Add new feature", 5, metadata)

if err != nil {
t.Fatalf("Expected no error, got %v", err)
}

if task == nil {
t.Fatal("Expected task to be created, got nil")
}

if task.Title != "Implement feature" {
t.Errorf("Expected title 'Implement feature', got '%s'", task.Title)
}

if task.Status != TaskPending {
t.Errorf("Expected status 'pending', got '%s'", task.Status)
}

if task.Priority != 5 {
t.Errorf("Expected priority 5, got %d", task.Priority)
}
}

func TestAddTaskToNonExistentZone(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")

_, err := board.AddTask("non-existent-zone", "task-1", "Task", "Description", 1, nil)

if err != ErrZoneNotFound {
t.Errorf("Expected ErrZoneNotFound, got %v", err)
}
}

func TestUpdateTaskStatus(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")
board.AddTask("zone-1", "task-1", "Task", "Description", 5, nil)

err := board.UpdateTaskStatus("zone-1", "task-1", TaskCompleted, "Success", "")

if err != nil {
t.Fatalf("Expected no error, got %v", err)
}

zone, _ := board.GetZone("zone-1")
var updatedTask *Task
for _, t := range zone.Tasks {
if t.ID == "task-1" {
updatedTask = t
break
}
}

if updatedTask.Status != TaskCompleted {
t.Errorf("Expected status 'completed', got '%s'", updatedTask.Status)
}

if updatedTask.Result != "Success" {
t.Errorf("Expected result 'Success', got '%s'", updatedTask.Result)
}
}

func TestGetTasksByStatus(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")

board.AddTask("zone-1", "task-1", "Task 1", "Description 1", 5, nil)
board.AddTask("zone-1", "task-2", "Task 2", "Description 2", 3, nil)
board.UpdateTaskStatus("zone-1", "task-1", TaskCompleted, "", "")

pendingTasks := board.GetTasksByStatus("zone-1", TaskPending)
if len(pendingTasks) != 1 {
t.Errorf("Expected 1 pending task, got %d", len(pendingTasks))
}

completedTasks := board.GetTasksByStatus("zone-1", TaskCompleted)
if len(completedTasks) != 1 {
t.Errorf("Expected 1 completed task, got %d", len(completedTasks))
}
}

func TestGetPendingTasks(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Zone 1", "Description 1", "type-1")
board.CreateZone("zone-2", "Zone 2", "Description 2", "type-2")

board.AddTask("zone-1", "task-1", "Task 1", "Desc 1", 5, nil)
board.AddTask("zone-2", "task-2", "Task 2", "Desc 2", 3, nil)
board.UpdateTaskStatus("zone-1", "task-1", TaskCompleted, "", "")

pending := board.GetPendingTasks()

if len(pending) != 1 {
t.Errorf("Expected 1 zone with pending tasks, got %d", len(pending))
}

if len(pending["zone-2"]) != 1 {
t.Errorf("Expected 1 pending task in zone-2, got %d", len(pending["zone-2"]))
}
}

func TestHasActiveAgent(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")

// Initially no active agent
if board.HasActiveAgent("zone-1") {
t.Error("Expected no active agent initially")
}

board.AddTask("zone-1", "task-1", "Task", "Description", 5, nil)
board.UpdateTaskStatus("zone-1", "task-1", TaskRunning, "", "")

// Now should have active agent
if !board.HasActiveAgent("zone-1") {
t.Error("Expected active agent when task is running")
}
}

func TestGetZoneAgentType(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")

agentType, err := board.GetZoneAgentType("zone-1")

if err != nil {
t.Fatalf("Expected no error, got %v", err)
}

if agentType != "developer" {
t.Errorf("Expected agent type 'developer', got '%s'", agentType)
}

_, err = board.GetZoneAgentType("non-existent")
if err != ErrZoneNotFound {
t.Errorf("Expected ErrZoneNotFound, got %v", err)
}
}

func TestConcurrentAccess(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")

done := make(chan bool)

// Start multiple goroutines adding tasks
for i := 0; i < 10; i++ {
go func(id int) {
taskID := string(rune('a' + id))
board.AddTask("zone-1", taskID, "Task", "Description", 5, nil)
done <- true
}(i)
}

// Wait for all goroutines to complete
for i := 0; i < 10; i++ {
<-done
}

zone, _ := board.GetZone("zone-1")
if len(zone.Tasks) != 10 {
t.Errorf("Expected 10 tasks, got %d", len(zone.Tasks))
}
}

func TestTaskTimestamps(t *testing.T) {
board := NewKanbanBoard("test-board", "Test Board", "agent-1")
board.CreateZone("zone-1", "Development", "Development tasks", "developer")

beforeAdd := time.Now()
task, _ := board.AddTask("zone-1", "task-1", "Task", "Description", 5, nil)
afterAdd := time.Now()

if task.CreatedAt.Before(beforeAdd) || task.CreatedAt.After(afterAdd) {
t.Error("Task CreatedAt timestamp not within expected range")
}

// Update status and check UpdatedAt
time.Sleep(10 * time.Millisecond)
beforeUpdate := time.Now()
board.UpdateTaskStatus("zone-1", "task-1", TaskCompleted, "", "")
afterUpdate := time.Now()

zone, _ := board.GetZone("zone-1")
var updatedTask *Task
for _, t := range zone.Tasks {
if t.ID == "task-1" {
updatedTask = t
break
}
}

if updatedTask.UpdatedAt.Before(beforeUpdate) || updatedTask.UpdatedAt.After(afterUpdate) {
t.Error("Task UpdatedAt timestamp not within expected range after update")
}
}
