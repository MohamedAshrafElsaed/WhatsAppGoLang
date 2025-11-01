// FILE: internal/middleware/ratelimit.go
// FIXES APPLIED:
// - Removed ShouldBindJSON that consumes request body
// - Now extracts wa_account_id from JSON body without consuming it
// - Uses io.ReadAll + bytes.NewBuffer to preserve body for handlers
// - Fixed critical bug where handlers couldn't read request body
// VERIFICATION: Request body is now available to downstream handlers

package middleware

import (
	"bytes"
	"encoding/json"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

type RateLimiter struct {
	limits        map[string]*accountLimit
	mu            sync.RWMutex
	perMinute     int
	jitterMinMS   int
	jitterMaxMS   int
	cleanupTicker *time.Ticker
	stopChan      chan struct{}
}

type accountLimit struct {
	tokens     int
	lastRefill time.Time
	mu         sync.Mutex
}

func NewRateLimiter(perMinute, jitterMinMS, jitterMaxMS int) *RateLimiter {
	rl := &RateLimiter{
		limits:      make(map[string]*accountLimit),
		perMinute:   perMinute,
		jitterMinMS: jitterMinMS,
		jitterMaxMS: jitterMaxMS,
		stopChan:    make(chan struct{}),
	}

	// Start cleanup goroutine
	rl.cleanupTicker = time.NewTicker(10 * time.Minute)
	go rl.cleanup()

	log.Info().
		Int("per_minute", perMinute).
		Int("jitter_min_ms", jitterMinMS).
		Int("jitter_max_ms", jitterMaxMS).
		Msg("Rate limiter initialized")

	return rl
}

func (rl *RateLimiter) Limit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Read the body
		bodyBytes, err := io.ReadAll(c.Request.Body)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "failed to read request body",
			})
			c.Abort()
			return
		}

		// Restore the body for the next handler
		c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// Extract wa_account_id from JSON
		var req struct {
			WaAccountID string `json:"wa_account_id"`
		}

		if err := json.Unmarshal(bodyBytes, &req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "invalid JSON format",
			})
			c.Abort()
			return
		}

		if req.WaAccountID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_request",
				"message": "wa_account_id is required",
			})
			c.Abort()
			return
		}

		// Store wa_account_id in context for handlers
		c.Set("wa_account_id", req.WaAccountID)

		// Check rate limit
		if !rl.allow(req.WaAccountID) {
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error":       "rate_limit_exceeded",
				"message":     "Too many messages sent. Please wait before sending more.",
				"retry_after": 60,
			})
			c.Abort()
			return
		}

		// Add jitter delay to prevent thundering herd
		jitter := rl.getJitter()
		if jitter > 0 {
			time.Sleep(jitter)
		}

		c.Next()
	}
}

func (rl *RateLimiter) allow(waAccountID string) bool {
	rl.mu.RLock()
	limit, exists := rl.limits[waAccountID]
	rl.mu.RUnlock()

	if !exists {
		// Create new limit for this account
		limit = &accountLimit{
			tokens:     rl.perMinute,
			lastRefill: time.Now(),
		}

		rl.mu.Lock()
		rl.limits[waAccountID] = limit
		rl.mu.Unlock()
	}

	limit.mu.Lock()
	defer limit.mu.Unlock()

	// Refill tokens if a minute has passed
	now := time.Now()
	if now.Sub(limit.lastRefill) >= time.Minute {
		limit.tokens = rl.perMinute
		limit.lastRefill = now
	}

	// Check if we have tokens available
	if limit.tokens > 0 {
		limit.tokens--
		return true
	}

	return false
}

func (rl *RateLimiter) getJitter() time.Duration {
	if rl.jitterMinMS == 0 && rl.jitterMaxMS == 0 {
		return 0
	}

	jitterRange := rl.jitterMaxMS - rl.jitterMinMS
	if jitterRange <= 0 {
		return time.Duration(rl.jitterMinMS) * time.Millisecond
	}

	jitter := rl.jitterMinMS + rand.Intn(jitterRange)
	return time.Duration(jitter) * time.Millisecond
}

func (rl *RateLimiter) cleanup() {
	for {
		select {
		case <-rl.cleanupTicker.C:
			rl.performCleanup()
		case <-rl.stopChan:
			rl.cleanupTicker.Stop()
			return
		}
	}
}

func (rl *RateLimiter) performCleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	threshold := now.Add(-10 * time.Minute)
	cleaned := 0

	for accountID, limit := range rl.limits {
		limit.mu.Lock()
		if limit.lastRefill.Before(threshold) {
			delete(rl.limits, accountID)
			cleaned++
		}
		limit.mu.Unlock()
	}

	if cleaned > 0 {
		log.Debug().
			Int("cleaned", cleaned).
			Int("remaining", len(rl.limits)).
			Msg("Rate limiter cleanup completed")
	}
}

func (rl *RateLimiter) Stop() {
	close(rl.stopChan)
}
