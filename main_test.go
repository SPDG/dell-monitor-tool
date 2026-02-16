package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestLoadConfigs(t *testing.T) {
	// Create a temporary config file
	tmpFile, err := os.CreateTemp("", "monitors*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())

	testConfigs := []MonitorConfig{
		{
			Model: "Test Monitor",
			Match: "TEST-123",
			Features: map[string]FeatureConfig{
				"brightness": {VCP: "0x10", Values: map[string]string{"high": "100"}},
			},
		},
	}
	data, _ := json.Marshal(testConfigs)
	if _, err := tmpFile.Write(data); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	configs, err := loadConfigs(tmpFile.Name())
	if err != nil {
		t.Fatalf("Failed to load configs: %v", err)
	}

	if len(configs) != 1 || configs[0].Match != "TEST-123" {
		t.Errorf("Unexpected config loaded: %+v", configs)
	}
}

func TestParseHex(t *testing.T) {
	cases := []struct {
		input    string
		expected byte
	}{
		{"0x10", 0x10},
		{"10", 0x10},
		{"0xFF", 0xFF},
		{"FF", 0xFF},
		{"0x00", 0x00},
		{"00", 0x00},
	}

	for _, c := range cases {
		got, err := parseHex(c.input)
		if err != nil {
			t.Errorf("parseHex(%s) returned error: %v", c.input, err)
			continue
		}
		if got != c.expected {
			t.Errorf("parseHex(%s) = 0x%02X; want 0x%02X", c.input, got, c.expected)
		}
	}

	if _, err := parseHex("invalid"); err == nil {
		t.Error("parseHex(invalid) should have returned an error")
	}
}

func TestParseHex16(t *testing.T) {
	cases := []struct {
		input    string
		expected uint16
	}{
		{"0x0F0F", 0x0F0F},
		{"0f0f", 0x0F0F},
		{"0x19", 0x0019},
		{"0x0000", 0x0000},
		{"0000", 0x0000},
	}

	for _, c := range cases {
		got, err := parseHex16(c.input)
		if err != nil {
			t.Errorf("parseHex16(%s) returned error: %v", c.input, err)
			continue
		}
		if got != c.expected {
			t.Errorf("parseHex16(%s) = 0x%04X; want 0x%04X", c.input, got, c.expected)
		}
	}

	if _, err := parseHex16("invalid"); err == nil {
		t.Error("parseHex16(invalid) should have returned an error")
	}
}
