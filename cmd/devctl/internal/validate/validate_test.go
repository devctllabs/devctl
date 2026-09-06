package validate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"testing"

	"github.com/devctllabs/devctl/cmd/devctl/internal/validate/mocks"
	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v3"
	"go.uber.org/mock/gomock"
	"go.uber.org/zap"
)

func TestValidateWritesCommandSpecificJSON(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	validator := mocks.NewMockprojectValidator(ctrl)
	validator.EXPECT().Validate(gomock.Any(), projectdomain.ValidateQuery{ManifestPath: "custom.yaml"}).Return(projectdomain.ValidationResult{Issues: []projectdomain.Issue{}}, nil)
	built := false
	command := newValidateCmd(validateCmdOpts{}, func(_ context.Context, logger *zap.Logger) (validateRuntime, error) {
		built = true
		require.NotNil(t, logger)
		return validateRuntime{validator: validator, shutdown: func(context.Context) error { return nil }}, nil
	})
	var stdout bytes.Buffer
	command.Writer = &stdout

	err := command.Run(context.Background(), []string{"validate", "--file", "custom.yaml", "--json"})

	require.NoError(t, err)
	require.True(t, built)
	require.Contains(t, stdout.String(), `"msg":"project validation completed"`)
	require.Contains(t, stdout.String(), `"command":"validate"`)
	require.Contains(t, stdout.String(), `"data":{"valid":true,"issues":[]}`)
}

func TestValidateReturnsSafeExecutionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	validator := mocks.NewMockprojectValidator(ctrl)
	validator.EXPECT().Validate(gomock.Any(), projectdomain.ValidateQuery{}).Return(
		projectdomain.ValidationResult{}, fmt.Errorf("repository.LoadDocument: %w", &projectdomain.OperationError{
			Operation: projectdomain.OperationLoadManifest,
			Path:      "/private/work/devctl.yaml",
			Kind:      projectdomain.FailureNotFound,
			Cause:     fs.ErrNotExist,
		}),
	)
	command := newValidateCmd(validateCmdOpts{}, func(context.Context, *zap.Logger) (validateRuntime, error) {
		return validateRuntime{validator: validator, shutdown: func(context.Context) error { return nil }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Writer = &stdout
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"validate", "--json"})

	var exitCoder cli.ExitCoder
	require.ErrorAs(t, err, &exitCoder)
	require.Equal(t, 1, exitCoder.ExitCode())
	require.Empty(t, err.Error())
	require.ErrorIs(t, err, fs.ErrNotExist)
	require.Empty(t, stdout.String())
	require.Contains(t, stderr.String(), `"msg":"requested resource was not found"`)
	require.Contains(t, stderr.String(), `"code":"not_found"`)
	require.NotContains(t, stderr.String(), "/private/work/devctl.yaml")
}

func TestValidateReportsOperationAndShutdownFailures(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	validator := mocks.NewMockprojectValidator(ctrl)
	operationErr := errors.New("validation failed")
	shutdownErr := errors.New("shutdown failed")
	validator.EXPECT().Validate(gomock.Any(), projectdomain.ValidateQuery{}).Return(projectdomain.ValidationResult{}, operationErr)
	command := newValidateCmd(validateCmdOpts{}, func(context.Context, *zap.Logger) (validateRuntime, error) {
		return validateRuntime{validator: validator, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"validate", "--json"})

	require.ErrorIs(t, err, operationErr)
	require.ErrorIs(t, err, shutdownErr)
	require.Equal(t, 1, bytes.Count(stderr.Bytes(), []byte(`"level":"error"`)))
	require.NotContains(t, stderr.String(), `"data"`)
}

func TestValidateReportsCompletedValidationAfterShutdownFailure(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	validator := mocks.NewMockprojectValidator(ctrl)
	shutdownErr := errors.New("shutdown failed")
	validator.EXPECT().Validate(gomock.Any(), projectdomain.ValidateQuery{}).Return(
		projectdomain.ValidationResult{Issues: []projectdomain.Issue{}}, nil,
	)
	command := newValidateCmd(validateCmdOpts{}, func(context.Context, *zap.Logger) (validateRuntime, error) {
		return validateRuntime{validator: validator, shutdown: func(context.Context) error { return shutdownErr }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stderr bytes.Buffer
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"validate", "--json"})

	require.ErrorIs(t, err, shutdownErr)
	var event map[string]any
	require.NoError(t, json.Unmarshal(stderr.Bytes(), &event))
	require.NotContains(t, event, "data")
	details, ok := event["details"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, map[string]any{"valid": true, "issues": []any{}}, details["partial_result"])
}

func TestValidateWritesIssuesAndExitsOne(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	validator := mocks.NewMockprojectValidator(ctrl)
	validator.EXPECT().Validate(gomock.Any(), projectdomain.ValidateQuery{}).Return(projectdomain.ValidationResult{
		Issues: []projectdomain.Issue{{
			Code: "name_invalid", Path: "devctl.yaml", Line: 3, Column: 9, Field: "project.name",
		}},
	}, nil)
	command := newValidateCmd(validateCmdOpts{}, func(context.Context, *zap.Logger) (validateRuntime, error) {
		return validateRuntime{validator: validator, shutdown: func(context.Context) error { return nil }}, nil
	})
	command.ExitErrHandler = func(context.Context, *cli.Command, error) {}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Writer = &stdout
	command.ErrWriter = &stderr

	err := command.Run(context.Background(), []string{"validate", "--json"})

	var exitCoder cli.ExitCoder
	require.ErrorAs(t, err, &exitCoder)
	require.Equal(t, 1, exitCoder.ExitCode())
	require.Contains(t, stdout.String(), `"msg":"project validation completed"`)
	require.Contains(t, stdout.String(), `"data":{"valid":false,"issues":[{"code":"name_invalid","path":"devctl.yaml","line":3,"column":9,"field":"project.name"}]}`)
	require.Empty(t, stderr.String())
}
