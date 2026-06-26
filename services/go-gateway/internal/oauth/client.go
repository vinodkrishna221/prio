package oauth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ConvexClient represents the server-to-server client for Convex.
type ConvexClient struct {
	url       string
	deployKey string
	client    *http.Client
}

// ConvexRequest is the standard JSON request body format for Convex HTTP API.
type ConvexRequest struct {
	Path   string      `json:"path"`
	Args   interface{} `json:"args"`
	Format string      `json:"format"`
}

// ConvexResponse is the standard JSON response body format from Convex HTTP API.
type ConvexResponse struct {
	Status string          `json:"status"` // "success" or "error"
	Value  json.RawMessage `json:"value,omitempty"`
	Error  string          `json:"errorMessage,omitempty"`
}

// ConvexUser maps the user schema in Convex.
type ConvexUser struct {
	ID                 string  `json:"_id"`
	Email              string  `json:"email"`
	CreatedAt          float64 `json:"createdAt"`
	CurrentEnergyScore float64 `json:"currentEnergyScore"`
	EnergyLastUpdated  float64 `json:"energyLastUpdated"`
}

// NewConvexClient instantiates a Convex HTTP Client.
func NewConvexClient(url, deployKey string) *ConvexClient {
	return &ConvexClient{
		url:       url,
		deployKey: deployKey,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CallQuery invokes a query on Convex.
func (c *ConvexClient) CallQuery(ctx context.Context, path string, args interface{}, result interface{}) error {
	return c.callAPI(ctx, "query", path, args, result)
}

// CallMutation invokes a mutation on Convex.
func (c *ConvexClient) CallMutation(ctx context.Context, path string, args interface{}, result interface{}) error {
	return c.callAPI(ctx, "mutation", path, args, result)
}

func (c *ConvexClient) callAPI(ctx context.Context, apiType string, path string, args interface{}, result interface{}) error {
	if c.url == "" {
		return fmt.Errorf("convex/client: convex URL is empty")
	}
	reqURL := fmt.Sprintf("%s/api/%s", c.url, apiType)

	reqBody := ConvexRequest{
		Path:   path,
		Args:   args,
		Format: "json",
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("convex/client: failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(reqBytes))
	if err != nil {
		return fmt.Errorf("convex/client: failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.deployKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Convex %s", c.deployKey))
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("convex/client: http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("convex/client: request returned status %d", resp.StatusCode)
	}

	var convexResp ConvexResponse
	if err := json.NewDecoder(resp.Body).Decode(&convexResp); err != nil {
		return fmt.Errorf("convex/client: failed to decode response: %w", err)
	}

	if convexResp.Status == "error" {
		return fmt.Errorf("convex/client: error executing %s: %s", path, convexResp.Error)
	}

	if result != nil && len(convexResp.Value) > 0 {
		if err := json.Unmarshal(convexResp.Value, result); err != nil {
			return fmt.Errorf("convex/client: failed to unmarshal result: %w", err)
		}
	}

	return nil
}

// SetHTTPClient overrides the default http.Client. Used primarily for testing.
func (c *ConvexClient) SetHTTPClient(client *http.Client) {
	c.client = client
}

