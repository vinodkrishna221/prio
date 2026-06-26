package queue

import (
	"context"
	"testing"
	"time"
)

func TestMockCloudTasksDispatcher(t *testing.T) {
	ctx := context.Background()
	dispatcher, err := NewCloudTasksDispatcher(ctx)
	if err != nil {
		t.Fatalf("failed to initialize dispatcher: %v", err)
	}

	if dispatcher.client != nil {
		t.Error("expected client to be nil in mock mode")
	}

	err = dispatcher.QueueTaskCallback(ctx, "http://localhost:8080/tasks/execute", []byte(`{"userId": "123"}`), time.Now())
	if err != nil {
		t.Errorf("expected no error in mock mode, got %v", err)
	}

	err = dispatcher.Close()
	if err != nil {
		t.Errorf("expected no error closing mock dispatcher, got %v", err)
	}
}
