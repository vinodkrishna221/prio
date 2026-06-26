package main

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lastminutelifesaver/gateway/internal/agent"
	"github.com/lastminutelifesaver/gateway/internal/cache"
	"github.com/lastminutelifesaver/gateway/internal/handlers"
	"github.com/lastminutelifesaver/gateway/internal/oauth"
	"github.com/lastminutelifesaver/gateway/internal/queue"
	"github.com/lastminutelifesaver/gateway/internal/sse"
	"github.com/lastminutelifesaver/gateway/internal/workspace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// corsMiddleware adds CORS headers so the SvelteKit frontend (and dashboard)
// can reach the Go gateway from a different origin in development.
func corsMiddleware(allowedOrigin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		// Allow any origin that matches the configured dashboard URL, or
		// any localhost origin when running in dev mode.
		if origin != "" && (origin == allowedOrigin || strings.HasPrefix(origin, "http://localhost:")) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Requested-With")
			// SSE connections must not be buffered.
			w.Header().Set("Access-Control-Expose-Headers", "Content-Type")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	// Load .env file — try several candidate paths so the server works
	// whether invoked via `go run`, a compiled binary, or from any CWD.
	_, callerFile, _, _ := runtime.Caller(0)
	candidates := []string{
		// Relative to the source file (works with `go run`)
		filepath.Join(filepath.Dir(callerFile), "..", "..", "..", ".env"),
		// Relative to CWD (works when running from services/go-gateway)
		filepath.Join("..", "..", ".env"),
		// Same directory as the binary / CWD fallback
		".env",
	}
	for _, candidate := range candidates {
		if err := godotenv.Load(candidate); err == nil {
			slog.Info("loaded environment from .env file", "path", candidate)
			break
		}
	}

	// Initialize JSON structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("starting ingestion gateway service initialization")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 1. Gather configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	clientID := os.Getenv("GOOGLE_CLIENT_ID")
	clientSecret := os.Getenv("GOOGLE_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		slog.Warn("GOOGLE_CLIENT_ID or GOOGLE_CLIENT_SECRET is empty. OAuth redirects will fail until supplied.")
	}

	redirectURL := os.Getenv("REDIRECT_URL")
	if redirectURL == "" {
		redirectURL = "http://localhost:8080/auth/callback"
	}

	dashboardURL := os.Getenv("DASHBOARD_URL")
	if dashboardURL == "" {
		dashboardURL = "http://localhost:5173/"
	}

	convexURL := os.Getenv("CONVEX_URL")
	convexKey := os.Getenv("CONVEX_DEPLOY_KEY")
	if convexURL == "" {
		slog.Warn("CONVEX_URL is empty; database mutations will fail.")
	}

	// 2. Select and initialize KMS implementation
	var kmsService oauth.KMSService
	kmsKeyID := os.Getenv("KMS_KEY_ID")
	if kmsKeyID == "" || strings.HasPrefix(kmsKeyID, "mock://") {
		slog.Warn("KMS_KEY_ID not specified or is mock; using MockKMSService with local AES-GCM wrapping")
		kmsService = oauth.NewMockKMSService("local-dev-mock-kms-passphrase-32b")
	} else {
		var err error
		kmsService, err = oauth.NewGCPKMSService(ctx, kmsKeyID)
		if err != nil {
			slog.Error("failed to initialize GCP KMS client", "error", err)
			os.Exit(1)
		}
		slog.Info("successfully initialized GCP KMS Service client", "keyID", kmsKeyID)
	}

	// 3. Establish session cookie key
	sessionSecret := os.Getenv("SESSION_SECRET")
	var sessionSecretBytes []byte
	if sessionSecret == "" {
		slog.Warn("SESSION_SECRET is empty; generating a transient random secret key for session signing")
		sessionSecretBytes = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, sessionSecretBytes); err != nil {
			slog.Error("failed to generate transient random key", "error", err)
			os.Exit(1)
		}
	} else {
		sessionSecretBytes = []byte(sessionSecret)
	}

	// 4. Initialize Sub-services (gRPC client, Redis cache, SSE broker, Cloud Tasks dispatcher)
	agentClient, err := agent.GetClient()
	if err != nil {
		slog.Error("failed to initialize gRPC agent client", "error", err)
		os.Exit(1)
	}

	cacheManager, err := cache.NewCacheManager()
	if err != nil {
		slog.Error("failed to initialize redis cache manager", "error", err)
		os.Exit(1)
	}

	sseBroker := sse.NewBroker()

	taskDispatcher, err := queue.NewCloudTasksDispatcher(ctx)
	if err != nil {
		slog.Error("failed to initialize cloud tasks dispatcher", "error", err)
		os.Exit(1)
	}

	// 5. Initialize SyncService and AuthHandler
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes: []string{
			"openid",
			"https://www.googleapis.com/auth/userinfo.email",
			"https://www.googleapis.com/auth/gmail.compose",
			"https://www.googleapis.com/auth/gmail.readonly",
			"https://www.googleapis.com/auth/calendar",
			"https://www.googleapis.com/auth/tasks",
		},
		Endpoint: google.Endpoint,
	}
	convexClient := oauth.NewConvexClient(convexURL, convexKey)
	syncService := workspace.NewSyncService(config, convexClient, kmsService, cacheManager)

	authHandler := handlers.NewAuthHandler(
		clientID,
		clientSecret,
		redirectURL,
		dashboardURL,
		convexURL,
		convexKey,
		kmsService,
		sessionSecretBytes,
		agentClient,
		cacheManager,
		sseBroker,
		taskDispatcher,
		syncService,
	)

	// 6. Register HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/google", authHandler.HandleGoogleLogin)
	mux.HandleFunc("/auth/callback", authHandler.HandleGoogleCallback)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	// Gmail Webhooks
	mux.HandleFunc("/v1/users/me/watch", authHandler.HandleGmailWatch)
	mux.HandleFunc("/webhooks/gmail", authHandler.HandleGmailWebhook)

	// Tasks Sync & Completed Webhooks
	mux.HandleFunc("/tasks/sync", authHandler.HandleTasksSync)
	mux.HandleFunc("/webhooks/tasks", authHandler.HandleTaskCompletedWebhook)
	mux.HandleFunc("POST /v1/tasks/{taskId}/execute", authHandler.HandleTaskExecuteCard)

	// Google Cloud Tasks execute callback
	mux.HandleFunc("/tasks/execute", authHandler.HandleTaskExecute)

	// SSE Events Stream endpoint
	mux.HandleFunc("/v1/events", authHandler.HandleEventsStream)

	// Biological energy state update endpoint
	mux.HandleFunc("/api/user/energy-state", authHandler.HandleEnergyState)

	// Wrap the mux with CORS middleware so the browser frontend can reach the gateway.
	corsHandler := corsMiddleware(dashboardURL, mux)

	server := &http.Server{
		Addr:    ":" + port,
		Handler: corsHandler,
		// SSE connections are long-lived; WriteTimeout must be 0 so streaming
		// responses are not killed after the timeout window.
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	// 7. Start server asynchronously
	go func() {
		slog.Info("http server listening for connections", "port", port)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	// 8. Handle signals for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	sig := <-sigChan
	slog.Info("termination signal received; beginning graceful shutdown", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown returned an error", "error", err)
	}

	// Close gRPC agent client
	if err := agentClient.Close(); err != nil {
		slog.Error("failed to close gRPC agent client", "error", err)
	}

	// Close cache manager (Redis)
	if err := cacheManager.Close(); err != nil {
		slog.Error("failed to close redis cache manager", "error", err)
	}

	// Close Cloud Tasks dispatcher
	if err := taskDispatcher.Close(); err != nil {
		slog.Error("failed to close cloud tasks dispatcher", "error", err)
	}

	// If using GCPKMSService, close it
	if gcpKMS, ok := kmsService.(*oauth.GCPKMSService); ok {
		if err := gcpKMS.Close(); err != nil {
			slog.Error("failed to close GCP KMS client connection", "error", err)
		}
	}

	slog.Info("graceful shutdown of ingestion gateway complete")
}
