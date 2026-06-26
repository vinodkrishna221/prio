package workspace

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/lastminutelifesaver/gateway/internal/cache"
	"github.com/lastminutelifesaver/gateway/internal/oauth"
	"golang.org/x/oauth2"
	"google.golang.org/api/calendar/v3"
	"google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
	"google.golang.org/api/tasks/v1"
)

// Integration represents the OAuth credentials model stored in Convex.
type Integration struct {
	ID                    string `json:"_id"`
	UserID                string `json:"userId"`
	Provider              string `json:"provider"`
	AccessTokenEncrypted  string `json:"accessTokenEncrypted"`
	RefreshTokenEncrypted string `json:"refreshTokenEncrypted"`
	LastSyncCursor        string `json:"lastSyncCursor,omitempty"`
	WatchExpiration       float64 `json:"watchExpiration"`
}

// SyncService orchestrates Google Workspace synchronization tasks.
type SyncService struct {
	oauthConfig *oauth2.Config
	convex      *oauth.ConvexClient
	kms         oauth.KMSService
	cache       cache.CalendarCache
}

// NewSyncService creates a new instance of SyncService.
func NewSyncService(config *oauth2.Config, convex *oauth.ConvexClient, kms oauth.KMSService, cache cache.CalendarCache) *SyncService {
	return &SyncService{
		oauthConfig: config,
		convex:      convex,
		kms:         kms,
		cache:       cache,
	}
}

// GetGoogleTokenSource retrieves the TokenSource for a user, automatically refreshing and re-encrypting the token if needed.
func (s *SyncService) GetGoogleTokenSource(ctx context.Context, userId string) (oauth2.TokenSource, error) {
	var integration Integration
	err := s.convex.CallQuery(ctx, "queries:getIntegration", map[string]interface{}{
		"userId":   userId,
		"provider": "GOOGLE_WORKSPACE",
	}, &integration)
	if err != nil {
		return nil, fmt.Errorf("workspace/sync: failed to query integration: %w", err)
	}
	if integration.ID == "" {
		return nil, fmt.Errorf("workspace/sync: integration not found for user %s", userId)
	}

	accessToken, err := oauth.DecryptToken(ctx, s.kms, integration.AccessTokenEncrypted)
	if err != nil {
		return nil, fmt.Errorf("workspace/sync: failed to decrypt access token: %w", err)
	}

	refreshToken, err := oauth.DecryptToken(ctx, s.kms, integration.RefreshTokenEncrypted)
	if err != nil {
		return nil, fmt.Errorf("workspace/sync: failed to decrypt refresh token: %w", err)
	}

	token := &oauth2.Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       time.Unix(0, 0), // forces TokenSource to check refresh
	}

	tokenSource := s.oauthConfig.TokenSource(ctx, token)
	newToken, err := tokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("workspace/sync: token retrieval failed: %w", err)
	}

	// If token was refreshed, encrypt and save new credentials
	if newToken.AccessToken != accessToken {
		slog.Info("workspace/sync: token refreshed dynamically, persisting new access token", "userId", userId)
		newAccessTokenEncrypted, err := oauth.EncryptToken(ctx, s.kms, newToken.AccessToken)
		if err != nil {
			return nil, fmt.Errorf("workspace/sync: failed to encrypt refreshed access token: %w", err)
		}

		var integrationId string
		err = s.convex.CallMutation(ctx, "mutations:saveIntegration", map[string]interface{}{
			"userId":               userId,
			"provider":             "GOOGLE_WORKSPACE",
			"accessTokenEncrypted": newAccessTokenEncrypted,
			"refreshTokenEncrypted": integration.RefreshTokenEncrypted,
			"watchExpiration":      integration.WatchExpiration,
		}, &integrationId)
		if err != nil {
			slog.Error("workspace/sync: failed to update integration credentials in convex", "error", err)
		}
	}

	return tokenSource, nil
}

// GetFreeBusy retrieves Google Calendar freeBusy slots for the next 7 days, checking Redis cache first.
func (s *SyncService) GetFreeBusy(ctx context.Context, userId string) ([]cache.FreeBusySlot, error) {
	cached, err := s.cache.GetFreeBusyCache(ctx, userId)
	if err == nil && cached != nil {
		slog.Debug("workspace/sync: free-busy cache hit", "userId", userId)
		return cached, nil
	}

	slog.Info("workspace/sync: free-busy cache miss, querying google calendar API", "userId", userId)

	ts, err := s.GetGoogleTokenSource(ctx, userId)
	if err != nil {
		return nil, err
	}

	calService, err := calendar.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return nil, fmt.Errorf("workspace/sync: failed to create calendar service: %w", err)
	}

	now := time.Now()
	endTime := now.AddDate(0, 0, 7)

	req := &calendar.FreeBusyRequest{
		TimeMin: now.Format(time.RFC3339),
		TimeMax: endTime.Format(time.RFC3339),
		Items:   []*calendar.FreeBusyRequestItem{{Id: "primary"}},
	}

	resp, err := calService.Freebusy.Query(req).Context(ctx).Do()
	if err != nil {
		return nil, fmt.Errorf("workspace/sync: google calendar freeBusy query failed: %w", err)
	}

	var slots []cache.FreeBusySlot
	if primary, ok := resp.Calendars["primary"]; ok {
		for _, busy := range primary.Busy {
			start, err := time.Parse(time.RFC3339, busy.Start)
			if err != nil {
				continue
			}
			end, err := time.Parse(time.RFC3339, busy.End)
			if err != nil {
				continue
			}
			slots = append(slots, cache.FreeBusySlot{
				Start: start.UnixNano() / int64(time.Millisecond),
				End:   end.UnixNano() / int64(time.Millisecond),
			})
		}
	}

	if err := s.cache.SetFreeBusyCache(ctx, userId, slots, 300*time.Second); err != nil {
		slog.Error("workspace/sync: failed to update redis freebusy cache", "error", err)
	}

	return slots, nil
}

// CreateGmailDraft creates a reply draft on a specific thread in Gmail.
func (s *SyncService) CreateGmailDraft(ctx context.Context, userId string, threadId string, messageId string, toEmail string, subject string, body string) (string, error) {
	ts, err := s.GetGoogleTokenSource(ctx, userId)
	if err != nil {
		return "", err
	}

	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return "", fmt.Errorf("workspace/sync: failed to create gmail service: %w", err)
	}

	// Format a raw email message adhering to RFC 2822
	// For replies, reference In-Reply-To and References headers
	var headers []string
	headers = append(headers, fmt.Sprintf("To: %s", toEmail))
	headers = append(headers, fmt.Sprintf("Subject: %s", subject))
	if messageId != "" {
		headers = append(headers, fmt.Sprintf("In-Reply-To: %s", messageId))
		headers = append(headers, fmt.Sprintf("References: %s", messageId))
	}
	headers = append(headers, "Content-Type: text/plain; charset=UTF-8")

	rawMsg := strings.Join(headers, "\r\n") + "\r\n\r\n" + body

	encodedMsg := base64.URLEncoding.EncodeToString([]byte(rawMsg))
	// Replace Standard URL safe mappings
	encodedMsg = strings.ReplaceAll(encodedMsg, "/", "_")
	encodedMsg = strings.ReplaceAll(encodedMsg, "+", "-")

	draft := &gmail.Draft{
		Message: &gmail.Message{
			Raw:      encodedMsg,
			ThreadId: threadId,
		},
	}

	createdDraft, err := gmailService.Users.Drafts.Create("me", draft).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("workspace/sync: google drafts create failed: %w", err)
	}

	return createdDraft.Id, nil
}

// SyncTasks pulls updated tasks from Google Tasks API and syncs them to Convex.
func (s *SyncService) SyncTasks(ctx context.Context, userId string) error {
	var integration Integration
	err := s.convex.CallQuery(ctx, "queries:getIntegration", map[string]interface{}{
		"userId":   userId,
		"provider": "GOOGLE_WORKSPACE",
	}, &integration)
	if err != nil {
		return fmt.Errorf("workspace/sync: failed to query integration: %w", err)
	}

	ts, err := s.GetGoogleTokenSource(ctx, userId)
	if err != nil {
		return err
	}

	tasksService, err := tasks.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("workspace/sync: failed to create tasks service: %w", err)
	}

	nowStr := time.Now().Format(time.RFC3339)
	listCall := tasksService.Tasks.List("@default")
	if integration.LastSyncCursor != "" {
		listCall.UpdatedMin(integration.LastSyncCursor)
	}

	taskList, err := listCall.Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("workspace/sync: google tasks list failed: %w", err)
	}

	for _, item := range taskList.Items {
		var status string
		if item.Status == "completed" {
			status = "COMPLETED"
		} else {
			status = "ACTIVE"
		}

		dueAt := time.Now().Add(24 * time.Hour).UnixNano() / int64(time.Millisecond)
		if item.Due != "" {
			if parsedDue, err := time.Parse(time.RFC3339, item.Due); err == nil {
				dueAt = parsedDue.UnixNano() / int64(time.Millisecond)
			}
		}

		// Check if external task already exists in Convex
		var existingTask struct {
			ID string `json:"_id"`
		}
		err = s.convex.CallQuery(ctx, "queries:getTaskByExternalId", map[string]interface{}{
			"userId":         userId,
			"externalTaskId": item.Id,
		}, &existingTask)

		if err == nil && existingTask.ID != "" {
			err = s.convex.CallMutation(ctx, "mutations:updateTaskStatus", map[string]interface{}{
				"taskId": existingTask.ID,
				"status": status,
			}, nil)
			if err != nil {
				slog.Error("workspace/sync: failed to update status of synced task", "taskId", existingTask.ID, "error", err)
			}
		} else {
			var newTaskId string
			err = s.convex.CallMutation(ctx, "mutations:createSyncTask", map[string]interface{}{
				"userId":         userId,
				"title":          item.Title,
				"status":         status,
				"externalTaskId": item.Id,
				"dueAt":          dueAt,
			}, &newTaskId)
			if err != nil {
				slog.Error("workspace/sync: failed to create sync task in convex", "error", err)
			}
		}
	}

	err = s.convex.CallMutation(ctx, "mutations:updateLastSyncCursor", map[string]interface{}{
		"userId":         userId,
		"provider":       "GOOGLE_WORKSPACE",
		"lastSyncCursor": nowStr,
	}, nil)
	if err != nil {
		slog.Error("workspace/sync: failed to update integration lastSyncCursor", "error", err)
	}

	return nil
}

// CompleteExternalTask marks a task completed in the Google Tasks API.
func (s *SyncService) CompleteExternalTask(ctx context.Context, userId string, externalTaskId string) error {
	ts, err := s.GetGoogleTokenSource(ctx, userId)
	if err != nil {
		return err
	}

	tasksService, err := tasks.NewService(ctx, option.WithTokenSource(ts))
	if err != nil {
		return fmt.Errorf("workspace/sync: failed to create tasks service: %w", err)
	}

	item, err := tasksService.Tasks.Get("@default", externalTaskId).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("workspace/sync: failed to retrieve external task: %w", err)
	}

	if item.Status == "completed" {
		return nil
	}

	item.Status = "completed"
	_, err = tasksService.Tasks.Update("@default", externalTaskId, item).Context(ctx).Do()
	if err != nil {
		return fmt.Errorf("workspace/sync: failed to update status in google tasks API: %w", err)
	}

	slog.Info("workspace/sync: external task completed successfully in google tasks", "externalTaskId", externalTaskId)
	return nil
}
