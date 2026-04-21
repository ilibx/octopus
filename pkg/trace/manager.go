// Package trace provides distributed tracing for task execution
package trace

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/ilibx/octopus/pkg/logger"
)

// TraceContext holds the tracing context for a task execution
type TraceContext struct {
	TraceID      string            `json:"trace_id"`           // Main task ID (root trace)
	SpanID       string            `json:"span_id"`            // Current span ID
	ParentSpanID string            `json:"parent_span_id"`     // Parent span ID (for sub-tasks)
	TaskID       string            `json:"task_id"`            // Task ID
	TaskTitle    string            `json:"task_title"`         // Task title
	Status       string            `json:"status"`             // Current status
	StartTime    time.Time         `json:"start_time"`         // Start time
	EndTime      time.Time         `json:"end_time,omitempty"` // End time (if completed)
	Duration     time.Duration     `json:"duration,omitempty"` // Execution duration
	Metadata     map[string]string `json:"metadata,omitempty"`
	ChildSpans   []string          `json:"child_spans,omitempty"` // Child span IDs
}

// TraceManager manages distributed tracing for all tasks
type TraceManager struct {
	traces map[string]*TraceContext // traceID -> root trace
	spans  map[string]*TraceContext // spanID -> span context
	mu     sync.RWMutex
}

// NewTraceManager creates a new trace manager
func NewTraceManager() *TraceManager {
	return &TraceManager{
		traces: make(map[string]*TraceContext),
		spans:  make(map[string]*TraceContext),
	}
}

// StartTrace starts a new trace for a main task
func (tm *TraceManager) StartTrace(taskID, title string, metadata map[string]string) string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	traceID := taskID // Use taskID as traceID for main tasks
	if metadata != nil && metadata["trace_id"] != "" {
		traceID = metadata["trace_id"]
	}

	spanID := uuid.New().String()

	ctx := &TraceContext{
		TraceID:      traceID,
		SpanID:       spanID,
		ParentSpanID: "",
		TaskID:       taskID,
		TaskTitle:    title,
		Status:       "started",
		StartTime:    time.Now(),
		Metadata:     metadata,
		ChildSpans:   make([]string, 0),
	}

	tm.traces[traceID] = ctx
	tm.spans[spanID] = ctx

	logger.InfoCF("trace", "Trace started",
		map[string]any{
			"trace_id": traceID,
			"span_id":  spanID,
			"task_id":  taskID,
		})

	return traceID
}

// StartSubTask starts a new sub-task span under an existing trace
func (tm *TraceManager) StartSubTask(parentTraceID, taskID, title string, metadata map[string]string) string {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// Find parent trace
	parentCtx, exists := tm.traces[parentTraceID]
	if !exists {
		logger.WarnCF("trace", "Parent trace not found, creating new trace",
			map[string]any{"parent_trace_id": parentTraceID})
		// Create new trace if parent not found
		tm.mu.Unlock()
		return tm.StartTrace(taskID, title, metadata)
	}

	spanID := uuid.New().String()

	ctx := &TraceContext{
		TraceID:      parentTraceID,
		SpanID:       spanID,
		ParentSpanID: parentCtx.SpanID,
		TaskID:       taskID,
		TaskTitle:    title,
		Status:       "started",
		StartTime:    time.Now(),
		Metadata:     metadata,
		ChildSpans:   make([]string, 0),
	}

	// Add to parent's child spans
	parentCtx.ChildSpans = append(parentCtx.ChildSpans, spanID)

	tm.spans[spanID] = ctx

	logger.InfoCF("trace", "Sub-task span started",
		map[string]any{
			"trace_id":       parentTraceID,
			"span_id":        spanID,
			"parent_span_id": parentCtx.SpanID,
			"task_id":        taskID,
		})

	return spanID
}

// UpdateStatus updates the status of a span
func (tm *TraceManager) UpdateStatus(spanID, status string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	span, exists := tm.spans[spanID]
	if !exists {
		logger.WarnCF("trace", "Span not found for status update",
			map[string]any{"span_id": spanID})
		return
	}

	span.Status = status
	logger.DebugCF("trace", "Span status updated",
		map[string]any{
			"span_id":  spanID,
			"status":   status,
			"trace_id": span.TraceID,
		})
}

// EndSpan ends a span and calculates duration
func (tm *TraceManager) EndSpan(spanID string, status string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	span, exists := tm.spans[spanID]
	if !exists {
		logger.WarnCF("trace", "Span not found for ending",
			map[string]any{"span_id": spanID})
		return
	}

	span.Status = status
	span.EndTime = time.Now()
	span.Duration = span.EndTime.Sub(span.StartTime)

	logger.InfoCF("trace", "Span ended",
		map[string]any{
			"span_id":  spanID,
			"trace_id": span.TraceID,
			"status":   status,
			"duration": span.Duration.String(),
		})
}

// GetTrace retrieves a complete trace by trace ID
func (tm *TraceManager) GetTrace(traceID string) (*TraceContext, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	trace, exists := tm.traces[traceID]
	if !exists {
		return nil, ErrTraceNotFound
	}

	return trace, nil
}

// GetSpan retrieves a span by span ID
func (tm *TraceManager) GetSpan(spanID string) (*TraceContext, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	span, exists := tm.spans[spanID]
	if !exists {
		return nil, ErrSpanNotFound
	}

	return span, nil
}

// GetAllTraces returns all active traces
func (tm *TraceManager) GetAllTraces() []*TraceContext {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	traces := make([]*TraceContext, 0, len(tm.traces))
	for _, trace := range tm.traces {
		traces = append(traces, trace)
	}

	return traces
}

// GetTraceTree returns the full trace tree with all child spans
func (tm *TraceManager) GetTraceTree(traceID string) (*TraceContext, error) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	root, exists := tm.traces[traceID]
	if !exists {
		return nil, ErrTraceNotFound
	}

	// Build the full tree (in production, this would recursively fetch children)
	return root, nil
}

// CleanupOldTraces removes traces older than the specified duration
func (tm *TraceManager) CleanupOldTraces(maxAge time.Duration) int {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	now := time.Now()
	cleaned := 0

	for traceID, trace := range tm.traces {
		if !trace.EndTime.IsZero() && now.Sub(trace.EndTime) > maxAge {
			// Remove all spans for this trace
			for _, spanID := range trace.ChildSpans {
				delete(tm.spans, spanID)
			}
			delete(tm.traces, traceID)
			delete(tm.spans, trace.SpanID)
			cleaned++
		}
	}

	if cleaned > 0 {
		logger.InfoCF("trace", "Cleaned up old traces",
			map[string]any{"count": cleaned})
	}

	return cleaned
}

// AddMetadata adds metadata to a span
func (tm *TraceManager) AddMetadata(spanID string, key, value string) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	span, exists := tm.spans[spanID]
	if !exists {
		return
	}

	if span.Metadata == nil {
		span.Metadata = make(map[string]string)
	}
	span.Metadata[key] = value
}

// Errors for trace manager
type TraceError string

func (e TraceError) Error() string {
	return string(e)
}

const (
	ErrTraceNotFound TraceError = "trace not found"
	ErrSpanNotFound  TraceError = "span not found"
)
