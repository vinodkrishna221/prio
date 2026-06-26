package handlers

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lastminutelifesaver/gateway/internal/oauth"
	"golang.org/x/oauth2"
)

type mockRoundTripper struct {
	roundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.roundTripFunc(req)
}

func TestHandleGoogleLogin(t *testing.T) {
	cookieKey := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, cookieKey)
	mockKMS := oauth.NewMockKMSService("passphrase")

	handler := NewAuthHandler(
		"client-id",
		"client-secret",
		"http://localhost:8080/auth/callback",
		"http://localhost:5173/",
		"http://localhost:3000",
		"deploy-key",
		mockKMS,
		cookieKey,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	req := httptest.NewRequest(http.MethodGet, "/auth/google", nil)
	w := httptest.NewRecorder()

	handler.HandleGoogleLogin(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if !strings.Contains(location, "accounts.google.com") {
		t.Errorf("expected redirect to google accounts, got %s", location)
	}
}

func TestHandleGoogleCallback_Success(t *testing.T) {
	cookieKey := make([]byte, 32)
	_, _ = io.ReadFull(rand.Reader, cookieKey)
	mockKMS := oauth.NewMockKMSService("passphrase")

	convexURL := "http://convex.local"
	handler := NewAuthHandler(
		"client-id",
		"client-secret",
		"http://localhost:8080/auth/callback",
		"http://localhost:5173/dashboard",
		convexURL,
		"deploy-key",
		mockKMS,
		cookieKey,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	// Mock HTTP interactions
	mockTransport := &mockRoundTripper{
		roundTripFunc: func(req *http.Request) (*http.Response, error) {
			urlStr := req.URL.String()

			// 1. Google OAuth2 token exchange
			if strings.Contains(urlStr, "oauth2.googleapis.com/token") || strings.Contains(urlStr, "accounts.google.com/o/oauth2/token") {
				tokenJSON := `{"access_token": "mock-access-token", "refresh_token": "mock-refresh-token", "token_type": "Bearer", "expires_in": 3600}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(tokenJSON)),
				}, nil
			}

			// 2. Google UserInfo email fetch
			if strings.Contains(urlStr, "googleapis.com/oauth2/v2/userinfo") {
				userInfo := `{"email": "user@example.com"}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(userInfo)),
				}, nil
			}

			// 3. Convex Query - getUserByEmail
			if strings.Contains(urlStr, "/api/query") {
				// Mock user not found: return status "success" with null value
				respJSON := `{"status": "success", "value": null}`
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(respJSON)),
				}, nil
			}

			// 4. Convex Mutation - createUser and saveIntegration
			if strings.Contains(urlStr, "/api/mutation") {
				var reqBody oauth.ConvexRequest
				_ = json.NewDecoder(req.Body).Decode(&reqBody)

				var respJSON string
				switch reqBody.Path {
				case "mutations:createUser":
					respJSON = `{"status": "success", "value": "user_id_123"}`
				case "mutations:saveIntegration":
					respJSON = `{"status": "success", "value": "integration_id_456"}`
				default:
					respJSON = `{"status": "error", "errorMessage": "unknown mutation"}`
				}

				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(bytes.NewBufferString(respJSON)),
				}, nil
			}

			return &http.Response{
				StatusCode: http.StatusNotFound,
				Body:       io.NopCloser(bytes.NewBufferString(`{"error": "not found"}`)),
			}, nil
		},
	}
	// Create test client that uses mock transport
	testClient := &http.Client{Transport: mockTransport}

	// Override standard library HTTP client for oauth2 and h.convex client
	handler.convex.SetHTTPClient(testClient)

	// Set up request
	req := httptest.NewRequest(http.MethodGet, "/auth/callback?code=mockcode123", nil)
	ctx := context.WithValue(req.Context(), oauth2.HTTPClient, testClient)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	handler.HandleGoogleCallback(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusTemporaryRedirect {
		t.Errorf("expected status 307 redirect, got %d", resp.StatusCode)
	}

	location := resp.Header.Get("Location")
	if location != "http://localhost:5173/dashboard" {
		t.Errorf("expected redirect to dashboard URL, got %s", location)
	}

	// Verify session cookie is set
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "session_id" {
			sessionCookie = c
			break
		}
	}

	if sessionCookie == nil {
		t.Fatal("expected session_id cookie to be set, but got none")
	}

	if !sessionCookie.HttpOnly {
		t.Error("expected session cookie to be HttpOnly")
	}

	// Verify cookie signature using the handler's verification method
	reqWithCookie := httptest.NewRequest(http.MethodGet, "/any-endpoint", nil)
	reqWithCookie.AddCookie(sessionCookie)

	userId, err := handler.VerifySession(reqWithCookie)
	if err != nil {
		t.Fatalf("session verification failed: %v", err)
	}

	if userId != "user_id_123" {
		t.Errorf("expected verified userId to be %q, got %q", "user_id_123", userId)
	}
}

