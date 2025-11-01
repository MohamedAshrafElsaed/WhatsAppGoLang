package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/whatsapp-api/go-whatsapp-service/internal/config"
	"github.com/whatsapp-api/go-whatsapp-service/internal/handlers"
	"github.com/whatsapp-api/go-whatsapp-service/internal/middleware"
	"github.com/whatsapp-api/go-whatsapp-service/internal/store"
	"github.com/whatsapp-api/go-whatsapp-service/internal/wa"
	"github.com/whatsapp-api/go-whatsapp-service/internal/webhooks"
)

func main() {
	// Configure logging
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	if os.Getenv("APP_ENV") == "production" {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
		log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr})
	}

	log.Info().Msg("Starting Go WhatsApp Service...")

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load configuration")
	}

	log.Info().
		Str("port", cfg.Port).
		Str("env", cfg.Env).
		Int("max_sessions", cfg.MaxConcurrentSessions).
		Dur("session_idle_ttl", cfg.SessionIdleTTL).
		Msg("Configuration loaded")

	// Initialize database store
	dbStore, err := store.NewPostgresStore(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize database store")
	}
	defer dbStore.Close()

	log.Info().Msg("Database store initialized")

	// Initialize webhook sender BEFORE client manager
	webhookSender := webhooks.NewSender(cfg.LaravelWebhookBase, cfg.SigningSecret)
	log.Info().Str("webhook_base", cfg.LaravelWebhookBase).Msg("Webhook sender initialized")

	// Initialize WhatsApp client manager WITH webhook sender
	clientManager := wa.NewClientManager(dbStore, cfg, webhookSender)
	log.Info().Msg("WhatsApp client manager initialized")

	// Setup Gin router
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(middleware.RequestID())
	router.Use(middleware.Logger())
	router.Use(middleware.CORS())

	// Apply rate limiting to message endpoints
	rateLimiter := middleware.NewRateLimiter(cfg.SendRatePerMinute, cfg.SendJitterMinMS, cfg.SendJitterMaxMS)

	// Health endpoints (no rate limiting)
	router.GET("/healthz", handlers.HealthCheck(dbStore))
	router.GET("/readyz", handlers.ReadinessCheck(clientManager))
	router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 routes
	v1 := router.Group("/v1")
	{
		// Session management
		sessions := v1.Group("/sessions")
		{
			h := handlers.NewSessionHandler(clientManager, webhookSender)
			sessions.POST("/:waAccountId/qr", h.GetQR)
			sessions.POST("/:waAccountId/pair", h.PairWithCode)
			sessions.POST("/:waAccountId/reconnect", h.Reconnect)
			sessions.POST("/:waAccountId/logout", h.Logout)
			sessions.GET("/:waAccountId/status", h.GetStatus)
		}

		// Message operations (WITH rate limiting)
		messages := v1.Group("/messages")
		messages.Use(rateLimiter.Limit())
		{
			h := handlers.NewMessageHandler(clientManager, webhookSender)
			messages.POST("", h.SendMessage)
			messages.POST("/:messageId/delete", h.DeleteMessage)
			messages.POST("/:messageId/revoke", h.RevokeMessage)
			messages.POST("/:messageId/react", h.ReactToMessage)
			messages.POST("/:messageId/update", h.UpdateMessage)
		}

		// Group operations
		groups := v1.Group("/groups")
		{
			h := handlers.NewGroupHandler(clientManager)
			groups.GET("", h.ListGroups)
			groups.POST("", h.CreateGroup)
			groups.POST("/join", h.JoinGroup)
			groups.GET("/preview", h.GetGroupPreview)
			groups.GET("/:groupId", h.GetGroupInfo)
			groups.POST("/:groupId/participants", h.ManageParticipants)
			groups.POST("/:groupId/photo", h.SetGroupPhoto)
			groups.POST("/:groupId/name", h.SetGroupName)
			groups.POST("/:groupId/locked", h.SetGroupLocked)
			groups.POST("/:groupId/announce", h.SetGroupAnnounce)
			groups.POST("/:groupId/topic", h.SetGroupTopic)
			groups.GET("/:groupId/invite_link", h.GetGroupInviteLink)
			groups.POST("/:groupId/leave", h.LeaveGroup)
		}

		// Account operations
		account := v1.Group("/account")
		{
			h := handlers.NewAccountHandler(clientManager)
			account.GET("/avatar", h.GetAvatar)
			account.POST("/avatar", h.ChangeAvatar)
			account.DELETE("/avatar", h.RemoveAvatar)
			account.POST("/push_name", h.ChangePushName)
			account.POST("/status", h.SetStatus)
			account.GET("/user_info", h.GetUserInfo)
			account.GET("/business_profile", h.GetBusinessProfile)
			account.GET("/privacy", h.GetPrivacySettings)
			account.GET("/user_check", h.CheckUserExists)
		}

		// Chat operations
		chats := v1.Group("/chats")
		{
			h := handlers.NewChatHandler(clientManager)
			chats.GET("", h.ListChats)
			chats.GET("/:chatId/messages", h.GetChatMessages)
			chats.POST("/:chatId/pin", h.PinChat)
			chats.POST("/:chatId/read", h.MarkAsRead)
			chats.POST("/:chatId/archive", h.ArchiveChat)
			chats.POST("/:chatId/mute", h.MuteChat)
		}

		// Contact operations
		contacts := v1.Group("/contacts")
		{
			h := handlers.NewContactHandler(clientManager)
			contacts.GET("", h.GetContacts)
			contacts.POST("/sync", h.SyncContacts)
		}

		// Newsletter operations
		newsletters := v1.Group("/newsletters")
		{
			h := handlers.NewNewsletterHandler(clientManager)
			newsletters.GET("", h.ListNewsletters)
		}
	}

	// Create server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info().
			Str("port", cfg.Port).
			Str("env", cfg.Env).
			Msg("🚀 Go WhatsApp Service started successfully")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("🛑 Shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Disconnect all WhatsApp clients
	log.Info().Msg("Disconnecting all WhatsApp clients...")
	clientManager.DisconnectAll()

	// Shutdown HTTP server
	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	} else {
		log.Info().Msg("Server exited gracefully")
	}
}
