package say

import "log/slog"

// ErrorAttr creates a slog.Attr for an error.
// Returns "<nil>" string if err is nil to prevent panic.
func ErrorAttr(err error) slog.Attr {
	if err == nil {
		return slog.String("error", "<nil>")
	}
	return slog.String("error", err.Error())
}
