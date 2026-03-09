package say

import (
	"context"
	"io"
	"log/slog"
	"sync"
)

// HandlerFn defines a function that transforms a handler by applying attributes or groups.
// This is used to replay accumulated transformations (WithAttrs, WithGroup) when the
// underlying handler is replaced during hot-reload.
type HandlerFn func(slog.Handler) slog.Handler

// ReloadableHandler is a proxy handler that allows swapping the underlying implementation
// (e.g., JSON <-> Text, File <-> Stdout) without recreating the logger.
// It maintains a chain of transformations (from WithAttrs/WithGroup calls) and applies
// them to the current handler on each log operation.
type ReloadableHandler struct {
	// core is a shared container holding the actual handler.
	// All loggers created from the same Config share this container.
	core *HandlerContainer

	// chain stores accumulated transformations (WithAttrs, WithGroup) for this specific logger.
	// When the handler is replaced, these transformations are reapplied to the new handler.
	chain []HandlerFn
}

// HandlerContainer holds the current active handler and manages resource lifecycle.
// It uses RWMutex to allow concurrent reads (logging) while preventing races during updates.
type HandlerContainer struct {
	handler slog.Handler
	closer  io.Closer
	mu      sync.RWMutex
}

// Update replaces the underlying handler and closes the previous writer if applicable.
// The old closer is released OUTSIDE the lock to avoid blocking concurrent logging operations.
func (c *HandlerContainer) Update(newHandler slog.Handler, newCloser io.Closer) {
	c.mu.Lock()
	oldCloser := c.closer
	c.handler = newHandler
	c.closer = newCloser
	c.mu.Unlock()

	// Close old resource outside the lock to avoid blocking loggers
	if oldCloser != nil {
		_ = oldCloser.Close()
	}
}

// Load returns the current handler under read lock.
func (c *HandlerContainer) Load() slog.Handler {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.handler
}

// Enabled delegates the level check to the current handler.
func (h *ReloadableHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.core.Load().Enabled(ctx, level)
}

// Handle applies accumulated transformations to the current handler and writes the log record.
// This overhead (replaying the chain) is necessary to support hot-reload of handler format/output
// for child loggers created via With().
func (h *ReloadableHandler) Handle(ctx context.Context, record slog.Record) error {
	// 1. Get current handler (may be JSON, Text, or writing to a different file)
	handler := h.core.Load()

	// 2. Apply accumulated transformations (WithAttrs, WithGroup).
	// Required because the new handler doesn't have the context of child loggers.
	for _, fn := range h.chain {
		handler = fn(handler)
	}

	// 3. Write the log record
	return handler.Handle(ctx, record)
}

// WithAttrs returns a new handler with the specified attributes.
// The attributes are stored in the transformation chain rather than applied immediately,
// allowing them to work correctly when the underlying handler is replaced.
func (h *ReloadableHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newChain := make([]HandlerFn, len(h.chain), len(h.chain)+1)
	copy(newChain, h.chain)

	// Append a transformation that applies these attributes
	newChain = append(newChain, func(next slog.Handler) slog.Handler {
		return next.WithAttrs(attrs)
	})

	return &ReloadableHandler{
		core:  h.core, // Share the same container
		chain: newChain,
	}
}

// WithGroup returns a new handler with the specified group.
// The group is stored in the transformation chain rather than applied immediately,
// allowing it to work correctly when the underlying handler is replaced.
func (h *ReloadableHandler) WithGroup(name string) slog.Handler {
	newChain := make([]HandlerFn, len(h.chain), len(h.chain)+1)
	copy(newChain, h.chain)

	// Append a transformation that applies this group
	newChain = append(newChain, func(next slog.Handler) slog.Handler {
		return next.WithGroup(name)
	})

	return &ReloadableHandler{
		core:  h.core, // Share the same container
		chain: newChain,
	}
}
