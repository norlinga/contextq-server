package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/norlinga/contextq-server/internal/namespace"
)

func TestContextqIntegration(t *testing.T) {
	binary := os.Getenv("CONTEXTQ_INTEGRATION_BINARY")
	if binary == "" {
		t.Skip("set CONTEXTQ_INTEGRATION_BINARY to a contextq binary")
	}
	store, err := namespace.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Init("integration"); err != nil {
		t.Fatal(err)
	}
	issued, err := store.IssueKey("integration", "integration-test")
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, CommandRunner{Binary: binary}, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})

	call := func(args ...string) map[string]any {
		t.Helper()
		body, err := json.Marshal(execRequest{Args: args})
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/v1/integration/exec", bytes.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+issued.Token)
		response := httptest.NewRecorder()
		server.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("args=%q status=%d body=%s", args, response.Code, response.Body.String())
		}
		var result map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	queue := call("queue", "create", "integration queue", "--name", "work")
	if queue["name"] != "work" {
		t.Fatalf("queue=%v", queue)
	}
	pushed := call("item", "push", "work", "job-1")
	if pushed["state"] != "AVAILABLE" {
		t.Fatalf("pushed=%v", pushed)
	}
	popped := call("item", "pop", "work")
	if popped["state"] != "CLAIMED" || popped["key"] != "job-1" {
		t.Fatalf("popped=%v", popped)
	}
}
