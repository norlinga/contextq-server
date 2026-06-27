package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/norlinga/contextq-server/internal/namespace"
)

type runnerCall struct {
	namespaceDir string
	contextqRoot string
	args         []string
}

type fakeRunner struct {
	mu     sync.Mutex
	calls  []runnerCall
	result Result
	err    error
	run    func(context.Context) error
}

func (r *fakeRunner) Run(ctx context.Context, namespaceDir, contextqRoot string, args []string) (Result, error) {
	r.mu.Lock()
	r.calls = append(r.calls, runnerCall{namespaceDir: namespaceDir, contextqRoot: contextqRoot, args: append([]string(nil), args...)})
	r.mu.Unlock()
	if r.run != nil {
		if err := r.run(ctx); err != nil {
			return Result{}, err
		}
	}
	return r.result, r.err
}

func TestAuthenticatedExecUsesNamespacePaths(t *testing.T) {
	store, token := testNamespace(t, "project-a")
	runner := &fakeRunner{result: Result{Stdout: []byte(`{"state":"CLAIMED"}`)}}
	server := NewServer(store, runner, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})

	request := httptest.NewRequest(http.MethodPost, "/v1/project-a/exec", bytes.NewBufferString(`{"args":["item","pop","work"]}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if len(runner.calls) != 1 {
		t.Fatalf("calls=%d", len(runner.calls))
	}
	call := runner.calls[0]
	wantDir, _ := store.NamespaceDir("project-a")
	wantRoot, _ := store.ContextqRoot("project-a")
	if call.namespaceDir != wantDir || call.contextqRoot != wantRoot {
		t.Fatalf("paths=(%q, %q), want (%q, %q)", call.namespaceDir, call.contextqRoot, wantDir, wantRoot)
	}
	if !reflect.DeepEqual(call.args, []string{"item", "pop", "work"}) {
		t.Fatalf("args=%q", call.args)
	}
}

func TestExecRejectsInvalidAuthenticationAndControlledFlags(t *testing.T) {
	store, token := testNamespace(t, "project-a")
	runner := &fakeRunner{result: Result{Stdout: []byte(`{}`)}}
	server := NewServer(store, runner, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})

	tests := []struct {
		name   string
		token  string
		body   string
		status int
	}{
		{name: "missing key", body: `{"args":["queue","list"]}`, status: http.StatusUnauthorized},
		{name: "wrong key", token: "cqk_k_deadbeef_secret", body: `{"args":["queue","list"]}`, status: http.StatusUnauthorized},
		{name: "root override", token: token, body: `{"args":["queue","list","--root","/tmp/escape"]}`, status: http.StatusBadRequest},
		{name: "unsupported command", token: token, body: `{"args":["admin","anything"]}`, status: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/project-a/exec", bytes.NewBufferString(test.body))
			if test.token != "" {
				request.Header.Set("Authorization", "Bearer "+test.token)
			}
			response := httptest.NewRecorder()
			server.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner called %d times", len(runner.calls))
	}
}

func TestContextqErrorsPreserveJSONAndMapStatus(t *testing.T) {
	store, token := testNamespace(t, "project-a")
	body := []byte(`{"code":"no_available_items","error":"no available items"}`)
	runner := &fakeRunner{result: Result{Stderr: body, ExitCode: 1}}
	server := NewServer(store, runner, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{})

	request := httptest.NewRequest(http.MethodPost, "/v1/project-a/exec", bytes.NewBufferString(`{"args":["item","pop","work"]}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)

	if response.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var got, want map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(body, &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("body=%v want=%v", got, want)
	}
}

func TestCommandTimeout(t *testing.T) {
	store, token := testNamespace(t, "project-a")
	runner := &fakeRunner{run: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	server := NewServer(store, runner, slog.New(slog.NewTextHandler(io.Discard, nil)), Options{RequestTimeout: 10 * time.Millisecond})

	request := httptest.NewRequest(http.MethodPost, "/v1/project-a/exec", bytes.NewBufferString(`{"args":["queue","list"]}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func testNamespace(t *testing.T, name string) (*namespace.Store, string) {
	t.Helper()
	store, err := namespace.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Init(name); err != nil {
		t.Fatal(err)
	}
	issued, err := store.IssueKey(name, "test-agent")
	if err != nil {
		t.Fatal(err)
	}
	return store, issued.Token
}
