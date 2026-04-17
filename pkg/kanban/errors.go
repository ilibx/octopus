package kanban

import "errors"

var (
	ErrZoneNotFound = errors.New("zone not found")
	ErrTaskNotFound = errors.New("task not found")
)
