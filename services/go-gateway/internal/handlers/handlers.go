package handlers

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	genomev1 "github.com/lastminutelifesaver/gateway/gen/genome/v1"
	triagev1 "github.com/lastminutelifesaver/gateway/gen/triage/v1"
	"github.com/lastminutelifesaver/gateway/internal/agent"
	"github.com/lastminutelifesaver/gateway/internal/cache"
	"github.com/lastminutelifesaver/gateway/internal/oauth"
	"github.com/lastminutelifesaver/gateway/internal/queue"
	"github.com/lastminutelifesaver/gateway/internal/sse"
	"github.com/lastminutelifesaver/gateway/internal/workspace"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
)

var defaultScopes = []string{
	"openid",
	"https://www.googleapis.com/auth/userinfo.email",
	"https://www.googleapis.com/auth/gmail.compose",
	"https://www.googleapis.com/auth/gmail.readonly",
	"https://www.googleapis.com/auth/calendar",
	"https://www.googleapis.com/auth/tasks",
}

// AuthHandler coordinates Google OAuth 2.0 flows and Convex data persistence.
type AuthHandler struct {
	oauthConfig    *oauth2.Config
	convex         *oauth.ConvexClient
	kms            oauth.KMSService
	cookieKey      []byte
	internalSecret []byte
	dashboardURL   string

	agentClient agent.AgentClient
	cache       cache.CalendarCache
	broker      *sse.Broker
	dispatcher  queue.TaskDispatcher
	syncService *workspace.SyncService
}

// NewAuthHandler creates an instance of AuthHandler.
func NewAuthHandler(
	clientID, clientSecret, redirectURL, dashboardURL, convexURL, convexKey string,
	kms oauth.KMSService,
	cookieKey []byte,
	internalSecret []byte,
	agentClient agent.AgentClient,
	cache cache.CalendarCache,
	broker *sse.Broker,
	dispatcher queue.TaskDispatcher,
	syncService *workspace.SyncService,
) *AuthHandler {
	config := &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
		Scopes:       defaultScopes,
		Endpoint:     google.Endpoint,
	}
	return &AuthHandler{
		oauthConfig:    config,
		convex:         oauth.NewConvexClient(convexURL, convexKey),
		kms:            kms,
		cookieKey:      cookieKey,
		internalSecret: internalSecret,
		dashboardURL:   dashboardURL,
		agentClient:    agentClient,
		cache:          cache,
		broker:         broker,
		dispatcher:     dispatcher,
		syncService:    syncService,
	}
}

// HandleGoogleLogin redirects the user to the Google OAuth consent page.
func (h *AuthHandler) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	state := "oauth-state-token"
	url := h.oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"))
	slog.Info("redirecting to google oauth consent screen", "url", url)
	http.Redirect(w, r, url, http.StatusTemporaryRedirect)
}

// HandleGoogleCallback processes the authorization code callback.
func (h *AuthHandler) HandleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := r.URL.Query().Get("code")
	if code == "" {
		slog.Error("oauth callback received with empty code parameter")
		http.Error(w, "missing code parameter", http.StatusBadRequest)
		return
	}

	token, err := h.oauthConfig.Exchange(ctx, code)
	if err != nil {
		slog.Error("failed to exchange authorization code", "error", err)
		http.Error(w, fmt.Sprintf("token exchange failed: %v", err), http.StatusInternalServerError)
		return
	}

	email, err := h.fetchUserEmail(ctx, token.AccessToken)
	if err != nil {
		slog.Error("failed to fetch user email via oauth token", "error", err)
		http.Error(w, fmt.Sprintf("email lookup failed: %v", err), http.StatusInternalServerError)
		return
	}

	var user oauth.ConvexUser
	err = h.convex.CallQuery(ctx, "mutations:getUserByEmail", map[string]interface{}{
		"email": email,
	}, &user)

	var userId string
	if err != nil || user.ID == "" {
		slog.Info("user profile not found, executing registration mutation", "email", email)
		var newUserId string
		err = h.convex.CallMutation(ctx, "mutations:createUser", map[string]interface{}{
			"email": email,
		}, &newUserId)
		if err != nil {
			slog.Error("failed to create user in database", "error", err)
			http.Error(w, fmt.Sprintf("database write failed: %v", err), http.StatusInternalServerError)
			return
		}
		userId = newUserId
	} else {
		userId = user.ID
	}

	accessTokenEncrypted, err := oauth.EncryptToken(ctx, h.kms, token.AccessToken)
	if err != nil {
		slog.Error("failed to encrypt access token", "error", err)
		http.Error(w, "token encryption failed", http.StatusInternalServerError)
		return
	}

	var refreshTokenEncrypted string
	if token.RefreshToken != "" {
		refreshTokenEncrypted, err = oauth.EncryptToken(ctx, h.kms, token.RefreshToken)
		if err != nil {
			slog.Error("failed to encrypt refresh token", "error", err)
			http.Error(w, "token encryption failed", http.StatusInternalServerError)
			return
		}
	}

	watchExpiration := time.Now().AddDate(0, 0, 7).UnixNano() / int64(time.Millisecond)
	var integrationId string
	err = h.convex.CallMutation(ctx, "mutations:saveIntegration", map[string]interface{}{
		"userId":               userId,
		"provider":             "GOOGLE_WORKSPACE",
		"accessTokenEncrypted": accessTokenEncrypted,
		"refreshTokenEncrypted": refreshTokenEncrypted,
		"watchExpiration":      watchExpiration,
	}, &integrationId)
	if err != nil {
		slog.Error("failed to save oauth credentials in integrations", "error", err)
		http.Error(w, fmt.Sprintf("database write failed: %v", err), http.StatusInternalServerError)
		return
	}

	cookieValue := h.setSessionCookie(w, userId)

	slog.Info("oauth authentication complete, redirecting to web client", "userId", userId)
	redirectTarget := fmt.Sprintf("%s/auth/callback?session_id=%s", strings.TrimSuffix(h.dashboardURL, "/"), url.QueryEscape(cookieValue))
	http.Redirect(w, r, redirectTarget, http.StatusTemporaryRedirect)
}

func (h *AuthHandler) fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	client := &http.Client{Timeout: 5 * time.Second}
	if ctxClient, ok := ctx.Value(oauth2.HTTPClient).(*http.Client); ok {
		client = ctxClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google userinfo request returned HTTP status %d", resp.StatusCode)
	}

	var info struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", err
	}

	return info.Email, nil
}

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, userId string) string {
	expiration := time.Now().Add(24 * time.Hour)
	expStr := strconv.FormatInt(expiration.Unix(), 10)

	payload := userId + ":" + expStr
	mac := hmac.New(sha256.New, h.cookieKey)
	mac.Write([]byte(payload))
	signature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	cookieValue := payload + ":" + signature

	cookie := &http.Cookie{
		Name:     "session_id",
		Value:    cookieValue,
		Path:     "/",
		Expires:  expiration,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	}

	http.SetCookie(w, cookie)
	return cookieValue
}

// VerifySession verifies the session cookie and extracts the Convex userId.
func (h *AuthHandler) VerifySession(r *http.Request) (string, error) {
	cookie, err := r.Cookie("session_id")
	if err != nil {
		return "", err
	}

	parts := strings.Split(cookie.Value, ":")
	if len(parts) != 3 {
		return "", errors.New("invalid session cookie format")
	}

	userId := parts[0]
	expStr := parts[1]
	signature := parts[2]

	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", errors.New("invalid session expiration")
	}
	if time.Now().Unix() > expUnix {
		return "", errors.New("session expired")
	}

	payload := userId + ":" + expStr
	mac := hmac.New(sha256.New, h.cookieKey)
	mac.Write([]byte(payload))
	expectedSignature := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return "", errors.New("invalid session signature")
	}

	return userId, nil
}

// VerifyInternalRequest authenticates a server-to-server call from the SvelteKit proxy.
// It checks the X-Internal-Auth header against the shared internal secret, and reads
// the proxied user identity from X-User-Id.
func (h *AuthHandler) VerifyInternalRequest(r *http.Request) (string, error) {
	if len(h.internalSecret) == 0 {
		return "", errors.New("handlers: internal secret not configured")
	}
	authHeader := r.Header.Get("X-Internal-Auth")
	if authHeader == "" {
		return "", errors.New("handlers: missing X-Internal-Auth header")
	}
	// Constant-time comparison to prevent timing attacks.
	if !hmac.Equal([]byte(authHeader), h.internalSecret) {
		return "", errors.New("handlers: invalid X-Internal-Auth header")
	}
	userId := r.Header.Get("X-User-Id")
	if userId == "" {
		return "", errors.New("handlers: missing X-User-Id header")
	}
	return userId, nil
}

// verifySSEToken validates a short-lived SSE bearer token issued by the SvelteKit server.
// Token format: userId:expUnixSecs:hmac-sha256-base64url
func (h *AuthHandler) verifySSEToken(token string) (string, error) {
	if len(h.internalSecret) == 0 {
		return "", errors.New("handlers: internal secret not configured")
	}
	parts := strings.Split(token, ":")
	if len(parts) != 3 {
		return "", errors.New("handlers: invalid SSE token format")
	}
	userId := parts[0]
	expStr := parts[1]
	sig := parts[2]

	expUnix, err := strconv.ParseInt(expStr, 10, 64)
	if err != nil {
		return "", errors.New("handlers: invalid SSE token expiry")
	}
	if time.Now().Unix() > expUnix {
		return "", errors.New("handlers: SSE token expired")
	}

	payload := userId + ":" + expStr
	mac := hmac.New(sha256.New, h.internalSecret)
	mac.Write([]byte(payload))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(sig), []byte(expectedSig)) {
		return "", errors.New("handlers: invalid SSE token signature")
	}
	return userId, nil
}

// verifyRequest authenticates a request via either internal proxy auth or session cookie.
// Internal auth takes priority (used by SvelteKit server-side proxy in production).
// Session cookie fallback supports local development where the cookie is same-origin.
func (h *AuthHandler) verifyRequest(r *http.Request) (string, error) {
	if userId, err := h.VerifyInternalRequest(r); err == nil {
		return userId, nil
	}
	return h.VerifySession(r)
}

// validateOIDCToken parses and validates OIDC token against the expected audience.
func validateOIDCToken(ctx context.Context, authHeader string, expectedAudience string) (string, error) {
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", fmt.Errorf("handlers: authorization header is missing Bearer prefix")
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	if os.Getenv("DISABLE_OIDC_VALIDATION") == "true" {
		slog.Warn("handlers: OIDC validation is disabled via DISABLE_OIDC_VALIDATION env var")
		return "mock-subject", nil
	}

	payload, err := idtoken.Validate(ctx, token, expectedAudience)
	if err != nil {
		return "", fmt.Errorf("handlers: idtoken validation failed: %w", err)
	}

	return payload.Subject, nil
}

// PubSubPushRequest defines the Google Cloud Pub/Sub push notification JSON payload.
type PubSubPushRequest struct {
	Message struct {
		Data        string    `json:"data"`
		MessageID   string    `json:"messageId"`
		PublishTime time.Time `json:"publishTime"`
	} `json:"message"`
	Subscription string `json:"subscription"`
}

// GmailWatchPayload defines the decoded payload structure from Gmail Pub/Sub notification data.
type GmailWatchPayload struct {
	EmailAddress string `json:"emailAddress"`
	HistoryID    uint64 `json:"historyId"`
}

// HandleGmailWebhook handles Google Cloud Pub/Sub push callbacks, parses email changes, and triages tasks.
func (h *AuthHandler) HandleGmailWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	audience := os.Getenv("WEBHOOK_AUDIENCE")
	if audience == "" {
		scheme := "https"
		if r.TLS == nil {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				scheme = proto
			} else {
				scheme = "http"
			}
		}
		audience = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.Path)
	}

	authHeader := r.Header.Get("Authorization")
	if _, err := validateOIDCToken(ctx, authHeader, audience); err != nil {
		slog.Error("handlers: gmail webhook OIDC verification failed", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var pushReq PubSubPushRequest
	if err := json.NewDecoder(r.Body).Decode(&pushReq); err != nil {
		slog.Error("handlers: failed to decode pubsub body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	dataBytes, err := base64.StdEncoding.DecodeString(pushReq.Message.Data)
	if err != nil {
		slog.Error("handlers: failed to decode pubsub data base64", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var watchPayload GmailWatchPayload
	if err := json.Unmarshal(dataBytes, &watchPayload); err != nil {
		slog.Error("handlers: failed to unmarshal gmail watch payload", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	slog.Info("handlers: received gmail watch event", "email", watchPayload.EmailAddress, "historyId", watchPayload.HistoryID)

	var user oauth.ConvexUser
	err = h.convex.CallQuery(ctx, "mutations:getUserByEmail", map[string]interface{}{
		"email": watchPayload.EmailAddress,
	}, &user)
	if err != nil || user.ID == "" {
		// Return 200 OK to ACK the Pub/Sub message. If we return any non-2xx (or even
		// some 2xx like 204 in some Pub/Sub configs) Google will retry the delivery
		// indefinitely. An unknown email address will never belong to a user, so we
		// must permanently drop it by returning 200.
		slog.Warn("handlers: gmail webhook ignored — email address not registered", "email", watchPayload.EmailAddress)
		w.WriteHeader(http.StatusOK)
		return
	}

	ts, err := h.syncService.GetGoogleTokenSource(ctx, user.ID)
	if err != nil {
		slog.Error("handlers: failed to get token source", "userId", user.ID, "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		slog.Error("handlers: failed to create gmail service", "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	msgList, err := gmailService.Users.Messages.List("me").Q("-label:DRAFT -label:SENT").MaxResults(1).Context(ctx).Do()
	if err != nil || len(msgList.Messages) == 0 {
		slog.Error("handlers: failed to list user messages", "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	msgID := msgList.Messages[0].Id
	threadID := msgList.Messages[0].ThreadId

	var existingTask map[string]interface{}
	err = h.convex.CallQuery(ctx, "queries:getTaskByExternalId", map[string]interface{}{
		"userId":         user.ID,
		"externalTaskId": msgID,
	}, &existingTask)
	if err == nil && existingTask != nil && existingTask["_id"] != nil {
		slog.Info("handlers: message already triaged, skipping", "userId", user.ID, "messageId", msgID)
		w.WriteHeader(http.StatusOK)
		return
	}

	msg, err := gmailService.Users.Messages.Get("me", msgID).Format("full").Context(ctx).Do()
	if err != nil {
		slog.Error("handlers: failed to retrieve message", "messageId", msgID, "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	var subject, sender string
	for _, header := range msg.Payload.Headers {
		switch strings.ToLower(header.Name) {
		case "subject":
			subject = header.Value
		case "from":
			sender = header.Value
		}
	}

	bodyContent := extractBodyText(msg.Payload)

	triageReq := &triagev1.ProcessTriageRequest{
		UserId:            user.ID,
		EmailId:           msgID,
		Subject:           subject,
		Sender:            sender,
		BodyContent:       bodyContent,
		ReceivedTimestamp: msg.InternalDate,
		UserContext: &triagev1.UserContext{
			EnergyScore: int32(user.CurrentEnergyScore),
		},
	}

	triageResp, err := h.agentClient.ProcessTriage(ctx, triageReq)
	if err != nil {
		slog.Error("handlers: triage gRPC call failed", "userId", user.ID, "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	slog.Info("handlers: triage evaluation completed", "userId", user.ID, "savesMinutes", triageResp.FrictionSavedMinutes)

	var draftID string
	if triageResp.ActionType == "GMAIL_DRAFT" && triageResp.DraftPayloadJson != "" {
		var payload struct {
			Body string `json:"body"`
		}
		_ = json.Unmarshal([]byte(triageResp.DraftPayloadJson), &payload)
		if payload.Body == "" {
			payload.Body = triageResp.DraftPayloadJson
		}

		replySubject := subject
		if !strings.HasPrefix(strings.ToLower(subject), "re:") {
			replySubject = "Re: " + subject
		}

		draftID, err = h.syncService.CreateGmailDraft(ctx, user.ID, threadID, msgID, sender, replySubject, payload.Body)
		if err != nil {
			slog.Error("handlers: failed to write reply draft to gmail", "userId", user.ID, "error", err)
		} else {
			slog.Info("handlers: draft reply constructed successfully", "draftId", draftID)
		}
	}

	savesMinutes := 15
	if mins, err := strconv.Atoi(triageResp.FrictionSavedMinutes); err == nil {
		savesMinutes = mins
	}

	actionCard := map[string]interface{}{
		"actionType":   triageResp.ActionType,
		"savesMinutes": float64(savesMinutes),
		"payloadJson":  triageResp.DraftPayloadJson,
	}
	if draftID != "" {
		actionCard["draftId"] = draftID
	}

	var newTaskId string
	err = h.convex.CallMutation(ctx, "mutations:ingestTriagedTask", map[string]interface{}{
		"userId":          user.ID,
		"title":           subject,
		"source":          "GMAIL",
		"priorityScore":   float64(triageResp.TriagePriorityScore),
		"durationMinutes": 15.0,
		"dueAt":           float64(time.Now().Add(24 * time.Hour).UnixNano() / int64(time.Millisecond)),
		"actionCard":      actionCard,
		"externalTaskId":  msgID,
	}, &newTaskId)
	if err != nil {
		slog.Error("handlers: failed to ingest task into database", "userId", user.ID, "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	h.broker.Broadcast(user.ID, sse.Event{
		Type: "TASK_TRIAGED",
		Data: map[string]interface{}{
			"taskId":        newTaskId,
			"title":         subject,
			"priorityScore": triageResp.TriagePriorityScore,
			"actionType":    triageResp.ActionType,
			"savesMinutes":  savesMinutes,
		},
	})

	w.WriteHeader(http.StatusOK)
}

func extractBodyText(part *gmail.MessagePart) string {
	if part.Body != nil && part.Body.Data != "" {
		decoded, err := base64.URLEncoding.DecodeString(part.Body.Data)
		if err == nil {
			return string(decoded)
		}
	}
	for _, subPart := range part.Parts {
		if text := extractBodyText(subPart); text != "" {
			return text
		}
	}
	return ""
}

// HandleGmailWatch sets up Gmail push notification watches for the user.
func (h *AuthHandler) HandleGmailWatch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := h.verifyRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	topic := os.Getenv("GMAIL_PUBSUB_TOPIC")
	if topic == "" {
		slog.Error("handlers: GMAIL_PUBSUB_TOPIC env is empty")
		http.Error(w, "server configuration missing topic", http.StatusInternalServerError)
		return
	}

	ts, err := h.syncService.GetGoogleTokenSource(ctx, userId)
	if err != nil {
		slog.Error("handlers: watch setup failed, missing oauth", "userId", userId, "error", err)
		http.Error(w, "unauthorized or missing integrations", http.StatusUnauthorized)
		return
	}

	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		slog.Error("handlers: failed to initialize gmail client for watch", "error", err)
		http.Error(w, "gmail watch init failed", http.StatusInternalServerError)
		return
	}

	watchReq := &gmail.WatchRequest{
		TopicName: topic,
		LabelIds:  []string{"INBOX"},
	}

	watchResp, err := gmailService.Users.Watch("me", watchReq).Context(ctx).Do()
	if err != nil {
		// Gmail watch registration can fail in local dev (Pub/Sub topic may not
		// exist yet). Treat as a soft warning so the UI sync does not hard-fail.
		slog.Warn("handlers: gmail watch registration failed (non-fatal)", "userId", userId, "error", err)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":  "warning",
			"warning": fmt.Sprintf("gmail watch skipped: %v", err),
		})
		return
	}

	var integrationId string
	var integration workspace.Integration
	err = h.convex.CallQuery(ctx, "queries:getIntegration", map[string]interface{}{
		"userId":   userId,
		"provider": "GOOGLE_WORKSPACE",
	}, &integration)
	if err == nil && integration.ID != "" {
		_ = h.convex.CallMutation(ctx, "mutations:saveIntegration", map[string]interface{}{
			"userId":               userId,
			"provider":             "GOOGLE_WORKSPACE",
			"accessTokenEncrypted": integration.AccessTokenEncrypted,
			"refreshTokenEncrypted": integration.RefreshTokenEncrypted,
			"watchExpiration":      watchResp.Expiration,
		}, &integrationId)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "success",
		"expiration": watchResp.Expiration,
		"historyId":  watchResp.HistoryId,
	})
}

// HandleTasksSync manually triggers the Google Tasks sync sequence.
func (h *AuthHandler) HandleTasksSync(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := h.verifyRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	err = h.syncService.SyncTasks(ctx, userId)
	if err != nil {
		slog.Error("handlers: tasks sync failed", "userId", userId, "error", err)
		http.Error(w, fmt.Sprintf("tasks sync failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "success",
	})
}

// TaskCompletedPayload defines the request structure for task completed webhooks.
type TaskCompletedPayload struct {
	UserID         string `json:"userId"`
	ExternalTaskID string `json:"externalTaskId"`
}

// HandleTaskCompletedWebhook receives a callback from Convex when a task changes status to completed.
func (h *AuthHandler) HandleTaskCompletedWebhook(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var payload TaskCompletedPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		slog.Error("handlers: failed to parse task completion body", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if payload.UserID == "" || payload.ExternalTaskID == "" {
		http.Error(w, "missing required fields (userId, externalTaskId)", http.StatusBadRequest)
		return
	}

	err := h.syncService.CompleteExternalTask(ctx, payload.UserID, payload.ExternalTaskID)
	if err != nil {
		slog.Error("handlers: failed to push completion status to google tasks", "userId", payload.UserID, "externalTaskId", payload.ExternalTaskID, "error", err)
		http.Error(w, fmt.Sprintf("failed to complete task: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ExecuteTaskPayload defines the payload structure for Cloud Tasks callback executions.
type ExecuteTaskPayload struct {
	UserID    string `json:"userId"`
	TaskID    string `json:"taskId"`
	TaskTitle string `json:"taskTitle"`
}

// HandleTaskExecute processes execution callbacks dispatched by Google Cloud Tasks.
func (h *AuthHandler) HandleTaskExecute(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	audience := os.Getenv("WEBHOOK_AUDIENCE")
	if audience == "" {
		scheme := "https"
		if r.TLS == nil {
			if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
				scheme = proto
			} else {
				scheme = "http"
			}
		}
		audience = fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.Path)
	}

	authHeader := r.Header.Get("Authorization")
	if _, err := validateOIDCToken(ctx, authHeader, audience); err != nil {
		slog.Error("handlers: execute callback OIDC verification failed", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload ExecuteTaskPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		slog.Error("handlers: failed to parse task execution callback payload", "error", err)
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if payload.UserID == "" || payload.TaskID == "" {
		http.Error(w, "missing required fields (userId, taskId)", http.StatusBadRequest)
		return
	}

	slog.Info("handlers: executing scheduled deferred task", "userId", payload.UserID, "taskId", payload.TaskID, "title", payload.TaskTitle)

	h.broker.Broadcast(payload.UserID, sse.Event{
		Type: "MICRO_TASK_DUE",
		Data: map[string]interface{}{
			"taskId":    payload.TaskID,
			"taskTitle": payload.TaskTitle,
		},
	})

	w.WriteHeader(http.StatusOK)
}

// HandleEventsStream establishes and handles persistent SSE streaming connections.
// In production the browser connects directly to go-gateway (bypassing the SvelteKit
// proxy to avoid Vercel serverless timeouts). Auth is via a short-lived signed token
// passed as ?token= query param, generated server-side by SvelteKit. Falls back to
// the session cookie for local development where the cookie is same-origin.
func (h *AuthHandler) HandleEventsStream(w http.ResponseWriter, r *http.Request) {
	var userId string
	var err error

	if token := r.URL.Query().Get("token"); token != "" {
		// Production path: browser uses a short-lived token issued by SvelteKit server.
		userId, err = h.verifySSEToken(token)
	} else {
		// Local dev path: browser cookie is same-origin with the Go server.
		userId, err = h.VerifySession(r)
	}
	if err != nil {
		slog.Warn("handlers: SSE connection rejected", "error", err)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming connection unsupported by the server", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Transfer-Encoding", "chunked")

	eventChan := h.broker.Register(userId)
	defer h.broker.Unregister(userId, eventChan)

	slog.Info("handlers: user client connection registered to events stream", "userId", userId)
	fmt.Fprintf(w, "event: connected\ndata: {}\n\n")
	flusher.Flush()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			slog.Info("handlers: user client connection closed on events stream", "userId", userId)
			return
		case ev := <-eventChan:
			dataBytes, err := json.Marshal(ev.Data)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", ev.Type, string(dataBytes))
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, "event: ping\ndata: {}\n\n")
			flusher.Flush()
		}
	}
}

// EnergyStatePayload represents the payload to update biological score.
type EnergyStatePayload struct {
	Score int `json:"score"`
}

// HandleEnergyState updates the biological score and logs it.
func (h *AuthHandler) HandleEnergyState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := h.verifyRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var payload EnergyStatePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	if payload.Score < 1 || payload.Score > 100 {
		http.Error(w, "energy score must be between 1 and 100", http.StatusBadRequest)
		return
	}

	err = h.convex.CallMutation(ctx, "mutations:updateUserEnergy", map[string]interface{}{
		"userId": userId,
		"score":  float64(payload.Score),
	}, nil)
	if err != nil {
		slog.Error("handlers: failed to update user energy in database", "userId", userId, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// ActionCard represents the action metadata associated with a task in Convex.
type ActionCard struct {
	ActionType   string  `json:"actionType"`
	SavesMinutes float64 `json:"savesMinutes"`
	DraftID      string  `json:"draftId,omitempty"`
	PayloadJSON  string  `json:"payloadJson"`
}

// Task represents the task document stored in Convex.
type Task struct {
	ID              string      `json:"_id"`
	UserID          string      `json:"userId"`
	Title           string      `json:"title"`
	Source          string      `json:"source"`
	Status          string      `json:"status"`
	PriorityScore   float64     `json:"priorityScore"`
	DurationMinutes float64     `json:"durationMinutes"`
	DueAt           float64     `json:"dueAt"`
	ActionCard      *ActionCard `json:"actionCard,omitempty"`
	ExternalTaskID  string      `json:"externalTaskId,omitempty"`
	// Email send retry tracking fields — mirror convex/schema.ts
	SendAttempts int    `json:"sendAttempts,omitempty"`
	LastError    string `json:"lastError,omitempty"`
	ErrorStatus  string `json:"errorStatus,omitempty"`
}

// Schedule represents the schedule allocation document in Convex.
type Schedule struct {
	ID              string  `json:"_id"`
	UserID          string  `json:"userId"`
	TaskID          string  `json:"taskId"`
	StartTime       float64 `json:"startTime"`
	EndTime         float64 `json:"endTime"`
	AllocationType  string  `json:"allocationType"`
	CalendarEventID string  `json:"calendarEventId"`
	Status          string  `json:"status"`
}

// HandleTaskExecuteCard processes execution calls for specific task cards.
func (h *AuthHandler) HandleTaskExecuteCard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := h.verifyRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	taskId := r.PathValue("taskId")
	if taskId == "" {
		// Fallback for older mux if path value is not set
		parts := strings.Split(r.URL.Path, "/")
		if len(parts) >= 4 {
			taskId = parts[3]
		}
	}

	if taskId == "" {
		http.Error(w, "missing task id", http.StatusBadRequest)
		return
	}

	// 1. Fetch task from Convex
	var task Task
	err = h.convex.CallQuery(ctx, "queries:getTask", map[string]interface{}{
		"taskId": taskId,
	}, &task)
	if err != nil {
		slog.Error("handlers: failed to query task details", "taskId", taskId, "error", err)
		http.Error(w, fmt.Sprintf("failed to get task: %v", err), http.StatusInternalServerError)
		return
	}

	if task.ID == "" {
		http.Error(w, "task not found", http.StatusNotFound)
		return
	}

	// 2. Security Check: ensure task belongs to user
	if task.UserID != userId {
		http.Error(w, "forbidden: task belongs to another user", http.StatusForbidden)
		return
	}

	// 3. Execute actions based on ActionType
	if task.ActionCard != nil {
		ts, tokenErr := h.syncService.GetGoogleTokenSource(ctx, userId)
		if tokenErr != nil {
			slog.Error("handlers: failed to get google token source for user", "userId", userId, "error", tokenErr)
			http.Error(w, "unauthorized or missing integrations", http.StatusUnauthorized)
			return
		}

		switch task.ActionCard.ActionType {
		case "GMAIL_DRAFT":
			if task.ActionCard.DraftID == "" {
				http.Error(w, "missing gmail draft id", http.StatusBadRequest)
				return
			}

			// ── Retry Gate ────────────────────────────────────────────────────────────
			// Max 3 send attempts. Once exhausted we permanently stop and surface an
			// error in the dashboard instead of letting the user loop forever.
			const maxSendAttempts = 3

			if task.SendAttempts >= maxSendAttempts {
				slog.Warn("handlers: gmail draft send blocked — max attempts reached",
					"taskId", taskId,
					"attempts", task.SendAttempts,
				)
				http.Error(w,
					fmt.Sprintf("email send permanently failed after %d attempts: %s", maxSendAttempts, task.LastError),
					http.StatusConflict, // 409 — caller should not retry
				)
				return
			}

			// ── Pre-flight: validate recipient email address ───────────────────────
			// Parse the To: field from the AI payload. If the address is malformed
			// (e.g., a hallucinated email) we skip the Gmail API call entirely and
			// mark the task as permanently failed — it will never succeed.
			var draftPayload struct {
				To string `json:"to"`
			}
			_ = json.Unmarshal([]byte(task.ActionCard.PayloadJSON), &draftPayload)

			if draftPayload.To != "" {
				if _, parseErr := mail.ParseAddress(draftPayload.To); parseErr != nil {
					slog.Error("handlers: gmail draft has invalid recipient address — marking permanently failed",
						"taskId", taskId,
						"to", draftPayload.To,
						"error", parseErr,
					)
					errMsg := fmt.Sprintf("invalid recipient email address %q: %v", draftPayload.To, parseErr)
					_ = h.convex.CallMutation(ctx, "mutations:recordTaskError", map[string]interface{}{
						"taskId":       taskId,
						"errorMessage": errMsg,
						"sendAttempts": float64(task.SendAttempts),
						"isFinal":      true,
					}, nil)
					h.broker.Broadcast(userId, sse.Event{
						Type: "TASK_SEND_FAILED",
						Data: map[string]interface{}{
							"taskId":   taskId,
							"title":    task.Title,
							"error":    errMsg,
							"attempts": task.SendAttempts,
						},
					})
					http.Error(w, errMsg, http.StatusUnprocessableEntity)
					return
				}
			}

			// ── Increment attempt counter before calling Gmail ────────────────────
			nextAttempts := task.SendAttempts + 1

			gmailService, gmailErr := gmail.NewService(ctx, option.WithTokenSource(ts))
			if gmailErr != nil {
				slog.Error("handlers: failed to initialize gmail service", "error", gmailErr)
				http.Error(w, "failed to initialize gmail service", http.StatusInternalServerError)
				return
			}

			_, sendErr := gmailService.Users.Drafts.Send("me", &gmail.Draft{Id: task.ActionCard.DraftID}).Context(ctx).Do()
			if sendErr != nil {
				isFinal := nextAttempts >= maxSendAttempts
				errMsg := fmt.Sprintf("attempt %d/%d: %v", nextAttempts, maxSendAttempts, sendErr)

				slog.Error("handlers: failed to send gmail draft",
					"draftId", task.ActionCard.DraftID,
					"attempt", nextAttempts,
					"isFinal", isFinal,
					"error", sendErr,
				)

				// Persist failure details in Convex
				_ = h.convex.CallMutation(ctx, "mutations:recordTaskError", map[string]interface{}{
					"taskId":       taskId,
					"errorMessage": errMsg,
					"sendAttempts": float64(nextAttempts),
					"isFinal":      isFinal,
				}, nil)

				if isFinal {
					// Broadcast a dashboard error event so the UI shows an error badge
					// without requiring a page refresh.
					h.broker.Broadcast(userId, sse.Event{
						Type: "TASK_SEND_FAILED",
						Data: map[string]interface{}{
							"taskId":   taskId,
							"title":    task.Title,
							"error":    fmt.Sprintf("Email permanently failed after %d attempts. Last error: %v", maxSendAttempts, sendErr),
							"attempts": nextAttempts,
						},
					})
					http.Error(w,
						fmt.Sprintf("email send failed after %d attempts and will not be retried: %v", maxSendAttempts, sendErr),
						http.StatusConflict,
					)
				} else {
					http.Error(w,
						fmt.Sprintf("email send failed (attempt %d/%d), please try again: %v", nextAttempts, maxSendAttempts, sendErr),
						http.StatusInternalServerError,
					)
				}
				return
			}
			slog.Info("handlers: gmail draft sent successfully", "taskId", taskId, "draftId", task.ActionCard.DraftID)

		case "CALENDAR_BOOKING":
			// Parse meeting details from the AI-generated payloadJson.
			// Expected fields: title, timeSlot ("3:15 PM - 3:45 PM"), date ("Today"), location,
			// description (meeting agenda/purpose), attendees ([]string of emails)
			var calPayload struct {
				Title       string   `json:"title"`
				TimeSlot    string   `json:"timeSlot"`
				Date        string   `json:"date"`
				Location    string   `json:"location"`
				Description string   `json:"description"`
				Attendees   []string `json:"attendees"`
			}
			if err := json.Unmarshal([]byte(task.ActionCard.PayloadJSON), &calPayload); err != nil {
				slog.Warn("handlers: could not parse calendar booking payload, using defaults", "taskId", taskId, "error", err)
			}

			// Resolve title — fall back to task title if the payload didn't carry one.
			eventTitle := calPayload.Title
			if eventTitle == "" {
				eventTitle = task.Title
			}

			// Resolve location — fall back to editable field sent in request body (if any).
			eventLocation := calPayload.Location
			if eventLocation == "" {
				eventLocation = "Google Meet"
			}

			// Parse the human-readable date + timeSlot into RFC3339 start/end times.
			// The AI emits strings like date="Today" and timeSlot="3:15 PM - 3:45 PM".
			startTime, endTime := parseCalendarSlot(calPayload.Date, calPayload.TimeSlot)

			// Build the Calendar event.
			calService, calErr := calendar.NewService(ctx, option.WithTokenSource(ts))
			if calErr != nil {
				slog.Error("handlers: failed to initialize calendar service for booking", "error", calErr)
				http.Error(w, "failed to initialize calendar service", http.StatusInternalServerError)
				return
			}

			newEvent := &calendar.Event{
				Summary:     eventTitle,
				Location:    eventLocation,
				Description: calPayload.Description,
				Start:       &calendar.EventDateTime{DateTime: startTime.Format(time.RFC3339), TimeZone: "UTC"},
				End:         &calendar.EventDateTime{DateTime: endTime.Format(time.RFC3339), TimeZone: "UTC"},
			}

			// Add attendee emails if provided.
			for _, email := range calPayload.Attendees {
				newEvent.Attendees = append(newEvent.Attendees, &calendar.EventAttendee{Email: email})
			}

			createdEvent, createErr := calService.Events.Insert("primary", newEvent).Context(ctx).Do()
			if createErr != nil {
				slog.Error("handlers: failed to create google calendar event", "taskId", taskId, "error", createErr)
				http.Error(w, fmt.Sprintf("failed to create calendar event: %v", createErr), http.StatusInternalServerError)
				return
			}

			slog.Info("handlers: calendar event created successfully",
				"taskId", taskId,
				"eventId", createdEvent.Id,
				"start", startTime,
				"end", endTime,
			)

			// Persist the created event in Convex so the schedule row exists for future reference.
			var scheduleId string
			err = h.convex.CallMutation(ctx, "mutations:createSchedule", map[string]interface{}{
				"userId":          userId,
				"taskId":          taskId,
				"startTime":       float64(startTime.UnixNano() / int64(time.Millisecond)),
				"endTime":         float64(endTime.UnixNano() / int64(time.Millisecond)),
				"allocationType":  "GHOST_BLOCK",
				"calendarEventId": createdEvent.Id,
				"status":          "COMMITTED",
			}, &scheduleId)
			if err != nil {
				// Non-fatal: the calendar event was already created. Log and continue.
				slog.Warn("handlers: calendar event created but failed to persist schedule in db",
					"taskId", taskId,
					"eventId", createdEvent.Id,
					"error", err,
				)
			}


		case "BILL_PAY":
			slog.Info("handlers: bill payment card executed", "taskId", taskId)
		}
	}

	// 4. Update task status in Convex to COMPLETED
	err = h.convex.CallMutation(ctx, "mutations:updateTaskStatus", map[string]interface{}{
		"taskId": taskId,
		"status": "COMPLETED",
	}, nil)
	if err != nil {
		slog.Error("handlers: failed to mark task as completed", "taskId", taskId, "error", err)
		http.Error(w, fmt.Sprintf("failed to complete task in database: %v", err), http.StatusInternalServerError)
		return
	}

	// If there is an external Google Task, mark it completed too
	if task.Source == "TASKS" && task.ExternalTaskID != "" {
		err = h.syncService.CompleteExternalTask(ctx, userId, task.ExternalTaskID)
		if err != nil {
			slog.Error("handlers: failed to complete external task in google tasks", "externalTaskId", task.ExternalTaskID, "error", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{
		"status": "success",
	})
}

// BiometricLog represents a biometric log record for Go JSON mapping.
type BiometricLog struct {
	ID                  string  `json:"_id"`
	UserID              string  `json:"userId"`
	LogDate             string  `json:"logDate"`
	SleepDurationHours  float64 `json:"sleepDurationHours"`
	RestingHeartRate    float64 `json:"restingHeartRate"`
	StepCount           float64 `json:"stepCount"`
	ComputedEnergyScore float64 `json:"computedEnergyScore"`
}

// HandleGenerateGenome processes requests to compile a weekly retrospective genome report.
func (h *AuthHandler) HandleGenerateGenome(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userId, err := h.verifyRequest(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	slog.Info("handlers: generating weekly genome report", "userId", userId)

	// 1. Fetch past week's stats from Convex
	var weeklyStats struct {
		Tasks         []Task         `json:"tasks"`
		Schedules     []Schedule     `json:"schedules"`
		BiometricLogs []BiometricLog `json:"biometricLogs"`
	}

	err = h.convex.CallQuery(ctx, "queries:getWeeklyStats", map[string]interface{}{
		"userId": userId,
	}, &weeklyStats)
	if err != nil {
		slog.Error("handlers: failed to query weekly stats from database", "userId", userId, "error", err)
		http.Error(w, fmt.Sprintf("database query failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 2. Map stats into gRPC GenerateGenomeRequest payload
	var grpcTasks []*genomev1.HistoricalTask
	for _, t := range weeklyStats.Tasks {
		var savesMins int32 = 0
		if t.ActionCard != nil {
			savesMins = int32(t.ActionCard.SavesMinutes)
		}
		grpcTasks = append(grpcTasks, &genomev1.HistoricalTask{
			TaskId:          t.ID,
			Title:           t.Title,
			Source:          t.Source,
			Status:          t.Status,
			PriorityScore:   int32(t.PriorityScore),
			DurationMinutes: int32(t.DurationMinutes),
			DueAt:           int64(t.DueAt),
			SavesMinutes:    savesMins,
		})
	}

	var grpcSchedules []*genomev1.HistoricalSchedule
	for _, s := range weeklyStats.Schedules {
		grpcSchedules = append(grpcSchedules, &genomev1.HistoricalSchedule{
			ScheduleId:     s.ID,
			TaskId:         s.TaskID,
			StartTime:      int64(s.StartTime),
			EndTime:        int64(s.EndTime),
			AllocationType: s.AllocationType,
			Status:         s.Status,
		})
	}

	var grpcBiometrics []*genomev1.BiometricLog
	for _, b := range weeklyStats.BiometricLogs {
		grpcBiometrics = append(grpcBiometrics, &genomev1.BiometricLog{
			LogDate:            b.LogDate,
			SleepDurationHours: float32(b.SleepDurationHours),
			RestingHeartRate:   int32(b.RestingHeartRate),
			StepCount:          int32(b.StepCount),
			ComputedEnergyScore: int32(b.ComputedEnergyScore),
		})
	}

	req := &genomev1.GenerateGenomeRequest{
		UserId:        userId,
		Tasks:         grpcTasks,
		Schedules:     grpcSchedules,
		BiometricLogs: grpcBiometrics,
	}

	// 3. Invoke gRPC request to Python reasoning service
	genomeResp, err := h.agentClient.GenerateGenome(ctx, req)
	if err != nil {
		slog.Error("handlers: weekly genome gRPC call failed", "userId", userId, "error", err)
		http.Error(w, fmt.Sprintf("AI genome generation failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 4. Save compiled genome report back to Convex
	var insights []map[string]interface{}
	for _, insight := range genomeResp.Insights {
		insights = append(insights, map[string]interface{}{
			"category":    insight.Category,
			"title":       insight.Title,
			"description": insight.Description,
			"impact":      insight.Impact,
		})
	}

	weekStartDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	var newGenomeId string
	err = h.convex.CallMutation(ctx, "mutations:saveGenome", map[string]interface{}{
		"userId":            userId,
		"weekStartDate":     weekStartDate,
		"deadlineRiskScore": float64(genomeResp.DeadlineRiskScore),
		"peakHours":         genomeResp.PeakHours,
		"insights":          insights,
	}, &newGenomeId)
	if err != nil {
		slog.Error("handlers: failed to save weekly genome in database", "userId", userId, "error", err)
		http.Error(w, fmt.Sprintf("database write failed: %v", err), http.StatusInternalServerError)
		return
	}

	// 5. Update user scheduling preferences if returned
	if genomeResp.SchedulingPreferencesJson != "" {
		err = h.convex.CallMutation(ctx, "mutations:updateUserSchedulingPreferences", map[string]interface{}{
			"userId":                userId,
			"schedulingPreferences": genomeResp.SchedulingPreferencesJson,
		}, nil)
		if err != nil {
			slog.Error("handlers: failed to save user scheduling preferences in database", "userId", userId, "error", err)
		}
	}

	// 6. Broadcast SSE event
	h.broker.Broadcast(userId, sse.Event{
		Type: "GENOME_UPDATED",
		Data: map[string]interface{}{
			"userId":            userId,
			"deadlineRiskScore": genomeResp.DeadlineRiskScore,
			"genomeId":          newGenomeId,
		},
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":            "success",
		"genomeId":          newGenomeId,
		"deadlineRiskScore": genomeResp.DeadlineRiskScore,
		"peakHours":         genomeResp.PeakHours,
	})
}

// parseCalendarSlot converts human-readable date and timeSlot strings from the AI
// triage payload into concrete time.Time values suitable for the Google Calendar API.
//
// dateStr examples : "Today", "Tomorrow", "Jun 28", "2026-06-28"
// slotStr examples : "3:15 PM - 3:45 PM", "14:00 - 14:30"
//
// If parsing fails the function returns a best-effort window starting 5 minutes from now.
func parseCalendarSlot(dateStr, slotStr string) (start, end time.Time) {
	now := time.Now().UTC()

	// Resolve the date part.
	var baseDate time.Time
	switch strings.ToLower(strings.TrimSpace(dateStr)) {
	case "today", "":
		baseDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case "tomorrow":
		tomorrow := now.AddDate(0, 0, 1)
		baseDate = time.Date(tomorrow.Year(), tomorrow.Month(), tomorrow.Day(), 0, 0, 0, 0, time.UTC)
	default:
		// Try common formats.
		for _, layout := range []string{"2006-01-02", "Jan 2", "January 2", "Jan 2 2006", "01/02/2006"} {
			candidate := dateStr
			if !strings.Contains(candidate, strconv.Itoa(now.Year())) && !strings.Contains(layout, "2006") {
				candidate = candidate + " " + strconv.Itoa(now.Year())
				layout = layout + " 2006"
			}
			if t, err := time.Parse(layout, candidate); err == nil {
				baseDate = time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
				break
			}
		}
		if baseDate.IsZero() {
			baseDate = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
		}
	}

	// Parse start and end time from slotStr (e.g. "3:15 PM - 3:45 PM").
	parseSlotTime := func(s string) (time.Time, bool) {
		s = strings.TrimSpace(s)
		for _, layout := range []string{"3:04 PM", "15:04", "3:04PM", "15:04:05"} {
			if t, err := time.Parse(layout, s); err == nil {
				return baseDate.Add(time.Duration(t.Hour())*time.Hour + time.Duration(t.Minute())*time.Minute), true
			}
		}
		return time.Time{}, false
	}

	parts := strings.SplitN(slotStr, "-", 2)
	if len(parts) == 2 {
		startParsed, startOK := parseSlotTime(parts[0])
		endParsed, endOK := parseSlotTime(parts[1])
		if startOK && endOK && endParsed.After(startParsed) {
			return startParsed, endParsed
		}
	}

	// Fallback: 5 minutes from now, 30-minute duration.
	fallbackStart := now.Add(5 * time.Minute)
	return fallbackStart, fallbackStart.Add(30 * time.Minute)
}



