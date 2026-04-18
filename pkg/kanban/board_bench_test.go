package kanban_test

import (
"testing"
"github.com/ilibx/octopus/pkg/kanban"
)

func BenchmarkGetPendingTasks(b *testing.B) {
board := kanban.NewKanbanBoard("test-board", "Test Board", "main-agent")

// Create zones with tasks
for i := 0; i < 10; i++ {
zoneID := string(rune('A' + i))
board.CreateZone(zoneID, "Zone "+zoneID, "Test zone", "test-agent")

// Add 100 tasks per zone, 30% pending
for j := 0; j < 100; j++ {
taskID := string(rune('A' + i)) + "-" + string(rune('0' + j%10))
status := "pending"
if j % 3 == 0 {
status = "completed"
} else if j % 3 == 1 {
status = "running"
}
board.AddTask(zoneID, taskID, "Task "+taskID, "Test task", 5, nil)
if status != "pending" {
board.UpdateTaskStatus(zoneID, taskID, kanban.TaskStatus(status), "", "")
}
}
}

b.ResetTimer()
for i := 0; i < b.N; i++ {
_ = board.GetPendingTasks()
}
}

func BenchmarkGetTasksByStatus(b *testing.B) {
board := kanban.NewKanbanBoard("test-board", "Test Board", "main-agent")

// Create zone with tasks
board.CreateZone("default", "Default Zone", "Test zone", "test-agent")

// Add 1000 tasks
for i := 0; i < 1000; i++ {
taskID := "task-" + string(rune('A' + i%26)) + string(rune('0' + i%10))
board.AddTask("default", taskID, "Task "+taskID, "Test task", 5, nil)
if i % 3 == 0 {
board.UpdateTaskStatus("default", taskID, kanban.TaskCompleted, "", "")
} else if i % 3 == 1 {
board.UpdateTaskStatus("default", taskID, kanban.TaskRunning, "", "")
}
}

b.ResetTimer()
for i := 0; i < b.N; i++ {
_ = board.GetTasksByStatus("default", kanban.TaskPending)
}
}
