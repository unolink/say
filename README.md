[Русская версия (README.ru.md)](README.ru.md)

# say

Structured logging package built on Go 1.21+ `slog` with file rotation and hot-reload support.

## Features

- **Built on slog** — uses Go's standard `log/slog` as the foundation
- **Multiple outputs** — write to stdout, files, or both simultaneously
- **File rotation** — automatic log rotation via lumberjack (size, age, backups)
- **Two formats** — JSON for machines, text for humans
- **Hot-reload** — change log level, format, and output without restart
- **Thread-safe** — concurrent logging from multiple goroutines
- **Zero config** — works out of the box with sensible defaults

## Install

```bash
go get github.com/unolink/say
```

## Quick Start

```go
package main

import "github.com/unolink/say"

func main() {
    // Initialize with defaults (text format, INFO level, stdout)
    if err := say.Init(nil); err != nil {
        panic(err)
    }

    say.Info("Application started")
    say.Debug("Debug message", "key", "value")  // Won't show (level is INFO)
    say.Error("Something failed", "error", "timeout")
}
```

### Custom Configuration

```go
compress := true
cfg := &say.Config{
    LevelStr:       "debug",
    Format:         say.FormatJSON,
    OutputStdout:   true,
    OutputFile:     true,
    FilePath:       "/var/log/app.log",
    FileMaxSize:    100,    // MB
    FileMaxBackups: 5,
    FileMaxAge:     30,     // days
    FileCompress:   &compress,
}

if err := say.Init(cfg); err != nil {
    panic(err)
}

say.Info("Application started", "config", "custom")
```

Output (JSON):
```json
{"time":"2024-01-15T10:30:00.123Z","level":"INFO","msg":"Application started","config":"custom"}
```

## Configuration Options

| Option | Type | Default | Description |
|--------|------|---------|-------------|
| `LevelStr` | string | `"info"` | Log level: debug, info, warn, error |
| `Format` | Format | `FormatText` | Output format: JSON or text |
| `OutputStdout` | bool | `true` | Enable stdout output |
| `OutputFile` | bool | `false` | Enable file output |
| `FilePath` | string | - | Path to log file (required if OutputFile=true) |
| `FileMaxSize` | int | `100` | Max file size in MB before rotation |
| `FileMaxBackups` | int | `3` | Max number of old log files to keep |
| `FileMaxAge` | int | `28` | Max age of log files in days |
| `FileCompress` | *bool | `true` | Compress rotated log files |
| `AddSource` | bool | `false` | Add source file location to logs |

## Usage Examples

### Structured Logging

```go
// Simple key-value pairs
say.Info("User logged in", "user_id", 123, "ip", "192.168.1.1")

// With context
ctx := context.Background()
say.InfoContext(ctx, "Request processed", "method", "GET", "duration", 45)

// Grouped attributes
say.Info("API call completed",
    slog.Group("request",
        slog.String("method", "POST"),
        slog.Int("status", 200),
    ),
    slog.Group("user",
        slog.Int("id", 123),
        slog.String("name", "Alice"),
    ),
)
```

### Logger With Attributes

```go
logger := say.GetLogger()
authLogger := logger.With(
    slog.String("component", "auth"),
    slog.String("version", "1.0.0"),
)

// All logs from authLogger include component and version
authLogger.Info("User authenticated", "user", "alice")
// Output: ... msg="User authenticated" component=auth version=1.0.0 user=alice
```

### Dynamic Level Changes

```go
say.Init(nil)
say.Debug("Debug 1")  // Not shown (default level is INFO)

say.SetLevel("debug")
say.Debug("Debug 2")  // Now shown
```

## Hot-Reload

### Observer Pattern

Loggers created with `NewLoggerWithHotReload` automatically update when their config changes:

```go
cfg := &say.Config{
    LevelStr:     "info",
    Format:       say.FormatText,
    OutputStdout: true,
}

logger, err := say.NewLoggerWithHotReload(cfg)
if err != nil {
    panic(err)
}
defer logger.Close() // IMPORTANT: prevents memory leaks

// Later, when config changes (e.g., from a file watcher):
cfg.LevelStr = "debug"
cfg.Format = say.FormatJSON
cfg.OnUpdate()  // All subscribed loggers update immediately
```

### Manual Reconfiguration

```go
logger, _ := say.NewLogger(cfg)

newCfg := &say.Config{
    LevelStr:     "debug",
    Format:       say.FormatJSON,
    OutputStdout: true,
}

if err := logger.Reconfigure(newCfg); err != nil {
    // handle error
}
```

### How It Works

1. `Config.Subscribe()` registers callbacks (observer pattern)
2. `Config.OnUpdate()` notifies all subscribers
3. `Logger.Reconfigure()` swaps the handler atomically
4. Child loggers (via `With()`) update automatically through shared `HandlerContainer`

### Custom Attribute Transformation

```go
cfg := &say.Config{
    ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
        if a.Key == "password" {
            return slog.String("password", "***REDACTED***")
        }
        return a
    },
}

logger, _ := say.NewLogger(cfg)
logger.Info("Login", "user", "alice", "password", "secret123")
// Output: ... msg=Login user=alice password=***REDACTED***
```

## Multiple Loggers

```go
// Service logger — console, DEBUG
serviceCfg := &say.Config{
    LevelStr:     "debug",
    Format:       say.FormatText,
    OutputStdout: true,
}
serviceLogger, _ := say.NewLogger(serviceCfg)

// Audit logger — file, INFO, JSON
auditCfg := &say.Config{
    LevelStr:   "info",
    Format:     say.FormatJSON,
    OutputFile: true,
    FilePath:   "/var/log/audit.log",
}
auditLogger, _ := say.NewLogger(auditCfg)
```

## Dependencies

- `log/slog` — Go stdlib (1.21+)
- `gopkg.in/natefinch/lumberjack.v2` — log rotation

## License

MIT — see [LICENSE](LICENSE).
