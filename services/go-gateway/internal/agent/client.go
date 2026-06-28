package agent

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"

	genomev1 "github.com/lastminutelifesaver/gateway/gen/genome/v1"
	schedulerv1 "github.com/lastminutelifesaver/gateway/gen/scheduler/v1"
	triagev1 "github.com/lastminutelifesaver/gateway/gen/triage/v1"
	"golang.org/x/oauth2"
	"google.golang.org/api/idtoken"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// AgentClient defines the contract for communicating with the Python reasoning agent.
type AgentClient interface {
	ProcessTriage(ctx context.Context, req *triagev1.ProcessTriageRequest) (*triagev1.ProcessTriageResponse, error)
	MatchSchedule(ctx context.Context, req *schedulerv1.MatchScheduleRequest) (*schedulerv1.MatchScheduleResponse, error)
	GenerateGenome(ctx context.Context, req *genomev1.GenerateGenomeRequest) (*genomev1.GenerateGenomeResponse, error)
	Close() error
}

// Client implements the AgentClient interface with a real gRPC connection.
type Client struct {
	conn         *grpc.ClientConn
	triageClient triagev1.TriageServiceClient
	schedClient  schedulerv1.SchedulerServiceClient
	genomeClient genomev1.GenomeServiceClient
	tokenSource  oauth2.TokenSource
}

var (
	instance *Client
	once     sync.Once
	initErr  error
)

// GetClient returns the singleton gRPC AgentClient instance, initializing it if necessary.
func GetClient() (AgentClient, error) {
	once.Do(func() {
		addr := os.Getenv("PYTHON_AGENT_ADDR")
		if addr == "" {
			addr = "localhost:50051"
		}
		slog.Info("initializing gRPC connection to python reasoning agent", "addr", addr)

		// Use TLS for Cloud Run hosts (any non-localhost address).
		// Cloud Run requires TLS; insecure is only safe for local dev.
		var creds grpc.DialOption
		var tokenSource oauth2.TokenSource
		if strings.HasPrefix(addr, "localhost") || strings.HasPrefix(addr, "127.0.0.1") {
			creds = grpc.WithTransportCredentials(insecure.NewCredentials())
		} else {
			creds = grpc.WithTransportCredentials(credentials.NewClientTLSFromCert(nil, ""))

			// Construct OIDC audience: remove port if present, prefix with https://
			host := addr
			if idx := strings.Index(addr, ":"); idx != -1 {
				host = addr[:idx]
			}
			audience := "https://" + host
			ts, err := idtoken.NewTokenSource(context.Background(), audience)
			if err != nil {
				slog.Error("agent/client: failed to create OIDC token source", "audience", audience, "error", err)
			} else {
				tokenSource = ts
				slog.Info("agent/client: configured OIDC token authentication for gRPC", "audience", audience)
			}
		}

		conn, err := grpc.NewClient(addr, creds)
		if err != nil {
			initErr = fmt.Errorf("agent/client: failed to create gRPC connection: %w", err)
			return
		}
		instance = &Client{
			conn:         conn,
			triageClient: triagev1.NewTriageServiceClient(conn),
			schedClient:  schedulerv1.NewSchedulerServiceClient(conn),
			genomeClient: genomev1.NewGenomeServiceClient(conn),
			tokenSource:  tokenSource,
		}
	})
	return instance, initErr
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		slog.Info("closing python agent gRPC client connection")
		return c.conn.Close()
	}
	return nil
}

// ProcessTriage forwards the triage request to the Python agent.
func (c *Client) ProcessTriage(ctx context.Context, req *triagev1.ProcessTriageRequest) (*triagev1.ProcessTriageResponse, error) {
	if c.tokenSource != nil {
		tok, err := c.tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("agent/client: failed to get OIDC token: %w", err)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tok.AccessToken)
	}
	resp, err := c.triageClient.ProcessTriage(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("agent/client: ProcessTriage failed: %w", err)
	}
	return resp, nil
}

// MatchSchedule forwards the schedule matching request to the Python agent.
func (c *Client) MatchSchedule(ctx context.Context, req *schedulerv1.MatchScheduleRequest) (*schedulerv1.MatchScheduleResponse, error) {
	if c.tokenSource != nil {
		tok, err := c.tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("agent/client: failed to get OIDC token: %w", err)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tok.AccessToken)
	}
	resp, err := c.schedClient.MatchSchedule(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("agent/client: MatchSchedule failed: %w", err)
	}
	return resp, nil
}

// GenerateGenome forwards the genome generation request to the Python agent.
func (c *Client) GenerateGenome(ctx context.Context, req *genomev1.GenerateGenomeRequest) (*genomev1.GenerateGenomeResponse, error) {
	if c.tokenSource != nil {
		tok, err := c.tokenSource.Token()
		if err != nil {
			return nil, fmt.Errorf("agent/client: failed to get OIDC token: %w", err)
		}
		ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+tok.AccessToken)
	}
	resp, err := c.genomeClient.GenerateGenome(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("agent/client: GenerateGenome failed: %w", err)
	}
	return resp, nil
}
