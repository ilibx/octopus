package kanban

import "errors"

var (
	ErrZoneNotFound       = errors.New("zone not found")
	ErrTaskNotFound       = errors.New("task not found")
	ErrCircularDependency = errors.New("circular dependency detected in task DAG")
)
