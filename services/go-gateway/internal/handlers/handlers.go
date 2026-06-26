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
	"os"
	"strconv"
	"strings"
	"time"

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
	oauthConfig  *oauth2.Config
	convex       *oauth.ConvexClient
	kms          oauth.KMSService
	cookieKey    []byte
	dashboardURL string

	agentClient  agent.AgentClient
	cache        cache.CalendarCache
	broker       *sse.Broker
	dispatcher   queue.TaskDispatcher
	syncService  *workspace.SyncService
}

// NewAuthHandler creates an instance of AuthHandler.
func NewAuthHandler(
	clientID, clientSecret, redirectURL, dashboardURL, convexURL, convexKey string,
	kms oauth.KMSService,
	cookieKey []byte,
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
		oauthConfig:  config,
		convex:       oauth.NewConvexClient(convexURL, convexKey),
		kms:          kms,
		cookieKey:    cookieKey,
		dashboardURL: dashboardURL,
		agentClient:  agentClient,
		cache:        cache,
		broker:       broker,
		dispatcher:   dispatcher,
		syncService:  syncService,
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

	h.setSessionCookie(w, userId)

	slog.Info("oauth authentication complete, redirecting to web client", "userId", userId)
	http.Redirect(w, r, h.dashboardURL, http.StatusTemporaryRedirect)
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

func (h *AuthHandler) setSessionCookie(w http.ResponseWriter, userId string) {
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
		slog.Error("handlers: user not found for email", "email", watchPayload.EmailAddress)
		w.WriteHeader(http.StatusNoContent)
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

	msgList, err := gmailService.Users.Messages.List("me").MaxResults(1).Context(ctx).Do()
	if err != nil || len(msgList.Messages) == 0 {
		slog.Error("handlers: failed to list user messages", "error", err)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	msgID := msgList.Messages[0].Id
	threadID := msgList.Messages[0].ThreadId

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
	userId, err := h.VerifySession(r)
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
	userId, err := h.VerifySession(r)
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
func (h *AuthHandler) HandleEventsStream(w http.ResponseWriter, r *http.Request) {
	userId, err := h.VerifySession(r)
	if err != nil {
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
	userId, err := h.VerifySession(r)
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
	userId, err := h.VerifySession(r)
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

			gmailService, gmailErr := gmail.NewService(ctx, option.WithTokenSource(ts))
			if gmailErr != nil {
				slog.Error("handlers: failed to initialize gmail service", "error", gmailErr)
				http.Error(w, "failed to initialize gmail service", http.StatusInternalServerError)
				return
			}

			_, sendErr := gmailService.Users.Drafts.Send("me", &gmail.Draft{Id: task.ActionCard.DraftID}).Context(ctx).Do()
			if sendErr != nil {
				slog.Error("handlers: failed to send gmail draft", "draftId", task.ActionCard.DraftID, "error", sendErr)
				http.Error(w, fmt.Sprintf("failed to send gmail draft: %v", sendErr), http.StatusInternalServerError)
				return
			}
			slog.Info("handlers: gmail draft sent successfully", "taskId", taskId, "draftId", task.ActionCard.DraftID)

		case "CALENDAR_BOOKING":
			// Find active schedules for this task
			var schedules []Schedule
			err = h.convex.CallQuery(ctx, "queries:getActiveSchedules", map[string]interface{}{
				"userId": userId,
			}, &schedules)
			if err != nil {
				slog.Error("handlers: failed to retrieve schedules", "userId", userId, "error", err)
				http.Error(w, "failed to retrieve active schedules", http.StatusInternalServerError)
				return
			}

			var targetSchedule *Schedule
			for i := range schedules {
				if schedules[i].TaskID == taskId {
					targetSchedule = &schedules[i]
					break
				}
			}

			if targetSchedule == nil {
				slog.Warn("handlers: no reserved schedule block found for task", "taskId", taskId)
			} else {
				calService, calErr := calendar.NewService(ctx, option.WithTokenSource(ts))
				if calErr != nil {
					slog.Error("handlers: failed to initialize calendar service", "error", calErr)
					http.Error(w, "failed to initialize calendar service", http.StatusInternalServerError)
					return
				}

				_, patchErr := calService.Events.Patch("primary", targetSchedule.CalendarEventID, &calendar.Event{
					Status: "confirmed",
				}).Context(ctx).Do()
				if patchErr != nil {
					slog.Error("handlers: failed to confirm event in google calendar", "eventId", targetSchedule.CalendarEventID, "error", patchErr)
					http.Error(w, fmt.Sprintf("failed to confirm calendar event: %v", patchErr), http.StatusInternalServerError)
					return
				}

				// Update schedule status to COMMITTED in Convex
				err = h.convex.CallMutation(ctx, "mutations:updateScheduleStatus", map[string]interface{}{
					"scheduleId": targetSchedule.ID,
					"status":     "COMMITTED",
				}, nil)
				if err != nil {
					slog.Error("handlers: failed to update schedule status in database", "scheduleId", targetSchedule.ID, "error", err)
				}
				slog.Info("handlers: calendar booking confirmed successfully", "taskId", taskId, "eventId", targetSchedule.CalendarEventID)
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


