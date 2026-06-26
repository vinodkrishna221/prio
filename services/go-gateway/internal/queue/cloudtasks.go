package queue

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	cloudtasks "cloud.google.com/go/cloudtasks/apiv2"
	"cloud.google.com/go/cloudtasks/apiv2/cloudtaskspb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TaskDispatcher defines the interface for queueing asynchronous tasks.
type TaskDispatcher interface {
	QueueTaskCallback(ctx context.Context, callbackURL string, payload []byte, scheduleTime time.Time) error
	Close() error
}

// CloudTasksDispatcher implements TaskDispatcher using Google Cloud Tasks apiv2.
type CloudTasksDispatcher struct {
	client    *cloudtasks.Client
	queuePath string
	saEmail   string
}

// NewCloudTasksDispatcher creates a new CloudTasksDispatcher.
func NewCloudTasksDispatcher(ctx context.Context) (*CloudTasksDispatcher, error) {
	queuePath := os.Getenv("CLOUD_TASKS_QUEUE_PATH")
	saEmail := os.Getenv("CLOUD_TASKS_SA_EMAIL")

	if queuePath == "" {
		slog.Warn("queue/cloudtasks: CLOUD_TASKS_QUEUE_PATH is empty; running in MOCK mode")
		return &CloudTasksDispatcher{client: nil}, nil
	}

	client, err := cloudtasks.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("queue/cloudtasks: failed to create Cloud Tasks client: %w", err)
	}

	slog.Info("queue/cloudtasks: initialized Google Cloud Tasks client", "queuePath", queuePath, "saEmail", saEmail)
	return &CloudTasksDispatcher{
		client:    client,
		queuePath: queuePath,
		saEmail:   saEmail,
	}, nil
}

// Close closes the Cloud Tasks client connection.
func (d *CloudTasksDispatcher) Close() error {
	if d.client != nil {
		slog.Info("queue/cloudtasks: closing Google Cloud Tasks client")
		return d.client.Close()
	}
	return nil
}

// QueueTaskCallback queues a callback HTTP request in Google Cloud Tasks.
func (d *CloudTasksDispatcher) QueueTaskCallback(ctx context.Context, callbackURL string, payload []byte, scheduleTime time.Time) error {
	if d.client == nil {
		slog.Info("queue/cloudtasks: MOCK dispatch task",
			"callbackURL", callbackURL,
			"payload", string(payload),
			"scheduleTime", scheduleTime.Format(time.RFC3339),
		)
		return nil
	}

	req := &cloudtaskspb.CreateTaskRequest{
		Parent: d.queuePath,
		Task: &cloudtaskspb.Task{
			MessageType: &cloudtaskspb.Task_HttpRequest{
				HttpRequest: &cloudtaskspb.HttpRequest{
					HttpMethod: cloudtaskspb.HttpMethod_POST,
					Url:        callbackURL,
					Headers:    map[string]string{"Content-Type": "application/json"},
					Body:       payload,
				},
			},
			ScheduleTime: timestamppb.New(scheduleTime),
		},
	}

	if d.saEmail != "" {
		req.Task.GetHttpRequest().AuthorizationHeader = &cloudtaskspb.HttpRequest_OidcToken{
			OidcToken: &cloudtaskspb.OidcToken{
				ServiceAccountEmail: d.saEmail,
				Audience:            callbackURL,
			},
		}
	}

	createdTask, err := d.client.CreateTask(ctx, req)
	if err != nil {
		return fmt.Errorf("queue/cloudtasks: CreateTask failed: %w", err)
	}

	slog.Info("queue/cloudtasks: queued Cloud Task successfully", "taskName", createdTask.Name, "scheduleTime", scheduleTime)
	return nil
}
