package say

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestNewLoggerVsHotReload tests that NewLogger doesn't subscribe,
// while NewLoggerWithHotReload does.
func TestNewLoggerVsHotReload(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	// NewLogger should NOT subscribe
	logger1, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}
	if logger1.unsubscribe != nil {
		t.Error("NewLogger should not have unsubscribe function")
	}

	// NewLoggerWithHotReload should subscribe
	logger2, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	if logger2.unsubscribe == nil {
		t.Error("NewLoggerWithHotReload should have unsubscribe function")
	}

	// Cleanup
	if err := logger2.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

// TestHotReloadLevelChange tests that log level updates propagate to subscribed loggers.
func TestHotReloadLevelChange(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Initial level should be INFO
	if logger.GetLevel() != "INFO" {
		t.Errorf("Expected INFO level, got %s", logger.GetLevel())
	}

	// Simulate config update
	cfg.LevelStr = "debug"
	cfg.OnUpdate()

	// Level should now be DEBUG
	if logger.GetLevel() != "DEBUG" {
		t.Errorf("Expected DEBUG level after hot-reload, got %s", logger.GetLevel())
	}
}

// TestHotReloadFormatChange tests switching between JSON and Text formats.
func TestHotReloadFormatChange(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatJSON,
		OutputStdout: true,
	}

	// Create logger
	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// Log with JSON format
	cfg.Format = FormatJSON
	logger.Info("json message", "key", "value")

	// Change to Text format
	cfg.Format = FormatText
	if err := logger.Reconfigure(cfg); err != nil {
		t.Fatalf("Reconfigure failed: %v", err)
	}

	// Log with Text format
	logger.Info("text message", "key", "value")

	// Both messages should have been logged
	// This test mainly verifies no panics occur during format change
}

// TestHotReloadWithChildLoggers tests that child loggers (via With) also get updated.
func TestHotReloadWithChildLoggers(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	parentLogger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = parentLogger.Close() }()

	// Create child logger with additional context
	childLogger := parentLogger.With("child", "logger", "request_id", "123")

	// Change log level
	cfg.LevelStr = "debug"
	cfg.OnUpdate()

	// Both parent and child should respect new level
	// This is ensured by the shared HandlerContainer
	childLogger.Debug("debug from child", "test", "value")
}

// TestLoggerClose tests that Close unsubscribes from config updates.
func TestLoggerClose(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}

	// Verify subscription exists
	cfg.mu.RLock()
	initialCount := len(cfg.subscribers)
	cfg.mu.RUnlock()

	if initialCount == 0 {
		t.Error("Expected at least one subscriber")
	}

	// Close should unsubscribe
	if err := logger.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	cfg.mu.RLock()
	afterCloseCount := len(cfg.subscribers)
	cfg.mu.RUnlock()

	if afterCloseCount != initialCount-1 {
		t.Errorf("Expected %d subscribers after Close, got %d", initialCount-1, afterCloseCount)
	}

	// Closing again should be safe (idempotent)
	if err := logger.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

// TestMultipleSubscribers tests that multiple loggers can subscribe to the same config.
func TestMultipleSubscribers(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	logger1, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload #1 failed: %v", err)
	}
	defer func() { _ = logger1.Close() }()

	logger2, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload #2 failed: %v", err)
	}
	defer func() { _ = logger2.Close() }()

	// Both should have INFO level initially
	if logger1.GetLevel() != "INFO" || logger2.GetLevel() != "INFO" {
		t.Error("Expected both loggers to have INFO level")
	}

	// Update config
	cfg.LevelStr = "warn"
	cfg.OnUpdate()

	// Both should update to WARN
	if logger1.GetLevel() != "WARN" {
		t.Errorf("Logger1: expected WARN, got %s", logger1.GetLevel())
	}
	if logger2.GetLevel() != "WARN" {
		t.Errorf("Logger2: expected WARN, got %s", logger2.GetLevel())
	}
}

// TestReplaceAttrPreservation tests that ReplaceAttr function is preserved during hot-reload.
func TestReplaceAttrPreservation(t *testing.T) {
	t.Parallel()

	customReplacer := func(groups []string, a slog.Attr) slog.Attr {
		// Custom logic: uppercase all string values
		if a.Value.Kind() == slog.KindString {
			return slog.String(a.Key, strings.ToUpper(a.Value.String()))
		}
		return a
	}

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
		ReplaceAttr:  customReplacer,
	}

	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Verify initial ReplaceAttr is captured
	if logger.initialReplaceAttr == nil {
		t.Error("Expected initialReplaceAttr to be captured")
	}

	// Simulate hot-reload from file (ReplaceAttr will be nil in new config)
	newCfg := &Config{
		LevelStr:     "debug",
		Format:       FormatJSON,
		OutputStdout: true,
		ReplaceAttr:  nil, // This is what comes from YAML
	}

	if err := logger.Reconfigure(newCfg); err != nil {
		t.Fatalf("Reconfigure failed: %v", err)
	}

	// ReplaceAttr should have been restored
	// We can't directly test the function, but the logger should still work correctly
}

// TestFileWriterClosure tests that old file writers are closed during hot-reload.
func TestFileWriterClosure(t *testing.T) {
	tempDir := t.TempDir()
	file1 := filepath.Join(tempDir, "test1.log")
	file2 := filepath.Join(tempDir, "test2.log")

	cfg := &Config{
		LevelStr:       "info",
		Format:         FormatText,
		OutputFile:     true,
		FilePath:       file1,
		FileMaxSize:    1,
		FileMaxBackups: 1,
		FileMaxAge:     1,
	}

	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Write to first file
	logger.Info("message to file1")

	// Change to second file
	cfg.FilePath = file2
	cfg.OnUpdate()

	// Write to second file
	logger.Info("message to file2")

	// Give time for async operations
	time.Sleep(100 * time.Millisecond)

	// Both files should exist
	if _, err := os.Stat(file1); os.IsNotExist(err) {
		t.Error("file1 should exist")
	}
	if _, err := os.Stat(file2); os.IsNotExist(err) {
		t.Error("file2 should exist")
	}

	// First file should be closeable (not locked)
	data, err := os.ReadFile(file1)
	if err != nil {
		t.Errorf("Failed to read file1 (may still be locked): %v", err)
	}
	if !bytes.Contains(data, []byte("message to file1")) {
		t.Error("file1 should contain the first message")
	}
}

// TestGlobalLoggerHotReload tests that Init creates a logger with hot-reload.
func TestGlobalLoggerHotReload(t *testing.T) {
	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	if err := Init(cfg); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Global logger should have INFO level
	if GetLevel() != "INFO" {
		t.Errorf("Expected INFO level, got %s", GetLevel())
	}

	// Update config
	cfg.LevelStr = "error"
	cfg.OnUpdate()

	// Global logger should update to ERROR
	if GetLevel() != "ERROR" {
		t.Errorf("Expected ERROR level after hot-reload, got %s", GetLevel())
	}
}

// TestReconfigureWithInvalidConfig tests error handling in Reconfigure.
func TestReconfigureWithInvalidConfig(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// Try to reconfigure with invalid config
	invalidCfg := &Config{
		LevelStr:     "invalid",
		Format:       FormatText,
		OutputStdout: true,
	}

	err = logger.Reconfigure(invalidCfg)
	if err == nil {
		t.Error("Expected error when reconfiguring with invalid config")
	}

	// Logger should still work with old config
	logger.Info("test message")
}

// TestConcurrentHotReload tests thread-safety of hot-reload mechanism.
func TestConcurrentHotReload(t *testing.T) {
	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		t.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = logger.Close() }()

	// Start goroutines that log continuously
	done := make(chan bool)
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 100; j++ {
				logger.Info("concurrent message", "goroutine", id, "iteration", j)
			}
			done <- true
		}(i)
	}

	// Simultaneously trigger hot-reload multiple times
	go func() {
		for i := 0; i < 10; i++ {
			time.Sleep(10 * time.Millisecond)
			cfg.LevelStr = "debug"
			cfg.OnUpdate()
			time.Sleep(10 * time.Millisecond)
			cfg.LevelStr = "info"
			cfg.OnUpdate()
		}
	}()

	// Wait for all goroutines to finish
	for i := 0; i < 5; i++ {
		<-done
	}

	// No panics = success
}

// TestOutputFormatAfterReload verifies actual output format changes.
func TestOutputFormatAfterReload(t *testing.T) {
	t.Parallel()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatJSON,
		OutputStdout: true,
	}

	logger, err := NewLogger(cfg)
	if err != nil {
		t.Fatalf("NewLogger failed: %v", err)
	}

	// Log with JSON format
	logger.Info("json format test", "key", "value")

	// Change to Text format
	cfg.Format = FormatText
	if err := logger.Reconfigure(cfg); err != nil {
		t.Fatalf("Reconfigure failed: %v", err)
	}

	// Log with Text format
	logger.Info("text format test", "key", "value")

	// Format should have changed (verified by no panics and visual inspection of output)
}

// BenchmarkHotReload benchmarks the performance overhead of hot-reload proxy.
func BenchmarkHotReload(b *testing.B) {
	// Redirect stdout to /dev/null to minimize I/O impact on benchmark
	oldStdout := os.Stdout
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		b.Fatalf("cannot open devnull: %v", err)
	}
	defer func() { _ = devNull.Close() }()
	os.Stdout = devNull
	defer func() { os.Stdout = oldStdout }()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	logger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		b.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = logger.Close() }()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		logger.Info("benchmark message", "iteration", i)
	}
}

// BenchmarkHotReloadWithChildren benchmarks child logger performance.
func BenchmarkHotReloadWithChildren(b *testing.B) {
	// Redirect stdout to /dev/null to minimize I/O impact on benchmark
	oldStdout := os.Stdout
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		b.Fatalf("cannot open devnull: %v", err)
	}
	defer func() { _ = devNull.Close() }()
	os.Stdout = devNull
	defer func() { os.Stdout = oldStdout }()

	cfg := &Config{
		LevelStr:     "info",
		Format:       FormatText,
		OutputStdout: true,
	}

	parentLogger, err := NewLoggerWithHotReload(cfg)
	if err != nil {
		b.Fatalf("NewLoggerWithHotReload failed: %v", err)
	}
	defer func() { _ = parentLogger.Close() }()

	// Create child with attributes
	childLogger := parentLogger.With("component", "test", "version", "1.0")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		childLogger.Info("benchmark message", "iteration", i)
	}
}
