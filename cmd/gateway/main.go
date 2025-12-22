package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"instant/internal/consul"
	"instant/internal/gateway"
	"instant/internal/logger"
	"instant/internal/session"

	_ "instant/docs/swagger"
	_ "instant/internal/comments"
	_ "instant/internal/files"
	_ "instant/internal/follow"
	_ "instant/internal/likes"
	_ "instant/internal/posts"
	_ "github.com/joho/godotenv/autoload"
)

// @title Instant API Gateway
// @version 1.0
// @description Microservices API Gateway for Instant social media platform
// @description This API provides access to authentication, posts, files, likes, comments, and follow services.

// @contact.name API Support
// @contact.email support@instant.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:8080
// @BasePath /

// @securityDefinitions.apikey SessionAuth
// @in cookie
// @name session_id
// @description Session-based authentication via cookie

func main() {
	// Initialize structured logger
	log := logger.New()
	logger.SetDefault(log)

	// Load configuration from environment
	port := getEnv("GATEWAY_PORT", "8080")
	consulAddr := getEnv("CONSUL_HTTP_ADDR", "localhost:8500")
	consulToken := getEnv("CONSUL_HTTP_TOKEN", "")
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	redisPassword := getEnv("REDIS_PASSWORD", "")
	redisDB := 0

	slog.Info("Starting API Gateway",
		"port", port,
		"consul_addr", consulAddr,
		"redis_addr", redisAddr,
	)

	// Initialize Consul client
	consulClient, err := consul.NewClientWithToken(consulAddr, consulToken)
	if err != nil {
		slog.Error("Failed to create Consul client", "error", err)
		os.Exit(1)
	}
	slog.Info("Connected to Consul")

	// Initialize Redis session store
	store := session.NewRedisStore(redisAddr, redisPassword, redisDB)
	sessionMgr := session.NewManager(store)
	slog.Info("Connected to Redis")

	// Setup router
	router := gateway.SetupRouter(consulClient, sessionMgr)

	// Create HTTP server
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in a goroutine
	go func() {
		slog.Info("API Gateway listening", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shut down
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down API Gateway")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
		os.Exit(1)
	}

	slog.Info("API Gateway stopped")
}

// getEnv retrieves an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
