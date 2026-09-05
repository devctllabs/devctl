package toolrun_test

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/devctllabs/devctl/internal/client/toolrun"
	"github.com/stretchr/testify/require"
)

func TestOSRunnerPreservesCommandArgumentsAndWorkingDirectory(t *testing.T) {
	workingDirectory := t.TempDir()
	t.Setenv("DEVCTL_TOOLRUN_HELPER", "command")
	t.Setenv("DEVCTL_TOOLRUN_EXPECTED_DIR", workingDirectory)

	err := toolrun.New().Run(context.Background(), toolrun.Command{
		Name: os.Args[0],
		Args: []string{"-test.run=TestToolrunHelperProcess", "--", "first", "second value"},
		Dir:  workingDirectory,
	})

	require.NoError(t, err)
}

func TestOSRunnerPreservesExitErrorAndBoundsDiagnostics(t *testing.T) {
	t.Setenv("DEVCTL_TOOLRUN_HELPER", "failure")

	err := toolrun.New().Run(context.Background(), toolrun.Command{
		Name: os.Args[0], Args: []string{"-test.run=TestToolrunHelperProcess"},
	})

	require.Error(t, err)
	var exitError *exec.ExitError
	require.ErrorAs(t, err, &exitError)
	require.Equal(t, 7, exitError.ExitCode())
	require.Contains(t, err.Error(), strings.Repeat("x", 64))
	require.Less(t, len(err.Error()), 4300)
}

func TestOSRunnerPreservesContextCancellation(t *testing.T) {
	t.Setenv("DEVCTL_TOOLRUN_HELPER", "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := toolrun.New().Run(ctx, toolrun.Command{
		Name: os.Args[0], Args: []string{"-test.run=TestToolrunHelperProcess"},
	})

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestToolrunHelperProcess(t *testing.T) { //nolint:paralleltest // The parent tests control this subprocess through inherited environment.
	switch os.Getenv("DEVCTL_TOOLRUN_HELPER") {
	case "":
		return
	case "command":
		workingDirectory, err := os.Getwd()
		if err != nil {
			os.Exit(2)
		}
		separator := -1
		for index, argument := range os.Args {
			if argument == "--" {
				separator = index
				break
			}
		}
		if workingDirectory != os.Getenv("DEVCTL_TOOLRUN_EXPECTED_DIR") ||
			separator == -1 || fmt.Sprint(os.Args[separator+1:]) != "[first second value]" {
			os.Exit(3)
		}
	case "failure":
		_, _ = fmt.Fprint(os.Stderr, strings.Repeat("x", 5000))
		os.Exit(7)
	case "wait":
		time.Sleep(time.Minute)
	default:
		os.Exit(4)
	}
}
