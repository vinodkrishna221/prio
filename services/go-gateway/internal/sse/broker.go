package sse

import (
	"sync"
	"log/slog"
)

// Event represents an event pushed to the SvelteKit web client.
type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// Broker manages all active user SSE channels.
type Broker struct {
	mu      sync.RWMutex
	clients map[string]map[chan Event]bool
}

// NewBroker initializes an SSE connection Broker.
func NewBroker() *Broker {
	return &Broker{
		clients: make(map[string]map[chan Event]bool),
	}
}

// Register registers a new SSE event channel for a userId.
func (b *Broker) Register(userId string) chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, 10)
	if _, ok := b.clients[userId]; !ok {
		b.clients[userId] = make(map[chan Event]bool)
	}
	b.clients[userId][ch] = true
	slog.Info("sse/broker: registered new connection client", "userId", userId, "activeConnections", len(b.clients[userId]))
	return ch
}

// Unregister removes a registered channel for a userId and closes it.
func (b *Broker) Unregister(userId string, ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if userClients, ok := b.clients[userId]; ok {
		delete(userClients, ch)
		close(ch)
		slog.Info("sse/broker: unregistered connection client", "userId", userId, "activeConnections", len(userClients))
		if len(userClients) == 0 {
			delete(b.clients, userId)
		}
	}
}

// Broadcast sends an event to all active connections registered under the given userId.
func (b *Broker) Broadcast(userId string, ev Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	userClients, ok := b.clients[userId]
	if !ok {
		slog.Debug("sse/broker: no active connection to broadcast to", "userId", userId, "eventType", ev.Type)
		return
	}

	slog.Info("sse/broker: broadcasting event to user channels", "userId", userId, "eventType", ev.Type, "channelsCount", len(userClients))
	for ch := range userClients {
		select {
		case ch <- ev:
		default:
			// Non-blocking fallback if channel buffer is full
			slog.Warn("sse/broker: slow client channel, skipping event broadcast", "userId", userId)
		}
	}
}
