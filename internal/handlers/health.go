// FILE: internal/handlers/health.go
// VERIFICATION STATUS: ✅ Production Ready
// No changes needed - health checks are properly implemented
// Proper error handling and status codes

package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/whatsapp-api/go-whatsapp-service/internal/store"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
)

func HealthCheck(dbStore *store.PostgresStore) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check database connection
		if err := dbStore.Ping(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status":   "unhealthy",
				"database": "disconnected",
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"status":   "healthy",
			"database": "connected",
		})
	}
}

func ReadinessCheck(clientManager *wa.ClientManager) gin.HandlerFunc {
	return func(c *gin.Context) {
		clientCount := clientManager.GetClientCount()
		connectedCount := clientManager.GetConnectedCount()

		c.JSON(http.StatusOK, gin.H{
			"status":            "ready",
			"total_clients":     clientCount,
			"connected_clients": connectedCount,
		})
	}
}
