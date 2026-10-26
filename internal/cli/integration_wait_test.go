package cli_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/port-labs/terraform-provider-port-labs/v2/internal/adopttest"
	"github.com/port-labs/terraform-provider-port-labs/v2/internal/cli"
	"github.com/stretchr/testify/assert"
)

func integrationWithStatus(status, message string) map[string]any {
	return map[string]any{
		"installationId": "my-gitlab",
		"statusInfo":     map[string]any{"integrationStatus": map[string]any{"status": status, "message": message}},
	}
}

func TestWaitForIntegrationDeletedFailsFastWhenDeleteFailed(t *testing.T) {
	fake := adopttest.NewFakePort(t)
	fake.AddIntegration("my-gitlab", integrationWithStatus("Error", "Fail to delete Integration"))

	start := time.Now()
	err := fake.Client(nil).WaitForIntegrationDeleted(context.Background(), "my-gitlab")

	var failedErr *cli.IntegrationOperationFailedError
	if assert.True(t, errors.As(err, &failedErr), "got %v", err) {
		assert.Equal(t, "delete", failedErr.Operation)
		assert.Equal(t, "Error", failedErr.Status)
		assert.Equal(t, "Fail to delete Integration", failedErr.Message)
	}
	assert.Less(t, time.Since(start), time.Second, "a failed delete must not be polled")
}

func TestWaitForIntegrationDeletedReturnsWhenGone(t *testing.T) {
	fake := adopttest.NewFakePort(t)

	start := time.Now()
	err := fake.Client(nil).WaitForIntegrationDeleted(context.Background(), "my-gitlab")

	assert.NoError(t, err)
	assert.Less(t, time.Since(start), time.Second)
}

func TestWaitForIntegrationDeletedKeepsWaitingWhileDeleting(t *testing.T) {
	fake := adopttest.NewFakePort(t)
	fake.AddIntegration("my-gitlab", integrationWithStatus("Deleting", ""))

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := fake.Client(nil).WaitForIntegrationDeleted(ctx, "my-gitlab")

	var failedErr *cli.IntegrationOperationFailedError
	assert.False(t, errors.As(err, &failedErr), "a delete in progress is not a failure")
	assert.Error(t, err, "still deleting when the context ends")
}

func TestWaitForIntegrationReadyFailsFastOnError(t *testing.T) {
	fake := adopttest.NewFakePort(t)
	fake.AddIntegration("my-gitlab", integrationWithStatus("Error", "Fail to create Integration"))

	_, err := fake.Client(nil).WaitForIntegrationReady(context.Background(), "my-gitlab")

	var failedErr *cli.IntegrationOperationFailedError
	if assert.True(t, errors.As(err, &failedErr), "got %v", err) {
		assert.Equal(t, "Fail to create Integration", failedErr.Message)
	}
}
