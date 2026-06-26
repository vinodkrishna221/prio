package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
)

// FreeBusySlot represents a busy start/end epoch millisecond interval.
type FreeBusySlot struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// CalendarCache defines the caching operations for user free-busy slots.
type CalendarCache interface {
	GetFreeBusyCache(ctx context.Context, userId string) ([]FreeBusySlot, error)
	SetFreeBusyCache(ctx context.Context, userId string, slots []FreeBusySlot, ttl time.Duration) error
	Close() error
}

// CacheManager implements CalendarCache using go-redis/v9.
type CacheManager struct {
	client *redis.Client
}

// NewCacheManager initializes a new Redis CacheManager client.
func NewCacheManager() (*CacheManager, error) {
	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}
	redisURL := os.Getenv("REDIS_URL")
	var opt *redis.Options
	var err error

	if redisURL != "" {
		opt, err = redis.ParseURL(redisURL)
		if err != nil {
			return nil, fmt.Errorf("cache/redis: failed to parse REDIS_URL: %w", err)
		}
	} else {
		opt = &redis.Options{
			Addr: redisAddr,
		}
	}

	slog.Info("initializing redis connection", "addr", opt.Addr)
	client := redis.NewClient(opt)

	// Validate connectivity with a short ping timeout
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("cache/redis: check connection failed (ignoring for offline/mock flow)", "error", err)
	}

	return &CacheManager{client: client}, nil
}

// Close closes the underlying Redis client.
func (cm *CacheManager) Close() error {
	if cm.client != nil {
		slog.Info("closing redis client connection")
		return cm.client.Close()
	}
	return nil
}

// GetFreeBusyCache retrieves the user's cached busy slots from Redis.
func (cm *CacheManager) GetFreeBusyCache(ctx context.Context, userId string) ([]FreeBusySlot, error) {
	key := fmt.Sprintf("user:%s:freebusy_cache", userId)
	val, err := cm.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, nil // Cache miss
	} else if err != nil {
		return nil, fmt.Errorf("cache/redis: get failed for key %s: %w", key, err)
	}

	var slots []FreeBusySlot
	if err := json.Unmarshal([]byte(val), &slots); err != nil {
		return nil, fmt.Errorf("cache/redis: failed to deserialize slots: %w", err)
	}

	return slots, nil
}

// SetFreeBusyCache stores the user's busy slots in Redis with a TTL.
func (cm *CacheManager) SetFreeBusyCache(ctx context.Context, userId string, slots []FreeBusySlot, ttl time.Duration) error {
	key := fmt.Sprintf("user:%s:freebusy_cache", userId)
	data, err := json.Marshal(slots)
	if err != nil {
		return fmt.Errorf("cache/redis: failed to serialize slots: %w", err)
	}

	if err := cm.client.Set(ctx, key, data, ttl).Err(); err != nil {
		return fmt.Errorf("cache/redis: set failed for key %s: %w", key, err)
	}

	return nil
}
