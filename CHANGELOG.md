# Changelog

All notable changes to this project will be documented in this file.

## [1.0.14] - 2024-03-21

### Changed
- Refactored codebase to follow Go best practices and standards
- Improved error handling with proper error wrapping using `fmt.Errorf` and `%w` verb
- Enhanced logging system with better level control and message formatting
- Created `DelugeClient` struct to encapsulate Deluge-related functionality
- Improved command-line flag handling and configuration management
- Added comprehensive documentation for all types and functions
- Optimized resource management with proper use of `defer`
- Improved code organization and separation of concerns

### Added
- Command-line flags for Deluge daemon credentials
  - `-username` flag to override config file username
  - `-password` flag to override config file password
- Improved usage message with all available flags

## [1.0.12] - 2024-03-21

### Added
- Automatic retry mechanism for reannounce operations
- 7-second interval between retry attempts
- 120-second maximum timeout for reannounce operations
- Detailed logging of retry attempts in DEBUG mode

## [1.0.11] - 2024-03-21

### Added
- Command-line flag (-c) for specifying custom config file location
- Improved error messages for config file loading failures

## [1.0.10] - 2024-03-21

### Added
- File-based logging system
- Two logging levels: INFO and DEBUG
- Configurable log file path
- Detailed logging of Deluge daemon communication in DEBUG mode
- Automatic log directory creation

### Changed
- Switched from standard output to file logging
- Improved error messages and logging format
- Added timestamps to log entries

## [1.0.0] - 2024-03-21

### Added
- Initial release of the Deluge Reannounce Go program
- Pure Go implementation with no external dependencies
- Direct JSON-RPC communication with Deluge daemon
- Command-line interface matching Deluge Execute plugin requirements
- Configurable host and port via command-line flags
- Detailed logging functionality
- Connection checking before reannounce
- Error handling and proper exit codes 