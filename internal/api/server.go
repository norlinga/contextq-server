package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/norlinga/contextq-server/internal/namespace"
)

const maxRequestBody = 64 << 10

type Options struct {
	RequestTimeout time.Duration
	MaxConcurrent  int
}

type Server struct {
	namespaces *namespace.Store
	runner     Runner
	logger     *slog.Logger
	timeout    time.Duration
	semaphore  chan struct{}
	mux        *http.ServeMux
}

type execRequest struct {
	Args []string `json:"args"`
}

type errorResponse struct {
	Code  string `json:"code"`
	Error string `json:"error"`
}

func NewServer(namespaces *namespace.Store, runner Runner, logger *slog.Logger, options Options) *Server {
	if logger == nil {
		logger = slog.Default()
	}
	if options.RequestTimeout <= 0 {
		options.RequestTimeout = 15 * time.Second
	}
	if options.MaxConcurrent <= 0 {
		options.MaxConcurrent = 32
	}
	s := &Server{
		namespaces: namespaces,
		runner:     runner,
		logger:     logger,
		timeout:    options.RequestTimeout,
		semaphore:  make(chan struct{}, options.MaxConcurrent),
		mux:        http.NewServeMux(),
	}
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("POST /v1/{namespace}/exec", s.execute)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	s.mux.ServeHTTP(w, r)
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) execute(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("namespace")
	token, ok := bearerToken(r.Header.Get("Authorization"))
	if !ok {
		unauthorized(w)
		return
	}
	key, err := s.namespaces.Authenticate(name, token)
	if err != nil {
		if !errors.Is(err, namespace.ErrInvalidKey) {
			s.logger.Error("authenticate key", "namespace", name, "error", err)
		}
		unauthorized(w)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBody)
	var request execRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", fmt.Sprintf("invalid request body: %v", err))
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		writeAPIError(w, http.StatusBadRequest, "invalid_request", "request body must contain one JSON object")
		return
	}
	if err := validateArgs(request.Args); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_arguments", err.Error())
		return
	}

	namespaceDir, err := s.namespaces.NamespaceDir(name)
	if err != nil {
		unauthorized(w)
		return
	}
	contextqRoot, err := s.namespaces.ContextqRoot(name)
	if err != nil {
		unauthorized(w)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.timeout)
	defer cancel()
	select {
	case s.semaphore <- struct{}{}:
		defer func() { <-s.semaphore }()
	case <-ctx.Done():
		writeAPIError(w, http.StatusServiceUnavailable, "server_busy", "request could not start before its deadline")
		return
	}

	started := time.Now()
	result, err := s.runner.Run(ctx, namespaceDir, contextqRoot, request.Args)
	duration := time.Since(started)
	if err != nil {
		s.logger.Error("run contextq", "namespace", name, "key_id", key.ID, "key_label", key.Label, "command", commandName(request.Args), "duration", duration, "error", err)
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			writeAPIError(w, http.StatusGatewayTimeout, "command_timeout", "contextq command timed out")
		case errors.Is(err, context.Canceled):
			return
		case errors.Is(err, ErrOutputLimit):
			writeAPIError(w, http.StatusBadGateway, "output_limit", ErrOutputLimit.Error())
		default:
			writeAPIError(w, http.StatusBadGateway, "command_failed", "could not execute contextq")
		}
		return
	}

	s.logger.Info("contextq command", "namespace", name, "key_id", key.ID, "key_label", key.Label, "command", commandName(request.Args), "exit_code", result.ExitCode, "duration", duration)
	if result.ExitCode == 0 {
		writeCommandJSON(w, http.StatusOK, result.Stdout)
		return
	}
	status := contextqErrorStatus(result.Stderr)
	writeCommandJSON(w, status, result.Stderr)
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return "", false
	}
	return parts[1], parts[1] != ""
}

func validateArgs(args []string) error {
	if len(args) < 2 || len(args) > 64 {
		return fmt.Errorf("args must contain a contextq command and subcommand")
	}
	allowed := map[string]map[string]bool{
		"queue": {
			"create":  true,
			"list":    true,
			"read":    true,
			"destroy": true,
		},
		"item": {
			"push":    true,
			"pop":     true,
			"list":    true,
			"read":    true,
			"update":  true,
			"history": true,
		},
	}
	if !allowed[args[0]][args[1]] {
		return fmt.Errorf("unsupported contextq command %q", strings.Join(args[:2], " "))
	}
	flagsEnded := false
	totalLength := 0
	for _, arg := range args {
		totalLength += len(arg)
		if strings.IndexByte(arg, 0) >= 0 {
			return fmt.Errorf("arguments may not contain NUL bytes")
		}
		if arg == "--" && !flagsEnded {
			flagsEnded = true
			continue
		}
		if flagsEnded {
			continue
		}
		if arg == "--root" || strings.HasPrefix(arg, "--root=") ||
			arg == "--config" || strings.HasPrefix(arg, "--config=") ||
			arg == "--json" || strings.HasPrefix(arg, "--json=") ||
			arg == "--help" || arg == "-h" || arg == "--version" {
			return fmt.Errorf("argument %q is controlled by contextq-server", arg)
		}
	}
	if totalLength > maxRequestBody/2 {
		return fmt.Errorf("arguments are too large")
	}
	return nil
}

func commandName(args []string) string {
	if len(args) < 2 {
		return ""
	}
	return args[0] + " " + args[1]
}

func contextqErrorStatus(body []byte) int {
	var response errorResponse
	if json.Unmarshal(body, &response) != nil {
		return http.StatusBadGateway
	}
	switch response.Code {
	case "queue_not_found":
		return http.StatusNotFound
	case "queue_name_ambiguous", "duplicate_available_key", "no_available_items", "invalid_state_transition", "no_active_lifecycle":
		return http.StatusConflict
	case "force_required", "error":
		return http.StatusBadRequest
	default:
		return http.StatusBadGateway
	}
}

func writeCommandJSON(w http.ResponseWriter, status int, body []byte) {
	if !json.Valid(body) {
		writeAPIError(w, http.StatusBadGateway, "invalid_command_output", "contextq returned invalid JSON")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Bearer realm="contextq"`)
	writeAPIError(w, http.StatusUnauthorized, "unauthorized", "invalid namespace or access key")
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, errorResponse{Code: code, Error: message})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
