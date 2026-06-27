package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamespaceAndKeyCommands(t *testing.T) {
	root := t.TempDir()
	var stdout, stderr bytes.Buffer

	exit := Run(context.Background(), []string{"namespace", "init", "--data-root", root, "agents"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("namespace init exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()

	exit = Run(context.Background(), []string{"key", "add", "--data-root", root, "--label", "build-agent", "--json", "agents"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("key add exit=%d stderr=%s", exit, stderr.String())
	}
	var issued struct {
		ID    string `json:"id"`
		Label string `json:"label"`
		Token string `json:"token"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &issued); err != nil {
		t.Fatal(err)
	}
	if issued.ID == "" || issued.Token == "" || issued.Label != "build-agent" {
		t.Fatalf("issued=%#v", issued)
	}

	stdout.Reset()
	stderr.Reset()
	exit = Run(context.Background(), []string{"key", "list", "--data-root", root, "agents"}, &stdout, &stderr)
	if exit != 0 || !strings.Contains(stdout.String(), issued.ID) || strings.Contains(stdout.String(), issued.Token) {
		t.Fatalf("key list exit=%d stdout=%s stderr=%s", exit, stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	exit = Run(context.Background(), []string{"key", "revoke", "--data-root", root, "agents", issued.ID}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("key revoke exit=%d stderr=%s", exit, stderr.String())
	}
}

func TestKeyAddRequiresLabel(t *testing.T) {
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{"key", "add", "agents"}, &stdout, &stderr)
	if exit != 2 || !strings.Contains(stderr.String(), "--label is required") {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
}

func TestTargetAndRemoteExecCommands(t *testing.T) {
	var received struct {
		Authorization string
		Args          []string
	}
	oldTransport := clientTransport
	clientTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		received.Authorization = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Args []string `json:"args"`
		}
		_ = json.Unmarshal(body, &request)
		received.Args = request.Args
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(`[{"name":"work"}]`)),
		}, nil
	})
	defer func() { clientTransport = oldTransport }()

	configPath := filepath.Join(t.TempDir(), "targets.json")
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{
		"target", "add", "--config", configPath,
		"--url", "https://q.example.com",
		"--namespace", "agents",
		"--key", "secret-key",
		"--use", "local",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("target add exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit = Run(context.Background(), []string{"exec", "--config", configPath, "queue", "list"}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("exec exit=%d stderr=%s", exit, stderr.String())
	}
	if received.Authorization != "Bearer secret-key" {
		t.Fatalf("authorization=%q", received.Authorization)
	}
	if strings.Join(received.Args, " ") != "queue list" {
		t.Fatalf("args=%q", received.Args)
	}
	if stdout.String() != `[{"name":"work"}]` {
		t.Fatalf("stdout=%q", stdout.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestRemoteInitPreview(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "targets.json")
	var stdout, stderr bytes.Buffer
	exit := Run(context.Background(), []string{
		"target", "add", "--config", configPath,
		"--url", "https://q.longship.dev",
		"--namespace", "agents",
		"--use", "longship",
	}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("target add exit=%d stderr=%s", exit, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	exit = Run(context.Background(), []string{"remote-init", "--config", configPath}, &stdout, &stderr)
	if exit != 0 {
		t.Fatalf("remote-init exit=%d stderr=%s", exit, stderr.String())
	}
	for _, text := range []string{"User=contextq", "q.longship.dev {", "contextq-server bootstrap complete"} {
		if !strings.Contains(stdout.String(), text) {
			t.Errorf("preview missing %q", text)
		}
	}
}

func TestLinuxBinaryArch(t *testing.T) {
	for name, machine := range map[string]uint16{"amd64": 62, "arm64": 183} {
		t.Run(name, func(t *testing.T) {
			header := make([]byte, 20)
			copy(header, []byte{0x7f, 'E', 'L', 'F'})
			header[4] = 2
			header[5] = 1
			header[18] = byte(machine)
			header[19] = byte(machine >> 8)
			path := filepath.Join(t.TempDir(), "binary")
			if err := os.WriteFile(path, header, 0o700); err != nil {
				t.Fatal(err)
			}
			got, err := linuxBinaryArch(path)
			if err != nil {
				t.Fatal(err)
			}
			if got != name {
				t.Fatalf("arch=%q want=%q", got, name)
			}
		})
	}
}
