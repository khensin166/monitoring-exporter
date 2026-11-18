package discovery

import (
	"testing"

	"docker-exporter/internal/model"
)

func TestSanitizeContainerName(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{input: []string{"/cams-tps-aggregator"}, expected: "cams-tps-aggregator"},
		{input: []string{"grafana-screenshot-v3"}, expected: "grafana-screenshot-v3"},
		{input: []string{"//nested/container"}, expected: "/nested/container"},
		{input: []string{""}, expected: "unnamed"},
		{input: []string{}, expected: "unnamed"},
	}

	for _, tt := range tests {
		result := SanitizeContainerName(tt.input)
		if result != tt.expected {
			t.Errorf("SanitizeContainerName(%v) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

func TestShortID(t *testing.T) {
	longID := "6e56dcb89a2b1234567890abcdef1234567890abcdef1234567890abcdef1234"
	expected := "6e56dcb89a2b"

	if res := ShortID(longID); res != expected {
		t.Errorf("ShortID(%s) = %s, expected %s", longID, res, expected)
	}

	short := "abc"
	if res := ShortID(short); res != short {
		t.Errorf("ShortID(%s) = %s, expected %s", short, res, short)
	}
}

func TestParseHealthStatus(t *testing.T) {
	tests := []struct {
		input    string
		expected float64
	}{
		{"healthy", 1.0},
		{"unhealthy", 0.0},
		{"starting", 2.0},
		{"none", -1.0},
		{"", -1.0},
		{"unknown", -1.0},
	}

	for _, tt := range tests {
		val := model.ParseHealthStatus(tt.input)
		if val != tt.expected {
			t.Errorf("ParseHealthStatus(%s) = %f, expected %f", tt.input, val, tt.expected)
		}
	}
}
