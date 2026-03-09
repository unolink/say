//go:build verify

package say_test

import (
	"context"
	"log/slog"
	"time"

	"github.com/unolink/say"
)

func ExampleInit() {
	cfg := &say.Config{}
	cfg.SetDefaults()
	cfg.LevelStr = "info"
	cfg.Format = say.FormatText

	if err := say.Init(cfg); err != nil {
		panic(err)
	}

	say.Info("Application started")
	// Output:
}

func ExampleInit_file() {
	compress := true
	cfg := &say.Config{
		LevelStr:       "info",
		Format:         say.FormatJSON,
		OutputStdout:   true,
		OutputFile:     true,
		FilePath:       "/tmp/app.log",
		FileMaxSize:    100,
		FileMaxBackups: 5,
		FileMaxAge:     30,
		FileCompress:   &compress,
	}

	if err := say.Init(cfg); err != nil {
		panic(err)
	}

	say.Info("Application started with file logging")
	// Output:
}

func Example_basicLogging() {
	cfg := &say.Config{}
	cfg.SetDefaults()
	_ = say.Init(cfg)

	say.Info("User logged in", "user_id", "123")
	say.Debug("Processing request", "request_id", "req-456")
	say.Warn("Rate limit approaching", "current", 95, "limit", 100)
	say.Error("Failed to connect", "error", "connection timeout")
	// Output:
}

func Example_contextLogging() {
	cfg := &say.Config{}
	cfg.SetDefaults()
	_ = say.Init(cfg)

	ctx := context.Background()
	say.InfoContext(ctx, "Request processed",
		"method", "GET",
		"path", "/api/users",
		"status", 200,
	)
	// Output:
}

func Example_withAttributes() {
	cfg := &say.Config{}
	cfg.SetDefaults()
	_ = say.Init(cfg)

	say.Info("User logged in",
		slog.String("user_id", "123"),
		slog.String("ip", "192.168.1.1"),
		slog.Time("timestamp", time.Now()),
	)
	// Output:
}

func Example_withGroupedAttributes() {
	cfg := &say.Config{}
	cfg.SetDefaults()
	_ = say.Init(cfg)

	say.Info("Request completed",
		slog.Group("request",
			slog.String("method", "POST"),
			slog.String("path", "/api/users"),
			slog.Int("status", 200),
		),
		slog.Group("user",
			slog.String("id", "123"),
			slog.String("email", "user@example.com"),
		),
	)
	// Output:
}

func Example_withLogger() {
	cfg := &say.Config{}
	cfg.SetDefaults()
	_ = say.Init(cfg)

	logger := say.GetLogger().With(
		slog.String("component", "auth"),
		slog.String("version", "1.0.0"),
	)
	logger.Info("Authentication successful", "user_id", "123")
	logger.Error("Authentication failed", "error", "invalid credentials")
	// Output:
}
