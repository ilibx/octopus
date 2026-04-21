package kanban

import (
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds all Prometheus metrics for the kanban system
type Metrics struct {
	// Business metrics
	TasksCreated   prometheus.Counter
	TasksCompleted prometheus.Counter
	TasksFailed    prometheus.Counter
	AgentsSpawned  prometheus.Counter
	AgentsReleased prometheus.Counter

	// Performance metrics
	TaskLatency      prometheus.Histogram
	OrchestratorLoop prometheus.Gauge
	MutexWaitTime    prometheus.Histogram
	ZoneTaskCount    *prometheus.GaugeVec
	AgentConcurrency prometheus.Gauge
}

var (
	metricsOnce   sync.Once
	globalMetrics *Metrics
)

// NewMetrics creates a new Metrics instance and registers all metrics
func NewMetrics() *Metrics {
	m := &Metrics{
		TasksCreated: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kanban_tasks_created_total",
			Help: "Total number of tasks created",
		}),
		TasksCompleted: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kanban_tasks_completed_total",
			Help: "Total number of tasks completed successfully",
		}),
		TasksFailed: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kanban_tasks_failed_total",
			Help: "Total number of tasks failed",
		}),
		AgentsSpawned: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kanban_agents_spawned_total",
			Help: "Total number of agents spawned",
		}),
		AgentsReleased: prometheus.NewCounter(prometheus.CounterOpts{
			Name: "kanban_agents_released_total",
			Help: "Total number of agents released",
		}),
		TaskLatency: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "kanban_task_latency_seconds",
			Help:    "Latency of task completion in seconds",
			Buckets: prometheus.DefBuckets,
		}),
		OrchestratorLoop: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "kanban_orchestrator_loop_duration_seconds",
			Help: "Duration of orchestrator loop iterations",
		}),
		MutexWaitTime: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "kanban_mutex_wait_time_seconds",
			Help:    "Time spent waiting for mutex locks",
			Buckets: prometheus.DefBuckets,
		}),
		ZoneTaskCount: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "kanban_zone_task_count",
			Help: "Number of tasks in each zone by status",
		}, []string{"zone_id", "status"}),
		AgentConcurrency: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "kanban_agent_active_tasks",
			Help: "Current number of active tasks across all agents",
		}),
	}

	// Register all metrics
	prometheus.MustRegister(
		m.TasksCreated,
		m.TasksCompleted,
		m.TasksFailed,
		m.AgentsSpawned,
		m.AgentsReleased,
		m.TaskLatency,
		m.OrchestratorLoop,
		m.MutexWaitTime,
		m.ZoneTaskCount,
		m.AgentConcurrency,
	)

	return m
}

// GetGlobalMetrics returns the global metrics instance (singleton)
func GetGlobalMetrics() *Metrics {
	metricsOnce.Do(func() {
		globalMetrics = NewMetrics()
	})
	return globalMetrics
}

// RecordTaskCreated increments the tasks created counter
func (m *Metrics) RecordTaskCreated() {
	if m != nil {
		m.TasksCreated.Inc()
	}
}

// RecordTaskCompleted increments the tasks completed counter and records latency
func (m *Metrics) RecordTaskCompleted(latency time.Duration) {
	if m != nil {
		m.TasksCompleted.Inc()
		m.TaskLatency.Observe(latency.Seconds())
	}
}

// RecordTaskFailed increments the tasks failed counter
func (m *Metrics) RecordTaskFailed() {
	if m != nil {
		m.TasksFailed.Inc()
	}
}

// RecordAgentSpawned increments the agents spawned counter
func (m *Metrics) RecordAgentSpawned() {
	if m != nil {
		m.AgentsSpawned.Inc()
	}
}

// RecordAgentReleased increments the agents released counter
func (m *Metrics) RecordAgentReleased() {
	if m != nil {
		m.AgentsReleased.Inc()
	}
}

// RecordOrchestratorLoop records the duration of an orchestrator loop iteration
func (m *Metrics) RecordOrchestratorLoop(duration time.Duration) {
	if m != nil {
		m.OrchestratorLoop.Set(duration.Seconds())
	}
}

// RecordMutexWaitTime records the time spent waiting for a mutex
func (m *Metrics) RecordMutexWaitTime(duration time.Duration) {
	if m != nil {
		m.MutexWaitTime.Observe(duration.Seconds())
	}
}

// UpdateZoneTaskCount updates the gauge for task count in a zone
func (m *Metrics) UpdateZoneTaskCount(zoneID string, status TaskStatus, count int) {
	if m != nil {
		m.ZoneTaskCount.WithLabelValues(zoneID, string(status)).Set(float64(count))
	}
}

// UpdateAgentConcurrency updates the current agent concurrency gauge
func (m *Metrics) UpdateAgentConcurrency(count int) {
	if m != nil {
		m.AgentConcurrency.Set(float64(count))
	}
}

// ClearZoneMetrics clears all metrics for a specific zone
func (m *Metrics) ClearZoneMetrics(zoneID string) {
	if m != nil {
		statuses := []TaskStatus{TaskPending, TaskRunning, TaskCompleted, TaskFailed, TaskBlocked, TaskInProgress}
		for _, status := range statuses {
			m.ZoneTaskCount.DeleteLabelValues(zoneID, string(status))
		}
	}
}
