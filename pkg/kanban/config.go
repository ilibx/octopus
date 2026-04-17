package kanban

import (
	"os"
	"strconv"
	"time"
)

// KanbanConfig holds all configurable parameters for the kanban system
type KanbanConfig struct {
	// MonitorInterval is the base interval for checking pending tasks
	MonitorInterval time.Duration
	// MaxMonitorInterval is the maximum interval when no work is found
	MaxMonitorInterval time.Duration
	// AgentStopTimeout is the timeout for stopping agents gracefully
	AgentStopTimeout time.Duration
	// MaxConcurrency is the maximum number of concurrent tasks per agent
	MaxConcurrency int
	// WebSocketEnabled enables WebSocket real-time updates
	WebSocketEnabled bool
	// WebSocketPort is the port for WebSocket server
	WebSocketPort int
	// EnableMetrics enables Prometheus metrics collection
	EnableMetrics bool
	// BoardID is the unique identifier for the board
	BoardID string
	// BoardName is the human-readable name for the board
	BoardName string
}

// DefaultKanbanConfig returns a configuration with default values
func DefaultKanbanConfig() *KanbanConfig {
	return &KanbanConfig{
		MonitorInterval:    2 * time.Second,
		MaxMonitorInterval: 10 * time.Second,
		AgentStopTimeout:   10 * time.Second,
		MaxConcurrency:     5,
		WebSocketEnabled:   false,
		WebSocketPort:      8080,
		EnableMetrics:      false,
		BoardID:            "default-board",
		BoardName:          "Default Kanban Board",
	}
}

// LoadKanbanConfigFromEnv loads configuration from environment variables
func LoadKanbanConfigFromEnv() *KanbanConfig {
	cfg := DefaultKanbanConfig()

	if val := os.Getenv("KANBAN_MONITOR_INTERVAL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.MonitorInterval = d
		}
	}

	if val := os.Getenv("KANBAN_MAX_MONITOR_INTERVAL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.MaxMonitorInterval = d
		}
	}

	if val := os.Getenv("AGENT_STOP_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.AgentStopTimeout = d
		}
	}

	if val := os.Getenv("AGENT_MAX_CONCURRENCY"); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.MaxConcurrency = i
		}
	}

	if val := os.Getenv("KANBAN_WEBSOCKET_ENABLED"); val != "" {
		cfg.WebSocketEnabled = val == "true" || val == "1"
	}

	if val := os.Getenv("KANBAN_WEBSOCKET_PORT"); val != "" {
		if i, err := strconv.Atoi(val); err == nil && i > 0 {
			cfg.WebSocketPort = i
		}
	}

	if val := os.Getenv("KANBAN_ENABLE_METRICS"); val != "" {
		cfg.EnableMetrics = val == "true" || val == "1"
	}

	if val := os.Getenv("KANBAN_BOARD_ID"); val != "" {
		cfg.BoardID = val
	}

	if val := os.Getenv("KANBAN_BOARD_NAME"); val != "" {
		cfg.BoardName = val
	}

	return cfg
}

// Validate validates the configuration and returns errors if any
func (c *KanbanConfig) Validate() error {
	if c.MonitorInterval <= 0 {
		c.MonitorInterval = 2 * time.Second
	}
	if c.MaxMonitorInterval < c.MonitorInterval {
		c.MaxMonitorInterval = c.MonitorInterval * 5
	}
	if c.AgentStopTimeout <= 0 {
		c.AgentStopTimeout = 10 * time.Second
	}
	if c.MaxConcurrency <= 0 {
		c.MaxConcurrency = 5
	}
	if c.WebSocketPort <= 0 || c.WebSocketPort > 65535 {
		c.WebSocketPort = 8080
	}
	return nil
}
