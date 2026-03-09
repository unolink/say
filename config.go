package say

import (
	"fmt"
	"log/slog"
	"sync"
)

// ConfigKey is the default section key used in configuration files.
const ConfigKey = "logger"

// Subscriber defines a callback function that is invoked when the configuration is updated.
// It receives the updated Config and should return an error if reconfiguration fails.
type Subscriber func(c *Config) error

// Config represents logger configuration.
// Implements the observer pattern for hot-reload support: subscribers are notified
// when OnUpdate is called (typically by a configuration manager watching file changes).
//
// Hot-reload: Supports automatic updates for format (JSON/Text), output (File/Stdout),
// and log level without restart.
type Config struct {
	FileCompress   *bool                                        `yaml:"file_compress" json:"file_compress" usage:"Compress rotated log files" hotreload:"yes"`
	subscribers    map[int]Subscriber                           `yaml:"-" json:"-"`
	ReplaceAttr    func(groups []string, a slog.Attr) slog.Attr `yaml:"-" json:"-"`
	Format         Format                                       `yaml:"format" json:"format" usage:"Output format: json or text" hotreload:"yes"`
	LevelStr       string                                       `yaml:"level" json:"level" usage:"Log level: debug, info, warn, error" hotreload:"yes"`
	FilePath       string                                       `yaml:"file_path" json:"file_path" usage:"Path to log file (required if output_file is true)" hotreload:"yes"`
	nextID         int                                          `yaml:"-" json:"-"`
	FileMaxSize    int                                          `yaml:"file_max_size" json:"file_max_size" usage:"Maximum log file size in MB before rotation" hotreload:"yes"`
	FileMaxBackups int                                          `yaml:"file_max_backups" json:"file_max_backups" usage:"Maximum number of old log files to retain" hotreload:"yes"`
	FileMaxAge     int                                          `yaml:"file_max_age" json:"file_max_age" usage:"Maximum age of log files in days" hotreload:"yes"`
	mu             sync.RWMutex                                 `yaml:"-" json:"-"`
	OutputFile     bool                                         `yaml:"output_file" json:"output_file" usage:"Enable file output" hotreload:"yes"`
	AddSource      bool                                         `yaml:"add_source" json:"add_source" usage:"Add source file location to log entries" hotreload:"yes"`
	OutputStdout   bool                                         `yaml:"output_stdout" json:"output_stdout" usage:"Enable stdout output" hotreload:"yes"`
}

// ConfigKey returns the section key for use in configuration files (e.g., YAML).
func (c *Config) ConfigKey() string {
	return ConfigKey
}

// SetDefaults sets default values.
func (c *Config) SetDefaults() {
	if c.LevelStr == "" {
		c.LevelStr = "info"
	}
	if c.Format == "" {
		c.Format = FormatText
	}
	if !c.OutputStdout && !c.OutputFile {
		// Default to stdout if nothing is specified
		c.OutputStdout = true
	}
	if c.FileMaxSize == 0 {
		c.FileMaxSize = 100
	}
	if c.FileMaxBackups == 0 {
		c.FileMaxBackups = 3
	}
	if c.FileMaxAge == 0 {
		c.FileMaxAge = 28
	}
	// FileCompress defaults to true if OutputFile is enabled
	if c.OutputFile && c.FileCompress == nil {
		compress := true
		c.FileCompress = &compress
	}
}

// Validate checks the configuration for correctness.
func (c *Config) Validate() error {
	// Validate log level using UnmarshalText
	var level slog.Level
	if err := level.UnmarshalText([]byte(c.LevelStr)); err != nil {
		return fmt.Errorf("invalid log level %q: %w", c.LevelStr, err)
	}

	// Validate format
	if c.Format != FormatJSON && c.Format != FormatText {
		return fmt.Errorf("invalid format %q: must be one of json, text", c.Format)
	}

	// Check that at least one output is enabled
	if !c.OutputStdout && !c.OutputFile {
		return fmt.Errorf("at least one output (stdout or file) must be enabled")
	}

	// If file output is enabled, path is required
	if c.OutputFile && c.FilePath == "" {
		return fmt.Errorf("file_path is required when output_file is true")
	}

	// Validate positive values for file parameters
	if c.OutputFile {
		if c.FileMaxSize <= 0 {
			return fmt.Errorf("file_max_size must be positive")
		}
		if c.FileMaxBackups < 0 {
			return fmt.Errorf("file_max_backups must be non-negative")
		}
		if c.FileMaxAge < 0 {
			return fmt.Errorf("file_max_age must be non-negative")
		}
	}

	return nil
}

// Subscribe registers a callback to be invoked when the configuration is updated.
// Returns an unsubscribe function that removes the callback from the registry.
// The caller MUST call the returned function when the logger is no longer needed
// to prevent memory leaks.
func (c *Config) Subscribe(fn Subscriber) func() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Initialize map on first subscription
	if c.subscribers == nil {
		c.subscribers = make(map[int]Subscriber)
	}

	// Assign unique ID and register callback
	id := c.nextID
	c.nextID++
	c.subscribers[id] = fn

	// Return unsubscribe function
	return func() {
		c.mu.Lock()
		defer c.mu.Unlock()
		delete(c.subscribers, id)
	}
}

// OnUpdate notifies all registered subscribers about a configuration change.
// Invokes all registered subscribers synchronously.
// Typically called by a configuration manager when config file changes are detected.
func (c *Config) OnUpdate() {
	// Collect subscribers under read lock
	c.mu.RLock()
	subs := make([]Subscriber, 0, len(c.subscribers))
	for _, fn := range c.subscribers {
		subs = append(subs, fn)
	}
	c.mu.RUnlock()

	// Notify all subscribers synchronously.
	// Synchronous execution ensures config is applied immediately and in order.
	for _, fn := range subs {
		if err := fn(c); err != nil {
			// Use fmt to avoid circular dependency on logger
			_, _ = fmt.Printf("say: failed to reload logger config: %v\n", err)
		}
	}
}
