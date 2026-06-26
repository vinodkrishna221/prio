package sse

import (
	"sync"
	"testing"
	"time"
)

func TestBrokerRegisterUnregister(t *testing.T) {
	broker := NewBroker()
	userId := "user-123"

	ch := broker.Register(userId)
	if ch == nil {
		t.Fatal("expected channel to be registered, got nil")
	}

	broker.Unregister(userId, ch)

	// Verify that broadcasting after unregister does not panic/block
	broker.Broadcast(userId, Event{Type: "TEST", Data: "hello"})
}

func TestBrokerBroadcast(t *testing.T) {
	broker := NewBroker()
	userId := "user-456"

	ch1 := broker.Register(userId)
	ch2 := broker.Register(userId)

	event := Event{
		Type: "TASK_TRIAGED",
		Data: "some-data",
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		select {
		case ev := <-ch1:
			if ev.Type != "TASK_TRIAGED" {
				t.Errorf("expected TASK_TRIAGED, got %s", ev.Type)
			}
		case <-time.After(500 * time.Millisecond):
			t.Error("timeout waiting for broadcast on ch1")
		}
	}()

	go func() {
		defer wg.Done()
		select {
		case ev := <-ch2:
			if ev.Type != "TASK_TRIAGED" {
				t.Errorf("expected TASK_TRIAGED, got %s", ev.Type)
			}
		case <-time.After(500 * time.Millisecond):
			t.Error("timeout waiting for broadcast on ch2")
		}
	}()

	// Short pause to allow goroutines to run
	time.Sleep(10 * time.Millisecond)
	broker.Broadcast(userId, event)

	wg.Wait()
}
