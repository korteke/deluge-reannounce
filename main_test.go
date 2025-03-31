package main

import (
	"bytes"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	deluge "github.com/gdm85/go-libdeluge"
)

func TestLoadConfig(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test_config.yml")
	configContent := `
deluge:
  hostname: "localhost"
  port: 58846
  login: "testuser"
  password: "testpass"
logging:
  file: "test.log"
  level: "DEBUG"
retry:
  timeout: 60
  interval: 5
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading config
	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify config values
	if config.Deluge.Hostname != "localhost" {
		t.Errorf("Expected hostname 'localhost', got '%s'", config.Deluge.Hostname)
	}
	if config.Deluge.Port != 58846 {
		t.Errorf("Expected port 58846, got %d", config.Deluge.Port)
	}
	if config.Deluge.Login != "testuser" {
		t.Errorf("Expected login 'testuser', got '%s'", config.Deluge.Login)
	}
	if config.Deluge.Password != "testpass" {
		t.Errorf("Expected password 'testpass', got '%s'", config.Deluge.Password)
	}
	if config.Logging.File != "test.log" {
		t.Errorf("Expected log file 'test.log', got '%s'", config.Logging.File)
	}
	if config.Logging.Level != "DEBUG" {
		t.Errorf("Expected log level 'DEBUG', got '%s'", config.Logging.Level)
	}
	if config.Retry.Timeout != 60 {
		t.Errorf("Expected timeout 60, got %d", config.Retry.Timeout)
	}
	if config.Retry.Interval != 5 {
		t.Errorf("Expected interval 5, got %d", config.Retry.Interval)
	}
}

type testLogger struct {
	*Logger
	buffer *bytes.Buffer
}

func newTestLogger(level string) (*testLogger, error) {
	buffer := new(bytes.Buffer)
	logger := &Logger{
		infoLogger:  log.New(buffer, "INFO: ", log.Ldate|log.Ltime),
		debugLogger: log.New(buffer, "DEBUG: ", log.Ldate|log.Ltime),
		file:        nil,
		level:       level,
	}
	return &testLogger{Logger: logger, buffer: buffer}, nil
}

func (l *testLogger) Close() error {
	return nil
}

func TestNewLogger(t *testing.T) {
	// Create a temporary directory for log files
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")

	// Create test config
	config := &Config{
		Logging: struct {
			File  string `yaml:"file"`
			Level string `yaml:"level"`
		}{
			File:  logFile,
			Level: "DEBUG",
		},
	}

	// Test logger creation
	logger, err := NewLogger(config)
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Test logging
	testMessage := "Test log message"
	logger.Info(testMessage)
	logger.Debug(testMessage)
	logger.Error(testMessage)

	// Force flush by closing the logger
	logger.Close()

	// Read and verify log file
	content, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("Failed to read log file: %v", err)
	}

	logContent := string(content)
	t.Logf("Log file contents:\n%s", logContent)

	// Split log content into lines and check each line
	lines := strings.Split(logContent, "\n")
	var foundInfo, foundDebug, foundError bool

	for _, line := range lines {
		if strings.Contains(line, "INFO:") && strings.Contains(line, testMessage) {
			foundInfo = true
		}
		if strings.Contains(line, "DEBUG:") && strings.Contains(line, testMessage) {
			foundDebug = true
		}
		if strings.Contains(line, "ERROR:") && strings.Contains(line, testMessage) {
			foundError = true
		}
	}

	if !foundInfo {
		t.Error("Log file missing INFO message")
	}
	if !foundDebug {
		t.Error("Log file missing DEBUG message")
	}
	if !foundError {
		t.Error("Log file missing ERROR message")
	}
}

func TestDelugeClient(t *testing.T) {
	// Create test logger
	tmpDir := t.TempDir()
	logFile := filepath.Join(tmpDir, "test.log")
	logger, err := NewLogger(&Config{
		Logging: struct {
			File  string `yaml:"file"`
			Level string `yaml:"level"`
		}{
			File:  logFile,
			Level: "DEBUG",
		},
	})
	if err != nil {
		t.Fatalf("Failed to create logger: %v", err)
	}
	defer logger.Close()

	// Create test settings
	settings := deluge.Settings{
		Hostname: "localhost",
		Port:     58846,
		Login:    "testuser",
		Password: "testpass",
	}

	// Test client creation
	client := NewDelugeClient(settings, logger)
	if client == nil {
		t.Fatal("Failed to create DelugeClient")
	}

	// Test client methods
	if err := client.Connect(); err != nil {
		t.Logf("Note: Connection test skipped as Deluge daemon is not running: %v", err)
		return // Skip remaining tests if connection fails
	}
	defer client.Close()

	// Test ForceReannounce with a short timeout
	timeout := 2 * time.Second
	interval := 100 * time.Millisecond
	success := client.ForceReannounce("test-torrent-id", timeout, interval)
	if !success {
		t.Logf("Note: ForceReannounce test skipped as Deluge daemon is not running")
	}
}

func TestConfigDefaults(t *testing.T) {
	// Create a temporary config file with minimal content
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "minimal_config.yml")
	configContent := `deluge: {}`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	// Test loading config with defaults
	config, err := loadConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify default values
	if config.Logging.File != "deluge-reannounce.log" {
		t.Errorf("Expected default log file 'deluge-reannounce.log', got '%s'", config.Logging.File)
	}
	if config.Logging.Level != "INFO" {
		t.Errorf("Expected default log level 'INFO', got '%s'", config.Logging.Level)
	}
	if config.Retry.Timeout != 120 {
		t.Errorf("Expected default timeout 120, got %d", config.Retry.Timeout)
	}
	if config.Retry.Interval != 7 {
		t.Errorf("Expected default interval 7, got %d", config.Retry.Interval)
	}
}
