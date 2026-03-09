// Package say provides a structured logging package based on the standard slog logger.
// It supports both structured (JSON) and console (text) logging modes, with the ability
// to write logs to stdout and files with rotation using lumberjack.
package say

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"

	"gopkg.in/natefinch/lumberjack.v2"
)

// Format represents the logging format
type Format string

const (
	// FormatJSON uses JSON format for structured logging
	FormatJSON Format = "json"
	// FormatText uses human-readable text format for console logging
	FormatText Format = "text"
)

// Logger wraps slog.Logger and provides dynamic level control and hot-reload support.
type Logger struct {
	*slog.Logger
	levelVar *slog.LevelVar

	// handlerContainer holds the reloadable handler for hot-reload support.
	// Shared across all child loggers created via With().
	handlerContainer *HandlerContainer

	// initialReplaceAttr preserves the ReplaceAttr function set at logger creation.
	// This is necessary because ReplaceAttr is code (not data) and cannot be
	// deserialized from YAML/ENV during hot-reload.
	initialReplaceAttr func(groups []string, a slog.Attr) slog.Attr

	// unsubscribe is called by Close() to remove this logger from config updates.
	// Only set for loggers created with NewLoggerWithHotReload.
	unsubscribe func()
}

// SetLevel dynamically changes the log level for this logger instance.
func (l *Logger) SetLevel(levelStr string) error {
	var level slog.Level
	if err := level.UnmarshalText([]byte(levelStr)); err != nil {
		return &ConfigError{Message: "invalid log level: " + levelStr}
	}
	if l.levelVar != nil {
		l.levelVar.Set(level)
	}
	return nil
}

// GetLevel returns the current log level as a string.
func (l *Logger) GetLevel() string {
	if l.levelVar != nil {
		return l.levelVar.Level().String()
	}
	return ""
}

// Reconfigure updates the logger with a new configuration.
// This method can be called manually or automatically via hot-reload subscription.
// It preserves the initialReplaceAttr if the new config doesn't provide one.
func (l *Logger) Reconfigure(cfg *Config) error {
	if cfg == nil {
		return &ConfigError{Message: "config cannot be nil"}
	}

	// Validate the new configuration
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Update log level (cheap operation)
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LevelStr)); err != nil {
		return &ConfigError{Message: "invalid log level: " + cfg.LevelStr}
	}
	if l.levelVar != nil {
		l.levelVar.Set(level)
	}

	// Preserve ReplaceAttr function if not set in new config.
	// ReplaceAttr is code (not data) and cannot be deserialized from YAML/ENV
	// files during hot-reload.
	if cfg.ReplaceAttr == nil && l.initialReplaceAttr != nil {
		cfg.ReplaceAttr = l.initialReplaceAttr
	}

	// Create new handler (expensive operation)
	newHandler, newCloser, err := createSlogHandler(cfg, l.levelVar)
	if err != nil {
		return err
	}

	// Update the handler container atomically.
	// All loggers using this container (including children via With()) will
	// immediately start using the new format/output.
	if l.handlerContainer != nil {
		l.handlerContainer.Update(newHandler, newCloser)
	}

	return nil
}

// Close releases resources associated with this logger.
// For loggers created with NewLoggerWithHotReload, this unsubscribes from config updates.
// IMPORTANT: Must be called to prevent memory leaks when the logger is no longer needed.
func (l *Logger) Close() error {
	if l.unsubscribe != nil {
		l.unsubscribe()
		l.unsubscribe = nil
	}
	return nil
}

// With returns a new Logger with the given attributes.
// Overrides slog.Logger.With to return *say.Logger wrapper.
func (l *Logger) With(args ...any) *Logger {
	newSlog := l.Logger.With(args...)
	return &Logger{
		Logger:             newSlog,
		levelVar:           l.levelVar,
		handlerContainer:   l.handlerContainer,
		initialReplaceAttr: l.initialReplaceAttr,
		unsubscribe:        nil,
	}
}

// WithGroup returns a new Logger that starts a group.
// Overrides slog.Logger.WithGroup to return *say.Logger wrapper.
func (l *Logger) WithGroup(name string) *Logger {
	newSlog := l.Logger.WithGroup(name)
	return &Logger{
		Logger:             newSlog,
		levelVar:           l.levelVar,
		handlerContainer:   l.handlerContainer,
		initialReplaceAttr: l.initialReplaceAttr,
		unsubscribe:        nil,
	}
}

var (
	// defaultLogger is the global logger instance
	defaultLogger *Logger
	mu            sync.RWMutex
)

// Init initializes the default logger with the provided configuration.
// The global logger is automatically subscribed to config updates (hot-reload).
// This function is idempotent and safe to call multiple times.
// Sets the logger as the global default for both this package and slog.
func Init(cfg *Config) error {
	if cfg == nil {
		cfg = &Config{}
	}
	// Apply defaults and validate
	cfg.SetDefaults()
	if err := cfg.Validate(); err != nil {
		return err
	}

	// Create logger with hot-reload support.
	// Global logger is a singleton, so memory leak from subscription is not a concern.
	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		return err
	}

	// Set as default
	mu.Lock()
	defaultLogger = logger
	slog.SetDefault(logger.Logger)
	mu.Unlock()

	return nil
}

// NewLogger creates a new logger instance with the provided configuration.
// Each logger is independent with its own level settings.
// This logger will NOT automatically update when the config changes.
// For hot-reload support, use NewLoggerWithHotReload instead.
func NewLogger(cfg *Config) (*Logger, error) {
	if cfg == nil {
		return nil, &ConfigError{Message: "config cannot be nil"}
	}

	// Parse and set log level
	var level slog.Level
	if err := level.UnmarshalText([]byte(cfg.LevelStr)); err != nil {
		return nil, &ConfigError{Message: "invalid log level: " + cfg.LevelStr}
	}
	lv := &slog.LevelVar{}
	lv.Set(level)

	// Create the base handler
	baseHandler, closer, err := createSlogHandler(cfg, lv)
	if err != nil {
		return nil, err
	}

	// Wrap in ReloadableHandler (even without hot-reload, for consistency)
	container := &HandlerContainer{
		handler: baseHandler,
		closer:  closer,
	}
	proxyHandler := &ReloadableHandler{
		core: container,
	}

	return &Logger{
		Logger:             slog.New(proxyHandler),
		levelVar:           lv,
		handlerContainer:   container,
		initialReplaceAttr: cfg.ReplaceAttr,
		unsubscribe:        nil, // No subscription
	}, nil
}

// NewLoggerWithHotReload creates a new logger instance that automatically updates
// when the configuration changes (via config.Manager.Watch).
// IMPORTANT: The caller MUST call logger.Close() when the logger is no longer needed
// to prevent memory leaks from the subscription.
func NewLoggerWithHotReload(cfg *Config) (*Logger, error) {
	logger, err := NewLogger(cfg)
	if err != nil {
		return nil, err
	}

	// Subscribe to config updates
	unsubscribe := cfg.Subscribe(func(newCfg *Config) error {
		return logger.Reconfigure(newCfg)
	})

	logger.unsubscribe = unsubscribe
	return logger, nil
}

// createSlogHandler creates a new slog.Handler based on the configuration.
// Returns the handler, an optional closer (for file writers), and an error.
// The closer should be called when the handler is no longer needed to release file resources.
func createSlogHandler(cfg *Config, lv *slog.LevelVar) (slog.Handler, io.Closer, error) {
	if cfg == nil {
		return nil, nil, &ConfigError{Message: "config cannot be nil"}
	}

	var writers []io.Writer
	var closer io.Closer // Will be set if we use lumberjack

	// Add stdout writer if enabled
	if cfg.OutputStdout {
		writers = append(writers, os.Stdout)
	}

	// Add file writer if enabled
	if cfg.OutputFile {
		if cfg.FilePath == "" {
			return nil, nil, &ConfigError{Message: "FilePath is required when OutputFile is true"}
		}

		// FileCompress defaults to true when nil
		fileCompress := true
		if cfg.FileCompress != nil {
			fileCompress = *cfg.FileCompress
		}

		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FilePath,
			MaxSize:    cfg.FileMaxSize,
			MaxBackups: cfg.FileMaxBackups,
			MaxAge:     cfg.FileMaxAge,
			Compress:   fileCompress,
		}
		writers = append(writers, fileWriter)
		closer = fileWriter // lumberjack.Logger implements io.Closer
	}

	if len(writers) == 0 {
		return nil, nil, &ConfigError{Message: "at least one output (stdout or file) must be enabled"}
	}

	// Create multi-writer
	var writer io.Writer
	if len(writers) == 1 {
		writer = writers[0]
	} else {
		writer = io.MultiWriter(writers...)
	}

	// Create handler options with LevelVar for dynamic level changes
	opts := &slog.HandlerOptions{
		Level:     lv,
		AddSource: cfg.AddSource,
	}

	if cfg.ReplaceAttr != nil {
		opts.ReplaceAttr = cfg.ReplaceAttr
	}

	// Create handler based on format
	var handler slog.Handler
	switch cfg.Format {
	case FormatJSON:
		handler = slog.NewJSONHandler(writer, opts)
	case FormatText:
		handler = slog.NewTextHandler(writer, opts)
	default:
		return nil, nil, &ConfigError{Message: "invalid format: " + string(cfg.Format)}
	}

	return handler, closer, nil
}

// GetLogger returns the default logger instance.
// Returns slog.Default() if not initialized via Init.
func GetLogger() *slog.Logger {
	mu.RLock()
	defer mu.RUnlock()
	if defaultLogger == nil {
		return slog.Default()
	}
	return defaultLogger.Logger
}

// SetLogger sets the default logger instance from a Logger wrapper.
// Note: This sets the logger globally for both this package and slog.Default().
func SetLogger(logger *Logger) {
	if logger == nil {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	defaultLogger = logger
	slog.SetDefault(logger.Logger)
}

// SetLevel dynamically changes the log level of the global logger.
// This is useful for runtime level adjustments (e.g., via HTTP endpoint or signal).
// Returns error if the level string is invalid or logger is not initialized.
func SetLevel(levelStr string) error {
	mu.RLock()
	logger := defaultLogger
	mu.RUnlock()

	if logger == nil {
		return &ConfigError{Message: "logger not initialized, call Init first"}
	}

	return logger.SetLevel(levelStr)
}

// GetLevel returns the current log level of the global logger as a string.
// Returns empty string if logger is not initialized.
func GetLevel() string {
	mu.RLock()
	logger := defaultLogger
	mu.RUnlock()

	if logger == nil {
		return ""
	}

	return logger.GetLevel()
}

// ConfigError represents a configuration error.
type ConfigError struct {
	Message string
}

func (e *ConfigError) Error() string {
	return "say config error: " + e.Message
}

// Convenience functions that use the default logger.
// These functions delegate to slog package which is thread-safe and uses
// the logger we set via slog.SetDefault() in Init and SetLogger.

// Debug logs a message at Debug level.
func Debug(msg string, args ...any) {
	slog.Debug(msg, args...)
}

// Info logs a message at Info level.
func Info(msg string, args ...any) {
	slog.Info(msg, args...)
}

// Warn logs a message at Warn level.
func Warn(msg string, args ...any) {
	slog.Warn(msg, args...)
}

// Error logs a message at Error level.
func Error(msg string, args ...any) {
	slog.Error(msg, args...)
}

// DebugContext logs a message at Debug level with context.
func DebugContext(ctx context.Context, msg string, args ...any) {
	slog.DebugContext(ctx, msg, args...)
}

// InfoContext logs a message at Info level with context.
func InfoContext(ctx context.Context, msg string, args ...any) {
	slog.InfoContext(ctx, msg, args...)
}

// WarnContext logs a message at Warn level with context.
func WarnContext(ctx context.Context, msg string, args ...any) {
	slog.WarnContext(ctx, msg, args...)
}

// ErrorContext logs a message at Error level with context.
func ErrorContext(ctx context.Context, msg string, args ...any) {
	slog.ErrorContext(ctx, msg, args...)
}

// Log logs a message at the specified level.
func Log(level slog.Level, msg string, args ...any) {
	slog.Log(context.Background(), level, msg, args...)
}

// LogContext logs a message at the specified level with context.
func LogContext(ctx context.Context, level slog.Level, msg string, args ...any) {
	slog.Log(ctx, level, msg, args...)
}
