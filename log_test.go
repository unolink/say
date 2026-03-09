package say

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	t.Parallel()

	cfg := &Config{}
	cfg.SetDefaults()
	if cfg.LevelStr != "info" {
		t.Errorf("Expected level %v, got %v", "info", cfg.LevelStr)
	}
	if cfg.Format != FormatText {
		t.Errorf("Expected format %v, got %v", FormatText, cfg.Format)
	}
	if !cfg.OutputStdout {
		t.Error("Expected OutputStdout to be true")
	}
	if cfg.OutputFile {
		t.Error("Expected OutputFile to be false")
	}
}

func TestInit(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger() returned nil after Init()")
	}
}

func TestInitWithNilConfig(t *testing.T) {
	err := Init(nil)
	if err != nil {
		t.Fatalf("Init(nil) failed: %v", err)
	}

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger() returned nil after Init(nil)")
	}
}

func TestInitWithJSONFormat(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.Format = FormatJSON
	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() with JSON format failed: %v", err)
	}

	logger := GetLogger()
	if logger == nil {
		t.Fatal("GetLogger() returned nil")
	}
}

func TestInitWithFileOutput(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{}
	cfg.SetDefaults()
	cfg.OutputFile = true
	cfg.FilePath = tmpFile
	cfg.OutputStdout = false

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger() with file output failed: %v", err)
	}

	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}

	// Test logging
	logger.Info("Test message")

	// Verify file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("Log file was not created")
	}
}

func TestInitWithBothOutputs(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.log")

	cfg := &Config{}
	cfg.SetDefaults()
	cfg.OutputFile = true
	cfg.FilePath = tmpFile
	cfg.OutputStdout = true

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger() with both outputs failed: %v", err)
	}

	if logger == nil {
		t.Fatal("NewLogger() returned nil")
	}
}

func TestInitWithInvalidConfig(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.OutputFile = true
	cfg.FilePath = "" // Invalid: file path required

	_, err := NewLogger(cfg)
	if err == nil {
		t.Error("Expected error for missing file path")
	}

	var configErr *ConfigError
	if !isConfigError(err, &configErr) {
		t.Error("Expected ConfigError")
	}
}

func TestInitWithNoOutputs(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.OutputStdout = false
	cfg.OutputFile = false

	_, err := NewLogger(cfg)
	if err == nil {
		t.Error("Expected error for no outputs")
	}

	var configErr *ConfigError
	if !isConfigError(err, &configErr) {
		t.Error("Expected ConfigError")
	}
}

func TestInitWithInvalidFormat(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.Format = Format("invalid")

	_, err := NewLogger(cfg)
	if err == nil {
		t.Error("Expected error for invalid format")
	}

	var configErr *ConfigError
	if !isConfigError(err, &configErr) {
		t.Error("Expected ConfigError")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.OutputStdout = true
	cfg.OutputFile = false
	err := Init(cfg)
	if err != nil {
		t.Fatalf("Init() failed: %v", err)
	}

	// Test convenience functions
	Debug("Debug message", "key", "value")
	Info("Info message", "key", "value")
	Warn("Warn message", "key", "value")
	Error("Error message", "key", "value")

	ctx := context.Background()
	DebugContext(ctx, "Debug context message", "key", "value")
	InfoContext(ctx, "Info context message", "key", "value")
	WarnContext(ctx, "Warn context message", "key", "value")
	ErrorContext(ctx, "Error context message", "key", "value")

	Log(slog.LevelInfo, "Log message", "key", "value")
	LogContext(ctx, slog.LevelInfo, "Log context message", "key", "value")
}

func TestSetLogger(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger() failed: %v", err)
	}

	SetLogger(logger)
	if GetLogger() != logger.Logger {
		t.Error("SetLogger() did not set the logger correctly")
	}
}

func TestGetLoggerBeforeInit(t *testing.T) {
	// GetLogger should never return nil, it returns slog.Default() instead
	logger := GetLogger()
	if logger == nil {
		t.Error("GetLogger() returned nil, expected slog.Default()")
	}
}

func TestLoggerSetLevel(t *testing.T) {
	cfg := &Config{}
	cfg.SetDefaults()
	cfg.LevelStr = "info"

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger() failed: %v", err)
	}

	// Check initial level
	if logger.GetLevel() != "INFO" {
		t.Errorf("Expected level INFO, got %s", logger.GetLevel())
	}

	// Change level dynamically
	err = logger.SetLevel("debug")
	if err != nil {
		t.Fatalf("SetLevel() failed: %v", err)
	}

	if logger.GetLevel() != "DEBUG" {
		t.Errorf("Expected level DEBUG after SetLevel, got %s", logger.GetLevel())
	}

	// Test invalid level
	err = logger.SetLevel("invalid")
	if err == nil {
		t.Error("Expected error for invalid level")
	}
}

func TestErrorAttr(t *testing.T) {
	t.Parallel()

	// Test with valid error
	err := fmt.Errorf("test error")
	attr := ErrorAttr(err)
	if attr.Key != "error" {
		t.Errorf("Expected key 'error', got '%s'", attr.Key)
	}
	if attr.Value.String() != "test error" {
		t.Errorf("Expected value 'test error', got '%s'", attr.Value.String())
	}

	// Test with nil error - should not panic
	attr = ErrorAttr(nil)
	if attr.Key != "error" {
		t.Errorf("Expected key 'error', got '%s'", attr.Key)
	}
	if attr.Value.String() != "<nil>" {
		t.Errorf("Expected value '<nil>', got '%s'", attr.Value.String())
	}
}

func isConfigError(err error, target **ConfigError) bool {
	if err == nil {
		return false
	}
	return errors.As(err, target)
}
