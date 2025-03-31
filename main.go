package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	deluge "github.com/gdm85/go-libdeluge"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Deluge struct {
		Hostname string `yaml:"hostname"`
		Port     uint   `yaml:"port"`
		Login    string `yaml:"login"`
		Password string `yaml:"password"`
	} `yaml:"deluge"`
	Logging struct {
		File  string `yaml:"file"`
		Level string `yaml:"level"`
	} `yaml:"logging"`
	Retry struct {
		Timeout  int `yaml:"timeout"`  // in seconds
		Interval int `yaml:"interval"` // in seconds
	} `yaml:"retry"`
}

type Logger struct {
	infoLogger  *log.Logger
	debugLogger *log.Logger
	file        *os.File
	level       string
}

func NewLogger(config *Config) (*Logger, error) {
	// Create log directory if it doesn't exist
	logDir := filepath.Dir(config.Logging.File)
	if logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %v", err)
		}
	}

	// Open log file
	file, err := os.OpenFile(config.Logging.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %v", err)
	}

	// Create loggers
	infoLogger := log.New(file, "INFO: ", log.Ldate|log.Ltime)
	debugLogger := log.New(file, "DEBUG: ", log.Ldate|log.Ltime)

	return &Logger{
		infoLogger:  infoLogger,
		debugLogger: debugLogger,
		file:        file,
		level:       strings.ToUpper(config.Logging.Level),
	}, nil
}

func (l *Logger) Info(format string, v ...interface{}) {
	l.infoLogger.Printf(format, v...)
}

func (l *Logger) Debug(format string, v ...interface{}) {
	l.debugLogger.Printf(format, v...)
}

func (l *Logger) Error(format string, v ...interface{}) {
	if l.level == "DEBUG" {
		l.debugLogger.Printf("ERROR: "+format, v...)
	}
}

func (l *Logger) Close() error {
	return l.file.Close()
}

func loadConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %v", configFile, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	// Set default values
	if config.Logging.File == "" {
		config.Logging.File = "deluge-reannounce.log"
	}
	if config.Logging.Level == "" {
		config.Logging.Level = "INFO"
	}
	if config.Retry.Timeout == 0 {
		config.Retry.Timeout = 120
	}
	if config.Retry.Interval == 0 {
		config.Retry.Interval = 7
	}

	return &config, nil
}

func main() {
	// Get the directory where the executable is located
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)

	// Parse command line flags
	configFile := flag.String("c", filepath.Join(execDir, "config.yml"), "Path to config file")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize logger
	logger, err := NewLogger(config)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Parse command line flags (these will override config file values)
	host := flag.String("host", config.Deluge.Hostname, "Deluge daemon host")
	port := flag.Uint("port", config.Deluge.Port, "Deluge daemon port")
	flag.Parse()

	// Check required arguments
	if len(flag.Args()) != 3 {
		logger.Info("Usage: %s <torrent_id> <torrent_name> <download_folder>", os.Args[0])
		os.Exit(1)
	}

	torrentID := flag.Arg(0)
	torrentName := flag.Arg(1)
	downloadFolder := flag.Arg(2)

	// Log the incoming parameters
	logger.Info("Received reannounce request for torrent: %s (ID: %s, Folder: %s)",
		torrentName, torrentID, downloadFolder)

	// Create Deluge client
	settings := deluge.Settings{
		Hostname: *host,
		Port:     *port,
		Login:    config.Deluge.Login,
		Password: config.Deluge.Password,
	}

	// Enable debug logging if configured
	if strings.ToUpper(config.Logging.Level) == "DEBUG" {
		settings.DebugServerResponses = true
	}

	client := deluge.NewV2(settings)

	// Connect to Deluge daemon
	if err := client.Connect(); err != nil {
		logger.Info("Error: Failed to connect to Deluge daemon: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	// Force reannounce with retries
	logger.Info("Starting reannounce attempts for torrent %s (timeout: %ds, interval: %ds)",
		torrentName, config.Retry.Timeout, config.Retry.Interval)

	timeout := time.After(time.Duration(config.Retry.Timeout) * time.Second)
	ticker := time.NewTicker(time.Duration(config.Retry.Interval) * time.Second)
	defer ticker.Stop()

	var lastErr error
	var attempts int
	for {
		attempts++
		select {
		case <-timeout:
			if lastErr != nil {
				logger.Error("Failed to force reannounce after %ds (%d attempts): %v",
					config.Retry.Timeout, attempts, lastErr)
				os.Exit(1)
			}
			logger.Info("Successfully forced reannounce for torrent %s after %d attempts",
				torrentName, attempts)
			os.Exit(0)
		case <-ticker.C:
			logger.Debug("Attempt %d: Forcing reannounce for torrent %s", attempts, torrentName)
			err = client.ForceReannounce([]string{torrentID})
			if err != nil {
				lastErr = err
				logger.Debug("Attempt %d failed: %v", attempts, err)
				continue
			}
			logger.Info("Successfully forced reannounce for torrent %s after %d attempts",
				torrentName, attempts)
			os.Exit(0)
		}
	}
}
