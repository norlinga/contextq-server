package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/norlinga/contextq-server/internal/api"
	"github.com/norlinga/contextq-server/internal/namespace"
)

const defaultDataRoot = "/var/contextq"

func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 2
	}
	var err error
	switch args[0] {
	case "serve":
		err = runServe(ctx, args[1:], stdout, stderr)
	case "target":
		err = runTarget(args[1:], stdout, stderr)
	case "remote-init":
		err = runRemoteInit(ctx, args[1:], stdout, stderr)
	case "remote":
		err = runRemote(ctx, args[1:], stdout, stderr)
	case "exec":
		err = runClientExec(ctx, args[1:], stdout, stderr)
	case "doctor":
		err = runDoctor(ctx, args[1:], stdout, stderr)
	case "namespace":
		err = runNamespace(args[1:], stdout, stderr)
	case "key":
		err = runKey(args[1:], stdout, stderr)
	case "help", "-h", "--help":
		printUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		printUsage(stderr)
		return 2
	}
	if err == nil {
		return 0
	}
	var usageErr *usageError
	if errors.As(err, &usageErr) {
		fmt.Fprintln(stderr, usageErr)
		return 2
	}
	fmt.Fprintln(stderr, err)
	return 1
}

type usageError struct {
	message string
}

func (e *usageError) Error() string { return e.message }

func runServe(ctx context.Context, args []string, _ io.Writer, stderr io.Writer) error {
	flags := newFlagSet("serve", stderr)
	listen := flags.String("listen", "127.0.0.1:8787", "address for the HTTP server")
	dataRoot := flags.String("data-root", dataRootDefault(), "namespace data root")
	contextqBin := flags.String("contextq-bin", "contextq", "path to the contextq binary")
	requestTimeout := flags.Duration("request-timeout", 15*time.Second, "maximum command duration")
	shutdownTimeout := flags.Duration("shutdown-timeout", 10*time.Second, "graceful shutdown timeout")
	maxConcurrent := flags.Int("max-concurrent", 32, "maximum simultaneous contextq commands")
	outputLimit := flags.Int("output-limit", 4<<20, "maximum bytes for stdout or stderr")
	if err := parseFlags(flags, args, 0); err != nil {
		return err
	}
	if *requestTimeout <= 0 || *shutdownTimeout <= 0 || *maxConcurrent <= 0 || *outputLimit <= 0 {
		return &usageError{message: "timeouts, max-concurrent, and output-limit must be positive"}
	}

	store, err := namespace.NewStore(*dataRoot)
	if err != nil {
		return err
	}
	binary, err := exec.LookPath(*contextqBin)
	if err != nil {
		return fmt.Errorf("find contextq binary: %w", err)
	}
	if !filepath.IsAbs(binary) {
		binary, err = filepath.Abs(binary)
		if err != nil {
			return err
		}
	}
	logger := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{}))
	handler := api.NewServer(store, api.CommandRunner{Binary: binary, OutputLimit: *outputLimit}, logger, api.Options{
		RequestTimeout: *requestTimeout,
		MaxConcurrent:  *maxConcurrent,
	})
	httpServer := &http.Server{
		Addr:              *listen,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}

	serveErr := make(chan error, 1)
	go func() {
		logger.Info("contextq-server listening", "address", *listen, "data_root", store.Root(), "contextq_binary", binary)
		serveErr <- httpServer.ListenAndServe()
	}()

	signalCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	select {
	case err := <-serveErr:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-signalCtx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), *shutdownTimeout)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		return nil
	}
}

func runNamespace(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{message: "usage: contextq-server namespace <init|list>"}
	}
	switch args[0] {
	case "init":
		flags := newFlagSet("namespace init", stderr)
		dataRoot := flags.String("data-root", dataRootDefault(), "namespace data root")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[1:], 1); err != nil {
			return err
		}
		store, err := namespace.NewStore(*dataRoot)
		if err != nil {
			return err
		}
		ns, err := store.Init(flags.Arg(0))
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(stdout, ns)
		}
		_, err = fmt.Fprintf(stdout, "initialized %s\n", ns.Name)
		return err
	case "list":
		flags := newFlagSet("namespace list", stderr)
		dataRoot := flags.String("data-root", dataRootDefault(), "namespace data root")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[1:], 0); err != nil {
			return err
		}
		store, err := namespace.NewStore(*dataRoot)
		if err != nil {
			return err
		}
		namespaces, err := store.List()
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(stdout, namespaces)
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, ns := range namespaces {
			fmt.Fprintf(writer, "%s\t%s\n", ns.Name, ns.CreatedAt.Format(time.RFC3339))
		}
		return writer.Flush()
	default:
		return &usageError{message: fmt.Sprintf("unknown namespace command %q", args[0])}
	}
}

func runKey(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{message: "usage: contextq-server key <add|list|revoke>"}
	}
	switch args[0] {
	case "add":
		flags := newFlagSet("key add", stderr)
		dataRoot := flags.String("data-root", dataRootDefault(), "namespace data root")
		label := flags.String("label", "", "required human-readable key label")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[1:], 1); err != nil {
			return err
		}
		if *label == "" {
			return &usageError{message: "--label is required"}
		}
		store, err := namespace.NewStore(*dataRoot)
		if err != nil {
			return err
		}
		issued, err := store.IssueKey(flags.Arg(0), *label)
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(stdout, issued)
		}
		_, err = fmt.Fprintf(stdout, "id: %s\nlabel: %s\ntoken: %s\n", issued.ID, issued.Label, issued.Token)
		return err
	case "list":
		flags := newFlagSet("key list", stderr)
		dataRoot := flags.String("data-root", dataRootDefault(), "namespace data root")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[1:], 1); err != nil {
			return err
		}
		store, err := namespace.NewStore(*dataRoot)
		if err != nil {
			return err
		}
		keys, err := store.ListKeys(flags.Arg(0))
		if err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(stdout, keys)
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, key := range keys {
			fmt.Fprintf(writer, "%s\t%s\t%s\n", key.ID, key.Label, key.CreatedAt.Format(time.RFC3339))
		}
		return writer.Flush()
	case "revoke":
		flags := newFlagSet("key revoke", stderr)
		dataRoot := flags.String("data-root", dataRootDefault(), "namespace data root")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[1:], 2); err != nil {
			return err
		}
		store, err := namespace.NewStore(*dataRoot)
		if err != nil {
			return err
		}
		if err := store.RevokeKey(flags.Arg(0), flags.Arg(1)); err != nil {
			return err
		}
		if *asJSON {
			return writeJSON(stdout, map[string]any{"revoked": true, "id": flags.Arg(1)})
		}
		_, err = fmt.Fprintf(stdout, "revoked %s\n", flags.Arg(1))
		return err
	default:
		return &usageError{message: fmt.Sprintf("unknown key command %q", args[0])}
	}
}

func newFlagSet(name string, stderr io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(stderr)
	return flags
}

func parseFlags(flags *flag.FlagSet, args []string, positional int) error {
	if err := flags.Parse(args); err != nil {
		return &usageError{message: err.Error()}
	}
	if flags.NArg() != positional {
		return &usageError{message: fmt.Sprintf("%s expects %d positional argument(s), got %d", flags.Name(), positional, flags.NArg())}
	}
	return nil
}

func dataRootDefault() string {
	if value := os.Getenv("CONTEXTQ_DATA_ROOT"); value != "" {
		return value
	}
	return defaultDataRoot
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `contextq-server serves namespace-scoped contextq commands.

Usage:
  contextq-server serve [flags]
  contextq-server target <add|list|use|remove> [flags]
  contextq-server remote-init [-t target] [--apply] [flags]
  contextq-server remote namespace init [-t target]
  contextq-server remote key <add|list|revoke> [-t target] [flags]
  contextq-server exec [-t target] <contextq arguments...>
  contextq-server doctor [-t target]
  contextq-server namespace init [--data-root path] [--json] <namespace>
  contextq-server namespace list [--data-root path] [--json]
  contextq-server key add [--data-root path] [--json] --label <label> <namespace>
  contextq-server key list [--data-root path] [--json] <namespace>
  contextq-server key revoke [--data-root path] [--json] <namespace> <key-id>

Environment:
  CONTEXTQ_DATA_ROOT      default namespace data root (default /var/contextq)
  CONTEXTQ_SERVER_CONFIG  local target file (default ~/.contextq-server)`)
}
