package add

import (
	"context"
	"errors"
	"testing"

	commandruntime "github.com/devctllabs/devctl/cmd/devctl/internal/command"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestFinishManifestAdditionReportsJoinedFailuresWithKnownDataOnly(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		result        projectdomain.ManifestResult
		operationErr  error
		shutdownErr   error
		expectPartial bool
	}{
		{
			name:         "operation and shutdown with known manifest",
			result:       projectdomain.ManifestResult{Manifest: "/project/devctl.yaml", Change: projectdomain.ChangeUpdated},
			operationErr: errors.New("operation failed"), shutdownErr: errors.New("shutdown failed"),
			expectPartial: true,
		},
		{
			name:          "operation with unknown manifest",
			operationErr:  errors.New("operation failed"),
			expectPartial: false,
		},
		{
			name:          "shutdown with unknown manifest",
			shutdownErr:   errors.New("shutdown failed"),
			expectPartial: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			core, observed := observer.New(zap.DebugLevel)
			reporter := commandruntime.NewErrorReporter(zap.New(core), false)
			err := finishManifestAddition(context.Background(), manifestAddition{
				stdout: zap.NewNop(), reporter: reporter, result: test.result,
				operationErr: test.operationErr,
				shutdown:     func(context.Context) error { return test.shutdownErr },
			})

			if test.operationErr != nil {
				require.ErrorIs(t, err, test.operationErr)
			}
			if test.shutdownErr != nil {
				require.ErrorIs(t, err, test.shutdownErr)
			}
			require.Len(t, observed.All(), 1)
			context := observed.All()[0].ContextMap()
			require.NotContains(t, context, "data")
			details, hasDetails := context["details"].(map[string]any)
			_, hasPartial := details["partial_result"]
			require.Equal(t, test.expectPartial, hasDetails && hasPartial)
		})
	}
}
