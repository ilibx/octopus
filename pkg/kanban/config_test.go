package kanban

import (
	"os"
	"testing"
	"time"
)

func TestDefaultKanbanConfig(t *testing.T) {
	cfg := DefaultKanbanConfig()

	if cfg.MonitorInterval != 2*time.Second {
		t.Errorf("Expected MonitorInterval to be 2s, got %v", cfg.MonitorInterval)
	}
	if cfg.MaxMonitorInterval != 10*time.Second {
		t.Errorf("Expected MaxMonitorInterval to be 10s, got %v", cfg.MaxMonitorInterval)
	}
	if cfg.AgentStopTimeout != 10*time.Second {
		t.Errorf("Expected AgentStopTimeout to be 10s, got %v", cfg.AgentStopTimeout)
	}
	if cfg.MaxConcurrency != 5 {
		t.Errorf("Expected MaxConcurrency to be 5, got %d", cfg.MaxConcurrency)
	}
	if cfg.WebSocketEnabled != false {
		t.Errorf("Expected WebSocketEnabled to be false, got %v", cfg.WebSocketEnabled)
	}
	if cfg.WebSocketPort != 8080 {
		t.Errorf("Expected WebSocketPort to be 8080, got %d", cfg.WebSocketPort)
	}
	if cfg.EnableMetrics != false {
		t.Errorf("Expected EnableMetrics to be false, got %v", cfg.EnableMetrics)
	}
	if cfg.BoardID != "default-board" {
		t.Errorf("Expected BoardID to be 'default-board', got %s", cfg.BoardID)
	}
	if cfg.BoardName != "Default Kanban Board" {
		t.Errorf("Expected BoardName to be 'Default Kanban Board', got %s", cfg.BoardName)
	}
}

func TestLoadKanbanConfigFromEnv(t *testing.T) {
	// Set environment variables
	os.Setenv("KANBAN_MONITOR_INTERVAL", "5s")
	os.Setenv("KANBAN_MAX_MONITOR_INTERVAL", "30s")
	os.Setenv("AGENT_STOP_TIMEOUT", "20s")
	os.Setenv("AGENT_MAX_CONCURRENCY", "10")
	os.Setenv("KANBAN_WEBSOCKET_ENABLED", "true")
	os.Setenv("KANBAN_WEBSOCKET_PORT", "9090")
	os.Setenv("KANBAN_ENABLE_METRICS", "true")
	os.Setenv("KANBAN_BOARD_ID", "test-board")
	os.Setenv("KANBAN_BOARD_NAME", "Test Board")

	defer func() {
		os.Unsetenv("KANBAN_MONITOR_INTERVAL")
		os.Unsetenv("KANBAN_MAX_MONITOR_INTERVAL")
		os.Unsetenv("AGENT_STOP_TIMEOUT")
		os.Unsetenv("AGENT_MAX_CONCURRENCY")
		os.Unsetenv("KANBAN_WEBSOCKET_ENABLED")
		os.Unsetenv("KANBAN_WEBSOCKET_PORT")
		os.Unsetenv("KANBAN_ENABLE_METRICS")
		os.Unsetenv("KANBAN_BOARD_ID")
		os.Unsetenv("KANBAN_BOARD_NAME")
	}()

	cfg := LoadKanbanConfigFromEnv()

	if cfg.MonitorInterval != 5*time.Second {
		t.Errorf("Expected MonitorInterval to be 5s, got %v", cfg.MonitorInterval)
	}
	if cfg.MaxMonitorInterval != 30*time.Second {
		t.Errorf("Expected MaxMonitorInterval to be 30s, got %v", cfg.MaxMonitorInterval)
	}
	if cfg.AgentStopTimeout != 20*time.Second {
		t.Errorf("Expected AgentStopTimeout to be 20s, got %v", cfg.AgentStopTimeout)
	}
	if cfg.MaxConcurrency != 10 {
		t.Errorf("Expected MaxConcurrency to be 10, got %d", cfg.MaxConcurrency)
	}
	if cfg.WebSocketEnabled != true {
		t.Errorf("Expected WebSocketEnabled to be true, got %v", cfg.WebSocketEnabled)
	}
	if cfg.WebSocketPort != 9090 {
		t.Errorf("Expected WebSocketPort to be 9090, got %d", cfg.WebSocketPort)
	}
	if cfg.EnableMetrics != true {
		t.Errorf("Expected EnableMetrics to be true, got %v", cfg.EnableMetrics)
	}
	if cfg.BoardID != "test-board" {
		t.Errorf("Expected BoardID to be 'test-board', got %s", cfg.BoardID)
	}
	if cfg.BoardName != "Test Board" {
		t.Errorf("Expected BoardName to be 'Test Board', got %s", cfg.BoardName)
	}
}

func TestLoadKanbanConfigFromEnv_InvalidValues(t *testing.T) {
	// Set invalid environment variables
	os.Setenv("KANBAN_MONITOR_INTERVAL", "invalid")
	os.Setenv("AGENT_MAX_CONCURRENCY", "-5")
	os.Setenv("KANBAN_WEBSOCKET_PORT", "99999")

	defer func() {
		os.Unsetenv("KANBAN_MONITOR_INTERVAL")
		os.Unsetenv("AGENT_MAX_CONCURRENCY")
		os.Unsetenv("KANBAN_WEBSOCKET_PORT")
	}()

	cfg := LoadKanbanConfigFromEnv()

	// Should fall back to defaults for invalid values
	if cfg.MonitorInterval != 2*time.Second {
		t.Errorf("Expected MonitorInterval to fallback to 2s, got %v", cfg.MonitorInterval)
	}
	if cfg.MaxConcurrency != 5 {
		t.Errorf("Expected MaxConcurrency to fallback to 5, got %d", cfg.MaxConcurrency)
	}
	if cfg.WebSocketPort != 8080 {
		t.Errorf("Expected WebSocketPort to fallback to 8080, got %d", cfg.WebSocketPort)
	}
}

func TestKanbanConfig_Validate(t *testing.T) {
	tests := []struct {
		name     string
		config   *KanbanConfig
		expected KanbanConfig
	}{
		{
			name:   "Valid config",
			config: DefaultKanbanConfig(),
			expected: KanbanConfig{
				MonitorInterval:    2 * time.Second,
				MaxMonitorInterval: 10 * time.Second,
				AgentStopTimeout:   10 * time.Second,
				MaxConcurrency:     5,
				WebSocketPort:      8080,
			},
		},
		{
			name: "Zero MonitorInterval",
			config: &KanbanConfig{
				MonitorInterval: 0,
			},
			expected: KanbanConfig{
				MonitorInterval: 2 * time.Second,
			},
		},
		{
			name: "MaxMonitorInterval less than MonitorInterval",
			config: &KanbanConfig{
				MonitorInterval:    5 * time.Second,
				MaxMonitorInterval: 2 * time.Second,
			},
			expected: KanbanConfig{
				MonitorInterval:    5 * time.Second,
				MaxMonitorInterval: 25 * time.Second,
			},
		},
		{
			name: "Zero AgentStopTimeout",
			config: &KanbanConfig{
				AgentStopTimeout: 0,
			},
			expected: KanbanConfig{
				AgentStopTimeout: 10 * time.Second,
			},
		},
		{
			name: "Zero MaxConcurrency",
			config: &KanbanConfig{
				MaxConcurrency: 0,
			},
			expected: KanbanConfig{
				MaxConcurrency: 5,
			},
		},
		{
			name: "Invalid WebSocketPort (too high)",
			config: &KanbanConfig{
				WebSocketPort: 70000,
			},
			expected: KanbanConfig{
				WebSocketPort: 8080,
			},
		},
		{
			name: "Invalid WebSocketPort (zero)",
			config: &KanbanConfig{
				WebSocketPort: 0,
			},
			expected: KanbanConfig{
				WebSocketPort: 8080,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if err != nil {
				t.Errorf("Validate() returned unexpected error: %v", err)
			}

			if tt.config.MonitorInterval != tt.expected.MonitorInterval {
				t.Errorf("Expected MonitorInterval %v, got %v", tt.expected.MonitorInterval, tt.config.MonitorInterval)
			}
			if tt.expected.MaxMonitorInterval != 0 && tt.config.MaxMonitorInterval != tt.expected.MaxMonitorInterval {
				t.Errorf("Expected MaxMonitorInterval %v, got %v", tt.expected.MaxMonitorInterval, tt.config.MaxMonitorInterval)
			}
			if tt.expected.AgentStopTimeout != 0 && tt.config.AgentStopTimeout != tt.expected.AgentStopTimeout {
				t.Errorf("Expected AgentStopTimeout %v, got %v", tt.expected.AgentStopTimeout, tt.config.AgentStopTimeout)
			}
			if tt.expected.MaxConcurrency != 0 && tt.config.MaxConcurrency != tt.expected.MaxConcurrency {
				t.Errorf("Expected MaxConcurrency %d, got %d", tt.expected.MaxConcurrency, tt.config.MaxConcurrency)
			}
			if tt.expected.WebSocketPort != 0 && tt.config.WebSocketPort != tt.expected.WebSocketPort {
				t.Errorf("Expected WebSocketPort %d, got %d", tt.expected.WebSocketPort, tt.config.WebSocketPort)
			}
		})
	}
}

func TestLoadKanbanConfigFromEnv_EmptyString(t *testing.T) {
	// Empty strings should use defaults
	os.Setenv("KANBAN_MONITOR_INTERVAL", "")
	os.Setenv("AGENT_MAX_CONCURRENCY", "")

	defer func() {
		os.Unsetenv("KANBAN_MONITOR_INTERVAL")
		os.Unsetenv("AGENT_MAX_CONCURRENCY")
	}()

	cfg := LoadKanbanConfigFromEnv()

	if cfg.MonitorInterval != 2*time.Second {
		t.Errorf("Expected MonitorInterval to be default 2s, got %v", cfg.MonitorInterval)
	}
	if cfg.MaxConcurrency != 5 {
		t.Errorf("Expected MaxConcurrency to be default 5, got %d", cfg.MaxConcurrency)
	}
}

func TestLoadKanbanConfigFromEnv_BooleanValues(t *testing.T) {
	tests := []struct {
		value    string
		expected bool
	}{
		{"true", true},
		{"1", true},
		{"false", false},
		{"0", false},
		{"yes", false},
		{"no", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			os.Setenv("KANBAN_WEBSOCKET_ENABLED", tt.value)
			defer os.Unsetenv("KANBAN_WEBSOCKET_ENABLED")

			cfg := LoadKanbanConfigFromEnv()
			if cfg.WebSocketEnabled != tt.expected {
				t.Errorf("Expected WebSocketEnabled to be %v for value '%s', got %v", tt.expected, tt.value, cfg.WebSocketEnabled)
			}
		})
	}
}
