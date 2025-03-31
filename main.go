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

// Config represents the application configuration
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

// Logger handles application logging
type Logger struct {
	infoLogger  *log.Logger
	debugLogger *log.Logger
	file        *os.File
	level       string
}

// NewLogger creates a new logger instance
func NewLogger(config *Config) (*Logger, error) {
	logDir := filepath.Dir(config.Logging.File)
	if logDir != "." {
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create log directory: %w", err)
		}
	}

	file, err := os.OpenFile(config.Logging.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	infoLogger := log.New(file, "INFO: ", log.Ldate|log.Ltime)
	debugLogger := log.New(file, "DEBUG: ", log.Ldate|log.Ltime)

	return &Logger{
		infoLogger:  infoLogger,
		debugLogger: debugLogger,
		file:        file,
		level:       strings.ToUpper(config.Logging.Level),
	}, nil
}

// Info logs an info message
func (l *Logger) Info(format string, v ...interface{}) {
	l.infoLogger.Printf(format, v...)
}

// Debug logs a debug message
func (l *Logger) Debug(format string, v ...interface{}) {
	if l.level == "DEBUG" {
		l.debugLogger.Printf(format, v...)
	}
}

// Error logs an error message
func (l *Logger) Error(format string, v ...interface{}) {
	if l.level == "DEBUG" {
		l.debugLogger.Printf("ERROR: "+format, v...)
	}
}

// Close closes the log file
func (l *Logger) Close() error {
	return l.file.Close()
}

// loadConfig loads and validates the configuration file
func loadConfig(configFile string) (*Config, error) {
	data, err := os.ReadFile(configFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configFile, err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
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

// DelugeClient wraps the Deluge client with additional functionality
type DelugeClient struct {
	client *deluge.ClientV2
	logger *Logger
}

// NewDelugeClient creates a new Deluge client
func NewDelugeClient(settings deluge.Settings, logger *Logger) *DelugeClient {
	return &DelugeClient{
		client: deluge.NewV2(settings),
		logger: logger,
	}
}

// Connect connects to the Deluge daemon
func (d *DelugeClient) Connect() error {
	if err := d.client.Connect(); err != nil {
		return fmt.Errorf("failed to connect to Deluge daemon: %w", err)
	}
	return nil
}

// Close closes the connection to the Deluge daemon
func (d *DelugeClient) Close() error {
	return d.client.Close()
}

// ForceReannounce attempts to force reannounce a torrent with retries
func (d *DelugeClient) ForceReannounce(torrentID string, timeout, interval time.Duration) bool {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	timeoutChan := time.After(timeout)
	attempts := 0

	for {
		select {
		case <-timeoutChan:
			log.Printf("Timeout reached after %v attempts", attempts)
			return false
		case <-ticker.C:
			attempts++
			log.Printf("Attempt %d: Force reannouncing torrent %s", attempts, torrentID)

			err := d.client.ForceReannounce([]string{torrentID})
			if err != nil {
				log.Printf("Error force reannouncing torrent: %v", err)
				continue
			}

			// Wait a bit for the torrent to start updating its status
			time.Sleep(2 * time.Second)

			// Check torrent status
			status, err := d.GetTorrentStatus(torrentID)
			if err != nil {
				log.Printf("Error getting torrent status: %v", err)
				continue
			}

			// Log torrent status
			log.Printf("Torrent status: State=%s, Progress=%.2f%%, Download Rate=%.2f KB/s, Upload Rate=%.2f KB/s, Peers=%d/%d, Seeds=%d",
				status.State,
				status.Progress*100,
				float64(status.DownloadPayloadRate)/1024,
				float64(status.UploadPayloadRate)/1024,
				status.NumPeers,
				status.TotalPeers,
				status.TotalSeeds)

			// Check both torrent state and tracker status
			if (status.State == "Downloading" || status.State == "Seeding") && status.TrackerStatus == "Announce OK" {
				log.Printf("Successfully reannounced torrent %s (State: %s, Tracker: %s)", torrentID, status.State, status.TrackerStatus)
				return true
			}

			log.Printf("Torrent not in desired state yet (current: %s), retrying...", status.State)
		}
	}
}

// GetTorrentStatus gets the status of a torrent
func (d *DelugeClient) GetTorrentStatus(torrentID string) (*deluge.TorrentStatus, error) {
	status, err := d.client.TorrentStatus(torrentID)
	if err != nil {
		return nil, fmt.Errorf("failed to get torrent status: %w", err)
	}

	// Log detailed torrent information if debug logging is enabled
	if d.logger.level == "DEBUG" {
		d.logger.Debug("Torrent Status Details for %s:", torrentID)
		d.logger.Debug("  Name: %s", status.Name)
		d.logger.Debug("  State: %s", status.State)
		d.logger.Debug("  Progress: %.2f%%", status.Progress*100)
		d.logger.Debug("  Download Rate: %.2f KB/s", float64(status.DownloadPayloadRate)/1024)
		d.logger.Debug("  Upload Rate: %.2f KB/s", float64(status.UploadPayloadRate)/1024)
		d.logger.Debug("  Peers: %d/%d", status.NumPeers, status.TotalPeers)
		d.logger.Debug("  Seeds: %d/%d", status.NumSeeds, status.TotalSeeds)
		d.logger.Debug("  Total Size: %.2f GB", float64(status.TotalSize)/1024/1024/1024)
		d.logger.Debug("  Total Done: %.2f GB", float64(status.TotalDone)/1024/1024/1024)
		d.logger.Debug("  ETA: %.0f seconds", status.ETA)
		d.logger.Debug("  Ratio: %.2f", status.Ratio)
		d.logger.Debug("  Is Finished: %v", status.IsFinished)
		d.logger.Debug("  Is Seed: %v", status.IsSeed)
		d.logger.Debug("  Private: %v", status.Private)
		d.logger.Debug("  Save Path: %s", status.SavePath)
		d.logger.Debug("  Download Location: %s", status.DownloadLocation)
		d.logger.Debug("  Next Announce: %d seconds", status.NextAnnounce)
		d.logger.Debug("  Num Pieces: %d", status.NumPieces)
		d.logger.Debug("  Piece Length: %d bytes", status.PieceLength)
		d.logger.Debug("  Seeding Time: %d seconds", status.SeedingTime)
		d.logger.Debug("  Active Time: %d seconds", status.ActiveTime)
		d.logger.Debug("  Completed Time: %d seconds", status.CompletedTime)
		d.logger.Debug("  Time Added: %.0f seconds", status.TimeAdded)
		d.logger.Debug("  Last Seen Complete: %.0f seconds", status.LastSeenComplete)
		d.logger.Debug("  Distributed Copies: %.2f", status.DistributedCopies)
		d.logger.Debug("  Tracker Host: %s", status.TrackerHost)
		d.logger.Debug("  Tracker Status: %s", status.TrackerStatus)
	}

	return status, nil
}

func main() {
	// Get the directory where the executable is located
	execPath, err := os.Executable()
	if err != nil {
		log.Fatalf("Failed to get executable path: %v", err)
	}
	execDir := filepath.Dir(execPath)

	// Parse command line flags
	configFile := flag.String("config", filepath.Join(execDir, "config.yml"), "Path to config file")
	host := flag.String("host", "", "Deluge daemon host")
	port := flag.Uint("port", 0, "Deluge daemon port")
	username := flag.String("username", "", "Deluge daemon username")
	password := flag.String("password", "", "Deluge daemon password")
	flag.Parse()

	// Load configuration
	config, err := loadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Override config with command line flags if provided
	if *host != "" {
		config.Deluge.Hostname = *host
	}
	if *port != 0 {
		config.Deluge.Port = *port
	}
	if *username != "" {
		config.Deluge.Login = *username
	}
	if *password != "" {
		config.Deluge.Password = *password
	}

	// Initialize logger
	logger, err := NewLogger(config)
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Close()

	// Check required arguments
	if len(flag.Args()) != 3 {
		logger.Info("Usage: %s [flags] <torrent_id> <torrent_name> <download_folder>\n\nFlags:\n  -config string\n        Path to config file (default \"config.yml\")\n  -host string\n        Deluge daemon host\n  -password string\n        Deluge daemon password\n  -port uint\n        Deluge daemon port\n  -username string\n        Deluge daemon username", os.Args[0])
		os.Exit(1)
	}

	torrentID := flag.Arg(0)
	torrentName := flag.Arg(1)
	downloadFolder := flag.Arg(2)

	// Log the incoming parameters
	logger.Info("Received reannounce request for torrent: %s (ID: %s, Folder: %s)",
		torrentName, torrentID, downloadFolder)

	// Create Deluge client settings
	settings := deluge.Settings{
		Hostname: config.Deluge.Hostname,
		Port:     config.Deluge.Port,
		Login:    config.Deluge.Login,
		Password: config.Deluge.Password,
	}

	// Enable debug logging if configured
	if strings.ToUpper(config.Logging.Level) == "DEBUG" {
		settings.DebugServerResponses = true
	}

	// Create and connect to Deluge client
	client := NewDelugeClient(settings, logger)
	if err := client.Connect(); err != nil {
		logger.Info("Error: %v", err)
		os.Exit(1)
	}
	defer client.Close()

	// Force reannounce with retries
	logger.Info("Starting reannounce attempts for torrent %s (timeout: %ds, interval: %ds)",
		torrentName, config.Retry.Timeout, config.Retry.Interval)

	timeout := time.Duration(config.Retry.Timeout) * time.Second
	interval := time.Duration(config.Retry.Interval) * time.Second

	if client.ForceReannounce(torrentID, timeout, interval) {
		os.Exit(0)
	}

	logger.Error("Failed to force reannounce")
	os.Exit(1)
}
