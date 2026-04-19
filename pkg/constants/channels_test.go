package constants

import (
	"testing"
)

func TestIsInternalChannel(t *testing.T) {
	tests := []struct {
		name     string
		channel  string
		expected bool
	}{
		{"cli is internal", "cli", true},
		{"system is internal", "system", true},
		{"subagent is internal", "subagent", true},
		{"slack is not internal", "slack", false},
		{"discord is not internal", "discord", false},
		{"empty string is not internal", "", false},
		{"case sensitive - CLI is not internal", "CLI", false},
		{"case sensitive - System is not internal", "System", false},
		{"partial match is not internal", "cli-bot", false},
		{"similar name is not internal", "system-admin", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsInternalChannel(tt.channel)
			if got != tt.expected {
				t.Errorf("IsInternalChannel(%q) = %v, want %v", tt.channel, got, tt.expected)
			}
		})
	}
}
