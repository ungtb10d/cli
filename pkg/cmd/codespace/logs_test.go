package codespace

import (
	"context"
	"testing"

	"github.com/ungtb10d/cli/v2/internal/codespaces/api"
	"github.com/ungtb10d/cli/v2/pkg/iostreams"
)

func TestPendingOperationDisallowsLogs(t *testing.T) {
	app := testingLogsApp()

	if err := app.Logs(context.Background(), "disabledCodespace", false); err != nil {
		if err.Error() != "codespace is disabled while it has a pending operation: Some pending operation" {
			t.Errorf("expected pending operation error, but got: %v", err)
		}
	} else {
		t.Error("expected pending operation error, but got nothing")
	}
}

func testingLogsApp() *App {
	disabledCodespace := &api.Codespace{
		Name:                           "disabledCodespace",
		PendingOperation:               true,
		PendingOperationDisabledReason: "Some pending operation",
	}
	apiMock := &apiClientMock{
		GetCodespaceFunc: func(_ context.Context, name string, _ bool) (*api.Codespace, error) {
			if name == "disabledCodespace" {
				return disabledCodespace, nil
			}
			return nil, nil
		},
	}

	ios, _, _, _ := iostreams.Test()
	return NewApp(ios, nil, apiMock, nil)
}
