//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublicHTTPServiceWorkflow(t *testing.T) {
	t.Parallel()

	repositoryRoot, err := filepath.Abs("..")
	require.NoError(t, err)
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	binary := filepath.Join(binDir, "devctl")
	run(t, repositoryRoot, nil, "go", "build", "-o", binary, "./cmd/devctl")

	project := filepath.Join(workspace, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	manifest := filepath.Join(project, "devctl.yaml")
	projectEnv := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	run(t, project, projectEnv, binary, "init", "manifest", "--file", manifest, "--lang", "go", "--preset", "http-service", "--name", "fixture-api", "--module", "example.test/fixture-api")
	run(t, project, projectEnv, binary, "init", "scaffold", "--file", manifest)
	run(t, project, projectEnv, "go", "mod", "tidy")
	run(t, project, projectEnv, binary, "sync")
	run(t, project, projectEnv, binary, "lint")
	run(t, project, projectEnv, binary, "gen")
	application := filepath.Join(binDir, "fixture-api")
	run(t, project, projectEnv, "go", "build", "-o", application, "./cmd/fixture-api")
	httpAddress := availableAddress(t)
	runtimeCommand := exec.Command(application, "api")
	runtimeCommand.Dir = project
	runtimeCommand.Env = append(projectEnv, "FIXTURE_API_HTTP_ADDR="+httpAddress, "FIXTURE_API_HEALTH_SERVER_ENABLED=false")
	var runtimeOutput bytes.Buffer
	runtimeCommand.Stdout = &runtimeOutput
	runtimeCommand.Stderr = &runtimeOutput
	require.NoError(t, runtimeCommand.Start())
	t.Cleanup(func() { _ = runtimeCommand.Process.Kill() })
	waitForHTTP(t, httpAddress)
	require.NoError(t, runtimeCommand.Process.Signal(syscall.SIGTERM))
	require.NoError(t, runtimeCommand.Wait(), runtimeOutput.String())

	run(t, project, projectEnv, "go", "test", "./...")

	run(t, project, projectEnv, "git", "init", "--quiet")
	run(t, project, projectEnv, "git", "add", ".")
	run(t, project, projectEnv, "git", "-c", "user.name=Devctl E2E", "-c", "user.email=devctl@example.test", "commit", "--quiet", "-m", "fixture")
	run(t, project, projectEnv, binary, "sync")
	run(t, project, projectEnv, binary, "gen")
	run(t, project, projectEnv, "git", "diff", "--exit-code")
}

func TestKafkaJSONQuicktypeWorkflow(t *testing.T) {
	t.Parallel()

	repositoryRoot, err := filepath.Abs("..")
	require.NoError(t, err)
	workspace := t.TempDir()
	binDir := filepath.Join(workspace, "bin")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	binary := filepath.Join(binDir, "devctl")
	run(t, repositoryRoot, nil, "go", "build", "-o", binary, "./cmd/devctl")

	project := filepath.Join(workspace, "project")
	require.NoError(t, os.MkdirAll(project, 0o755))
	manifest := filepath.Join(project, "devctl.yaml")
	projectEnv := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	run(t, project, projectEnv, binary, "init", "manifest", "--file", manifest, "--lang", "go", "--preset", "cli", "--name", "fixture-events", "--module", "example.test/fixture-events")
	run(t, project, projectEnv, binary, "add", "source", "contracts", "--file", manifest, "--type", "local", "--path", "api/contracts")
	run(t, project, projectEnv, binary, "add", "kafka-consumer", "audit", "--file", manifest,
		"--topic", "fixture_events.audit.created.v1", "--source", "contracts", "--format", "json",
		"--path", "fixture_events.audit.created.v1.json")
	writeFile(t, project, "api/contracts/fixture_events.audit.created.v1.json", `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "AuditEvent",
  "type": "object",
  "additionalProperties": false,
  "properties": {
    "id": {"type": "string"},
    "note": {"type": ["string", "null"]},
    "payload": {"oneOf": [{"type": "string"}, {"type": "integer"}]}
  },
  "required": ["id", "payload"]
}
`)
	run(t, project, projectEnv, binary, "init", "scaffold", "--file", manifest)
	run(t, project, projectEnv, binary, "validate", "--file", manifest)
	run(t, project, projectEnv, binary, "lint", "kafka", "--file", manifest)
	run(t, project, projectEnv, binary, "gen", "kafka", "--file", manifest)

	generatedPath := filepath.Join(project, "gen/kafka/consumer/audit/schema.gen.go")
	generated, err := os.ReadFile(generatedPath)
	require.NoError(t, err)
	require.Contains(t, string(generated), "func UnmarshalAuditEvent")
	require.Contains(t, string(generated), `json:"note,omitempty"`)
	writeFile(t, project, "cmd/roundtrip/main.go", `package main

import (
	"bytes"

	audit "example.test/fixture-events/gen/kafka/consumer/audit"
)

func main() {
	event, err := audit.UnmarshalAuditEvent([]byte("{\"id\":\"1\",\"payload\":\"created\"}"))
	if err != nil {
		panic(err)
	}
	encoded, err := event.Marshal()
	if err != nil {
		panic(err)
	}
	if !bytes.Contains(encoded, []byte("\"payload\":\"created\"")) {
		panic(string(encoded))
	}
}
`)
	run(t, project, projectEnv, "go", "mod", "tidy")
	run(t, project, projectEnv, "go", "test", "./gen/kafka/consumer/audit")
	run(t, project, projectEnv, "go", "run", "./cmd/roundtrip")
	run(t, project, projectEnv, binary, "gen", "kafka", "--file", manifest)
	regenerated, err := os.ReadFile(generatedPath)
	require.NoError(t, err)
	require.Equal(t, generated, regenerated)
}

func writeFile(t *testing.T, root, relative, content string) {
	t.Helper()

	filename := filepath.Join(root, filepath.FromSlash(relative))
	require.NoError(t, os.MkdirAll(filepath.Dir(filename), 0o755))
	require.NoError(t, os.WriteFile(filename, []byte(content), 0o644))
}

func availableAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := listener.Addr().String()
	require.NoError(t, listener.Close())
	return address
}

func waitForHTTP(t *testing.T, address string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for ctx.Err() == nil {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/", nil)
		require.NoError(t, err)
		response, err := client.Do(request)
		if err == nil {
			require.NoError(t, response.Body.Close())
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	require.FailNow(t, "HTTP server did not become ready", "address: %s", address)
}

func run(t *testing.T, directory string, environment []string, name string, args ...string) {
	t.Helper()
	command := exec.Command(name, args...)
	command.Dir = directory
	if environment != nil {
		command.Env = environment
	}
	output, err := command.CombinedOutput()
	require.NoError(t, err, "%s %v failed:\n%s", name, args, output)
}
