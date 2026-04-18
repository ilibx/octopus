package kanban

import (
	"sync"
)

// TaskPool represents a pool of reusable Task objects to reduce GC pressure
type TaskPool struct {
	pool sync.Pool
}

// NewTaskPool creates a new task object pool
func NewTaskPool() *TaskPool {
	return &TaskPool{
		pool: sync.Pool{
			New: func() interface{} {
				return &Task{
					Metadata: make(map[string]string),
				}
			},
		},
	}
}

// Get retrieves a Task from the pool
func (tp *TaskPool) Get() *Task {
	return tp.pool.Get().(*Task)
}

// Put returns a Task to the pool after resetting it
func (tp *TaskPool) Put(t *Task) {
	t.ID = ""
	t.Title = ""
	t.Description = ""
	t.Status = ""
	t.Priority = 0
	t.AssignedTo = ""
	t.Result = ""
	t.Error = ""
	if t.Metadata != nil {
		for k := range t.Metadata {
			delete(t.Metadata, k)
		}
	}
	tp.pool.Put(t)
}
