/*
Package say provides structured logging built on top of the standard slog logger.

# Overview

The say package implements a convenient logging system that supports:
  - Structured (JSON) and console (text) logging formats
  - Writing logs to stdout and files with automatic rotation
  - Adding attributes and context to log entries
  - Configuring log level and output format
  - Hot-reload of configuration without restart

# Initialization

## Basic initialization

	import "github.com/unolink/say"

	// Use default configuration (text format, stdout)
	err := say.Init(nil)
	if err != nil {
		panic(err)
	}

## Initialization with configuration

	cfg := &say.Config{}
	cfg.SetDefaults()
	cfg.LevelStr = "debug"
	cfg.Format = say.FormatJSON
	cfg.AddSource = true

	err := say.Init(cfg)
	if err != nil {
		panic(err)
	}

## Initialization with file output

	compress := true
	cfg := &say.Config{
		LevelStr:       "info",
		Format:         say.FormatJSON,
		OutputStdout:   true,
		OutputFile:     true,
		FilePath:       "/var/log/app.log",
		FileMaxSize:    100,    // MB
		FileMaxBackups: 5,
		FileMaxAge:     30,    // days
		FileCompress:   &compress,
	}

	err := say.Init(cfg)
	if err != nil {
		panic(err)
	}

# Usage

## Simple logging

	// After initialization, use the global convenience functions
	say.Info("Application started")
	say.Debug("Processing request", "user_id", "123")
	say.Warn("Rate limit approaching", "current", 95, "limit", 100)
	say.Error("Failed to connect", "error", err)

## Logging with context

	ctx := context.Background()
	say.InfoContext(ctx, "Request processed", "method", "GET", "path", "/api/users")
	say.ErrorContext(ctx, "Request failed", "error", err)

## Logging with attributes

	say.Info("User logged in",
		slog.String("user_id", "123"),
		slog.String("ip", "192.168.1.1"),
		slog.Time("timestamp", time.Now()),
	)

## Logging with grouped attributes

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

## Creating a logger with context

	// Obtain the logger and create a child with attributes
	logger := say.GetLogger() // Always valid (returns slog.Default if not initialized)
	logger = logger.With(
		slog.String("component", "auth"),
		slog.String("version", "1.0.0"),
	)
	logger.Info("Authentication successful", "user_id", "123")
	logger.Error("Authentication failed", "error", err)

# Examples

## In an HTTP handler

	func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
		requestID := r.Header.Get("X-Request-ID")
		logger := say.GetLogger().With(slog.String("request_id", requestID))

		userID := r.URL.Query().Get("id")
		if userID == "" {
			logger.Warn("Missing user ID parameter")
			http.Error(w, "Missing user ID", http.StatusBadRequest)
			return
		}

		logger.Info("Fetching user", "user_id", userID)

		user, err := h.userService.GetUser(userID)
		if err != nil {
			logger.Error("Failed to get user", "error", err, "user_id", userID)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		logger.Info("User fetched successfully", "user_id", userID)
		json.NewEncoder(w).Encode(user)
	}

## In a service layer

	func (s *UserService) CreateUser(ctx context.Context, user *User) error {
		logger := say.GetLogger().With(
			slog.String("operation", "create_user"),
			slog.String("email", user.Email),
		)

		logger.Info("Creating user")

		if err := s.validateUser(user); err != nil {
			logger.Warn("User validation failed", "error", err)
			return err
		}

		start := time.Now()
		if err := s.repo.Save(ctx, user); err != nil {
			logger.Error("Failed to save user", "error", err)
			return err
		}

		logger.Info("User created successfully",
			slog.Duration("duration", time.Since(start)),
			slog.String("user_id", user.ID),
		)

		return nil
	}

## With performance measurement

	func (s *Service) ProcessRequest(ctx context.Context, req *Request) error {
		start := time.Now()
		logger := say.GetLogger().With(
			slog.String("request_id", req.ID),
			slog.String("operation", "process_request"),
		)

		logger.Info("Processing request")

		// ... process the request ...

		duration := time.Since(start)
		logger.Info("Request processed",
			slog.Duration("duration", duration),
			slog.Int("status", 200),
		)

		return nil
	}

# Configuration

## Log formats

  - FormatJSON — structured JSON format, convenient for parsing and analysis
  - FormatText — human-readable text format, convenient for development

## Log levels

  - slog.LevelDebug — debug information
  - slog.LevelInfo — informational messages
  - slog.LevelWarn — warnings
  - slog.LevelError — errors

## File rotation

When file output is enabled, logs are automatically rotated:
  - By size (FileMaxSize in megabytes)
  - By age (FileMaxAge in days)
  - With a configurable number of retained backups (FileMaxBackups)
  - With optional compression of old files (FileCompress)

# Best practices

 1. Initialize the logger at application startup
 2. Use structured logging (JSON) in production
 3. Use console format (text) for development
 4. Add contextual information via attributes
 5. Group related attributes with GroupAttr
 6. Log errors with full context
 7. Use the appropriate log level for each message
 8. Measure performance using slog.Duration attributes
 9. Never log sensitive information (passwords, tokens, etc.)
*/
package say
