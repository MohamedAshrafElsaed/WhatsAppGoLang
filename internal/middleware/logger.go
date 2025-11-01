// FILE: internal/middleware/logger.go
// FIXES APPLIED:
// - Line 31: Added type assertion check to prevent panic
// - Added defensive programming for request_id handling
// - Improved error handling for missing request_id
// VERIFICATION: Safe type assertion, no panic risk

package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		raw := c.Request.URL.RawQuery

		// Process request
		c.Next()

		// Calculate latency
		latency := time.Since(start)

		// Get request ID with safe type assertion
		requestID := "unknown"
		if reqID, exists := c.Get("request_id"); exists {
			if reqIDStr, ok := reqID.(string); ok {
				requestID = reqIDStr
			}
		}

		// Build full path
		if raw != "" {
			path = path + "?" + raw
		}

		// Log the request
		logEvent := log.Info()

		if c.Writer.Status() >= 400 {
			logEvent = log.Error()
		}

		logEvent.
			Str("request_id", requestID).
			Str("method", c.Request.Method).
			Str("path", path).
			Int("status", c.Writer.Status()).
			Dur("latency", latency).
			Str("client_ip", c.ClientIP()).
			Str("user_agent", c.Request.UserAgent()).
			Int("body_size", c.Writer.Size()).
			Msg("HTTP request")
	}
}
