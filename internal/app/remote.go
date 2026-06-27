package app

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/norlinga/contextq-server/internal/namespace"
	"github.com/norlinga/contextq-server/internal/remote"
	"github.com/norlinga/contextq-server/internal/setup"
	"github.com/norlinga/contextq-server/internal/target"
)

var clientTransport http.RoundTripper = http.DefaultTransport

func runTarget(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return &usageError{message: "usage: contextq-server target <add|list|use|remove>"}
	}
	switch args[0] {
	case "add":
		flags := newFlagSet("target add", stderr)
		configPath := flags.String("config", "", "target configuration path")
		url := flags.String("url", "", "public contextq-server origin")
		namespaceName := flags.String("namespace", "", "namespace used by this target")
		sshHost := flags.String("ssh-host", "", "SSH host (defaults to URL hostname)")
		sshUser := flags.String("ssh-user", "root", "SSH administrator user")
		sshPort := flags.Int("ssh-port", 0, "SSH port")
		identity := flags.String("identity", "", "SSH identity file")
		key := flags.String("key", "", "existing namespace bearer key")
		use := flags.Bool("use", false, "make this the default target")
		if err := parseFlags(flags, args[1:], 1); err != nil {
			return err
		}
		cfg, path, err := loadTargets(*configPath)
		if err != nil {
			return err
		}
		name := flags.Arg(0)
		if existing, ok := cfg.Targets[name]; ok && *key == "" {
			*key = existing.Key
		}
		t := target.Target{URL: *url, Namespace: *namespaceName, Key: *key, SSHHost: *sshHost, SSHUser: *sshUser, SSHPort: *sshPort, Identity: *identity}
		if err := cfg.Set(name, t, *use); err != nil {
			return err
		}
		if err := cfg.Save(path); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "saved target %s\n", name)
		return err
	case "list":
		flags := newFlagSet("target list", stderr)
		configPath := flags.String("config", "", "target configuration path")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[1:], 0); err != nil {
			return err
		}
		cfg, _, err := loadTargets(*configPath)
		if err != nil {
			return err
		}
		if *asJSON {
			type safeTarget struct {
				Name       string `json:"name"`
				URL        string `json:"url"`
				Namespace  string `json:"namespace"`
				SSHHost    string `json:"ssh_host"`
				Default    bool   `json:"default"`
				KeyPresent bool   `json:"key_present"`
			}
			out := []safeTarget{}
			for _, name := range cfg.Names() {
				t := cfg.Targets[name]
				out = append(out, safeTarget{Name: name, URL: t.URL, Namespace: t.Namespace, SSHHost: t.SSHHost, Default: name == cfg.Default, KeyPresent: t.Key != ""})
			}
			return writeJSON(stdout, out)
		}
		writer := tabwriter.NewWriter(stdout, 0, 4, 2, ' ', 0)
		for _, name := range cfg.Names() {
			t := cfg.Targets[name]
			marker := " "
			if name == cfg.Default {
				marker = "*"
			}
			keyStatus := "no-key"
			if t.Key != "" {
				keyStatus = "key-ready"
			}
			fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n", marker, name, t.URL, t.Namespace, keyStatus)
		}
		return writer.Flush()
	case "use":
		flags := newFlagSet("target use", stderr)
		configPath := flags.String("config", "", "target configuration path")
		if err := parseFlags(flags, args[1:], 1); err != nil {
			return err
		}
		cfg, path, err := loadTargets(*configPath)
		if err != nil {
			return err
		}
		name := flags.Arg(0)
		if _, ok := cfg.Targets[name]; !ok {
			return fmt.Errorf("target %q does not exist", name)
		}
		cfg.Default = name
		if err := cfg.Save(path); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "using target %s\n", name)
		return err
	case "remove":
		flags := newFlagSet("target remove", stderr)
		configPath := flags.String("config", "", "target configuration path")
		if err := parseFlags(flags, args[1:], 1); err != nil {
			return err
		}
		cfg, path, err := loadTargets(*configPath)
		if err != nil {
			return err
		}
		name := flags.Arg(0)
		if _, ok := cfg.Targets[name]; !ok {
			return fmt.Errorf("target %q does not exist", name)
		}
		delete(cfg.Targets, name)
		if cfg.Default == name {
			cfg.Default = ""
			names := cfg.Names()
			if len(names) > 0 {
				cfg.Default = names[0]
			}
		}
		if err := cfg.Save(path); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "removed target %s\n", name)
		return err
	default:
		return &usageError{message: fmt.Sprintf("unknown target command %q", args[0])}
	}
}

func runRemoteInit(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("remote-init", stderr)
	configPath := flags.String("config", "", "target configuration path")
	targetName := flags.String("target", "", "target name")
	flags.StringVar(targetName, "t", "", "target name")
	apply := flags.Bool("apply", false, "upload binaries and apply the script")
	serverBinary := flags.String("server-binary", "", "local Linux contextq-server binary")
	contextqBinary := flags.String("contextq-binary", "", "local Linux contextq binary")
	listen := flags.String("listen", "127.0.0.1:8787", "remote loopback listen address")
	label := flags.String("label", "", "label for the initial namespace key")
	if err := parseFlags(flags, args, 0); err != nil {
		return err
	}
	cfg, path, resolved, err := resolveTarget(*configPath, *targetName)
	if err != nil {
		return err
	}

	stageDir := "/tmp/contextq-install"
	if *apply {
		stageDir, err = randomStageDir()
		if err != nil {
			return err
		}
	}
	options := setup.BootstrapOptions{
		Target:          resolved.Target,
		TargetName:      resolved.Name,
		Listen:          *listen,
		StagedServer:    stageDir + "/contextq-server",
		StagedContextq:  stageDir + "/contextq",
		SystemdUnitPath: "/etc/systemd/system/" + setup.ServiceName,
	}
	script, err := setup.BootstrapScript(options)
	if err != nil {
		return err
	}
	if !*apply {
		_, err := io.WriteString(stdout, script)
		return err
	}

	executor := remote.OSExecutor{}
	sshSession, err := remote.NewSession(resolved.Target)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = sshSession.Close(closeCtx, executor)
	}()
	remoteTarget := sshSession.Target()
	remoteArch, err := detectRemoteArch(ctx, executor, remoteTarget)
	if err != nil {
		return err
	}
	*serverBinary, *contextqBinary, err = resolveBootstrapBinaries(remoteArch, *serverBinary, *contextqBinary)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "deploying linux/%s binaries\n", remoteArch)
	if _, err := remote.RunSSH(ctx, executor, remoteTarget, remote.ShellCommand("install", "-d", "-m", "0700", stageDir), nil); err != nil {
		return fmt.Errorf("create remote staging directory: %w", err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = remote.RunSSH(cleanupCtx, executor, remoteTarget, remote.ShellCommand("rm", "-rf", "--", stageDir), nil)
	}()
	if err := remote.Upload(ctx, executor, remoteTarget, *serverBinary, options.StagedServer); err != nil {
		return err
	}
	if err := remote.Upload(ctx, executor, remoteTarget, *contextqBinary, options.StagedContextq); err != nil {
		return err
	}
	result, err := remote.RunSSH(ctx, executor, remoteTarget, "sh -s", strings.NewReader(script))
	if len(result.Stdout) > 0 {
		_, _ = stdout.Write(result.Stdout)
	}
	if len(result.Stderr) > 0 {
		_, _ = stderr.Write(result.Stderr)
	}
	if err != nil {
		return fmt.Errorf("apply remote bootstrap: %w", err)
	}
	if err := remoteNamespaceInit(ctx, executor, remoteTarget); err != nil {
		return err
	}
	if resolved.Target.Key == "" {
		if *label == "" {
			*label = defaultKeyLabel()
		}
		issued, err := remoteKeyAdd(ctx, executor, remoteTarget, *label)
		if err != nil {
			return err
		}
		updated := resolved.Target
		updated.Key = issued.Token
		cfg.Targets[resolved.Name] = updated
		if err := cfg.Save(path); err != nil {
			return fmt.Errorf("save issued key locally: %w; recover the issued token now: %s", err, issued.Token)
		}
		fmt.Fprintf(stdout, "configured namespace %s with key %s (%s)\n", updated.Namespace, issued.ID, issued.Label)
	}
	return nil
}

func runRemote(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	if len(args) < 2 {
		return &usageError{message: "usage: contextq-server remote <namespace init|key add|key list|key revoke>"}
	}
	switch args[0] + " " + args[1] {
	case "namespace init":
		flags := newFlagSet("remote namespace init", stderr)
		configPath, targetName := remoteTargetFlags(flags)
		if err := parseFlags(flags, args[2:], 0); err != nil {
			return err
		}
		_, _, resolved, err := resolveTarget(*configPath, *targetName)
		if err != nil {
			return err
		}
		if err := remoteNamespaceInit(ctx, remote.OSExecutor{}, resolved.Target); err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "initialized remote namespace %s\n", resolved.Target.Namespace)
		return err
	case "key add":
		flags := newFlagSet("remote key add", stderr)
		configPath, targetName := remoteTargetFlags(flags)
		label := flags.String("label", "", "required human-readable key label")
		noSave := flags.Bool("no-save", false, "do not save the new key to the target")
		asJSON := flags.Bool("json", false, "emit JSON")
		if err := parseFlags(flags, args[2:], 0); err != nil {
			return err
		}
		if *label == "" {
			return &usageError{message: "--label is required"}
		}
		cfg, path, resolved, err := resolveTarget(*configPath, *targetName)
		if err != nil {
			return err
		}
		issued, err := remoteKeyAdd(ctx, remote.OSExecutor{}, resolved.Target, *label)
		if err != nil {
			return err
		}
		if !*noSave {
			updated := resolved.Target
			updated.Key = issued.Token
			cfg.Targets[resolved.Name] = updated
			if err := cfg.Save(path); err != nil {
				return fmt.Errorf("save issued key locally: %w; recover the issued token now: %s", err, issued.Token)
			}
		}
		if *asJSON {
			return writeJSON(stdout, issued)
		}
		_, err = fmt.Fprintf(stdout, "id: %s\nlabel: %s\ntoken: %s\n", issued.ID, issued.Label, issued.Token)
		return err
	case "key list":
		flags := newFlagSet("remote key list", stderr)
		configPath, targetName := remoteTargetFlags(flags)
		if err := parseFlags(flags, args[2:], 0); err != nil {
			return err
		}
		_, _, resolved, err := resolveTarget(*configPath, *targetName)
		if err != nil {
			return err
		}
		result, err := runRemoteAdmin(ctx, remote.OSExecutor{}, resolved.Target, "key", "list", "--data-root", resolved.Target.DataRoot, "--json", resolved.Target.Namespace)
		if err != nil {
			return err
		}
		_, err = stdout.Write(result.Stdout)
		return err
	case "key revoke":
		flags := newFlagSet("remote key revoke", stderr)
		configPath, targetName := remoteTargetFlags(flags)
		if err := parseFlags(flags, args[2:], 1); err != nil {
			return err
		}
		cfg, path, resolved, err := resolveTarget(*configPath, *targetName)
		if err != nil {
			return err
		}
		id := flags.Arg(0)
		if _, err := runRemoteAdmin(ctx, remote.OSExecutor{}, resolved.Target, "key", "revoke", "--data-root", resolved.Target.DataRoot, "--json", resolved.Target.Namespace, id); err != nil {
			return err
		}
		if keyID(resolved.Target.Key) == id {
			updated := resolved.Target
			updated.Key = ""
			cfg.Targets[resolved.Name] = updated
			if err := cfg.Save(path); err != nil {
				return err
			}
		}
		_, err = fmt.Fprintf(stdout, "revoked remote key %s\n", id)
		return err
	default:
		return &usageError{message: fmt.Sprintf("unknown remote command %q", strings.Join(args[:2], " "))}
	}
}

func runClientExec(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("exec", stderr)
	configPath := flags.String("config", "", "target configuration path")
	targetName := flags.String("target", "", "target name")
	flags.StringVar(targetName, "t", "", "target name")
	timeout := flags.Duration("timeout", 30*time.Second, "HTTP request timeout")
	if err := flags.Parse(args); err != nil {
		return &usageError{message: err.Error()}
	}
	commandArgs := flags.Args()
	if len(commandArgs) < 2 {
		return &usageError{message: "exec requires contextq command arguments"}
	}
	_, _, resolved, err := resolveTarget(*configPath, *targetName)
	if err != nil {
		return err
	}
	if resolved.Target.Key == "" {
		return fmt.Errorf("target %q has no key; run remote key add", resolved.Name)
	}
	body, err := json.Marshal(map[string]any{"args": commandArgs})
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, resolved.Target.URL+"/v1/"+resolved.Target.Namespace+"/exec", bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+resolved.Target.Key)
	request.Header.Set("Content-Type", "application/json")
	response, err := newHTTPClient(*timeout).Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	if err != nil {
		return err
	}
	if response.StatusCode >= 200 && response.StatusCode < 300 {
		_, err = stdout.Write(responseBody)
		return err
	}
	_, _ = stderr.Write(responseBody)
	return fmt.Errorf("remote contextq command failed with HTTP %d", response.StatusCode)
}

func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("doctor", stderr)
	configPath := flags.String("config", "", "target configuration path")
	targetName := flags.String("target", "", "target name")
	flags.StringVar(targetName, "t", "", "target name")
	if err := parseFlags(flags, args, 0); err != nil {
		return err
	}
	_, _, resolved, err := resolveTarget(*configPath, *targetName)
	if err != nil {
		return err
	}
	executor := remote.OSExecutor{}
	sshSession, err := remote.NewSession(resolved.Target)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = sshSession.Close(closeCtx, executor)
	}()
	sshTarget := sshSession.Target()
	failures := 0
	check := func(name string, err error, detail string) {
		if err != nil {
			failures++
			fmt.Fprintf(stdout, "FAIL\t%s\t%v\n", name, err)
			return
		}
		fmt.Fprintf(stdout, "OK\t%s\t%s\n", name, detail)
	}
	client := newHTTPClient(10 * time.Second)
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, resolved.Target.URL+"/healthz", nil)
	response, httpErr := client.Do(request)
	if httpErr == nil {
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			httpErr = fmt.Errorf("HTTP %d", response.StatusCode)
		}
	}
	check("HTTPS health", httpErr, resolved.Target.URL+"/healthz")
	sshResult, sshErr := remote.RunSSH(ctx, executor, sshTarget, "true", nil)
	if sshErr != nil && len(sshResult.Stderr) > 0 {
		sshErr = fmt.Errorf("%w: %s", sshErr, bytes.TrimSpace(sshResult.Stderr))
	}
	check("SSH", sshErr, resolved.Target.SSHUser+"@"+resolved.Target.SSHHost)
	serviceCommand := remote.ShellCommand("systemctl", "is-active", "--quiet", setup.ServiceName)
	_, serviceErr := remote.RunSSH(ctx, executor, sshTarget, serviceCommand, nil)
	check("systemd service", serviceErr, setup.ServiceName+" active")
	filesCommand := strings.Join([]string{
		remote.ShellCommand("test", "-x", resolved.Target.RemoteBin),
		remote.ShellCommand("test", "-x", resolved.Target.ContextqBin),
		remote.ShellCommand("test", "-d", resolved.Target.DataRoot),
	}, " && ")
	_, filesErr := remote.RunSSH(ctx, executor, sshTarget, filesCommand, nil)
	check("remote files", filesErr, "binaries and data root present")
	caddyCommand := remote.ShellCommand("caddy", "validate", "--config", resolved.Target.Caddyfile)
	_, caddyErr := remote.RunSSH(ctx, executor, sshTarget, caddyCommand, nil)
	check("Caddy config", caddyErr, resolved.Target.Caddyfile+" valid")
	if resolved.Target.Key == "" {
		check("namespace key", errors.New("no key saved locally"), "")
	} else {
		body, _ := json.Marshal(map[string]any{"args": []string{"queue", "list"}})
		request, requestErr := http.NewRequestWithContext(ctx, http.MethodPost, resolved.Target.URL+"/v1/"+resolved.Target.Namespace+"/exec", bytes.NewReader(body))
		if requestErr == nil {
			request.Header.Set("Authorization", "Bearer "+resolved.Target.Key)
			request.Header.Set("Content-Type", "application/json")
			response, rpcErr := client.Do(request)
			if rpcErr == nil {
				defer response.Body.Close()
				if response.StatusCode != http.StatusOK {
					rpcErr = fmt.Errorf("HTTP %d", response.StatusCode)
				}
			}
			requestErr = rpcErr
		}
		check("authenticated RPC", requestErr, resolved.Target.Namespace+" accessible")
	}
	if failures > 0 {
		return fmt.Errorf("doctor found %d failing check(s)", failures)
	}
	return nil
}

func remoteTargetFlags(flags interface {
	String(string, string, string) *string
	StringVar(*string, string, string, string)
}) (*string, *string) {
	configPath := flags.String("config", "", "target configuration path")
	targetName := flags.String("target", "", "target name")
	flags.StringVar(targetName, "t", "", "target name")
	return configPath, targetName
}

func loadTargets(configPath string) (*target.Config, string, error) {
	path := configPath
	if path == "" {
		var err error
		path, err = target.DefaultPath()
		if err != nil {
			return nil, "", err
		}
	}
	cfg, err := target.Load(path)
	return cfg, path, err
}

func resolveTarget(configPath, name string) (*target.Config, string, target.Named, error) {
	cfg, path, err := loadTargets(configPath)
	if err != nil {
		return nil, "", target.Named{}, err
	}
	resolved, err := cfg.Resolve(name)
	return cfg, path, resolved, err
}

func remoteNamespaceInit(ctx context.Context, executor remote.Executor, t target.Target) error {
	_, err := runRemoteAdmin(ctx, executor, t, "namespace", "init", "--data-root", t.DataRoot, "--json", t.Namespace)
	return err
}

func remoteKeyAdd(ctx context.Context, executor remote.Executor, t target.Target, label string) (namespace.IssuedKey, error) {
	result, err := runRemoteAdmin(ctx, executor, t, "key", "add", "--data-root", t.DataRoot, "--label", label, "--json", t.Namespace)
	if err != nil {
		return namespace.IssuedKey{}, err
	}
	var issued namespace.IssuedKey
	if err := json.Unmarshal(result.Stdout, &issued); err != nil {
		return namespace.IssuedKey{}, fmt.Errorf("parse issued remote key: %w", err)
	}
	return issued, nil
}

func runRemoteAdmin(ctx context.Context, executor remote.Executor, t target.Target, args ...string) (remote.Result, error) {
	commandArgs := []string{"runuser", "-u", t.ServiceUser, "--", t.RemoteBin}
	commandArgs = append(commandArgs, args...)
	result, err := remote.RunSSH(ctx, executor, t, remote.ShellCommand(commandArgs...), nil)
	if err != nil {
		return result, fmt.Errorf("remote administration failed: %w: %s", err, bytes.TrimSpace(result.Stderr))
	}
	return result, nil
}

func randomStageDir() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "/tmp/contextq-install-" + hex.EncodeToString(b), nil
}

func defaultKeyLabel() string {
	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		return "contextq-client"
	}
	return hostname
}

func keyID(token string) string {
	parts := strings.SplitN(token, "_", 4)
	if len(parts) != 4 || parts[0] != "cqk" || parts[1] != "k" {
		return ""
	}
	return "k_" + parts[2]
}

func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: clientTransport,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
}

func detectRemoteArch(ctx context.Context, executor remote.Executor, t target.Target) (string, error) {
	result, err := remote.RunSSH(ctx, executor, t, "uname -m", nil)
	if err != nil {
		return "", fmt.Errorf("detect remote architecture: %w: %s", err, bytes.TrimSpace(result.Stderr))
	}
	switch strings.TrimSpace(string(result.Stdout)) {
	case "x86_64", "amd64":
		return "amd64", nil
	case "aarch64", "arm64":
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported remote architecture %q", strings.TrimSpace(string(result.Stdout)))
	}
}

func resolveBootstrapBinaries(arch, serverPath, contextqPath string) (string, string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", "", err
	}
	executable, err = filepath.Abs(executable)
	if err != nil {
		return "", "", err
	}
	controllerDir := filepath.Dir(executable)
	if serverPath == "" {
		serverPath = firstMatchingBinary(arch, []string{
			filepath.Join(controllerDir, "linux-"+arch, "contextq-server"),
			filepath.Join(controllerDir, "..", "dist", "linux-"+arch, "contextq-server"),
			executable,
		})
		if serverPath == "" {
			return "", "", fmt.Errorf("no linux/%s contextq-server artifact found; run make release TARGET_GOARCH=%s or pass --server-binary", arch, arch)
		}
	}
	serverPath, err = filepath.Abs(serverPath)
	if err != nil {
		return "", "", err
	}
	if contextqPath == "" {
		candidates := []string{
			filepath.Join(filepath.Dir(serverPath), "contextq"),
			filepath.Join(controllerDir, "linux-"+arch, "contextq"),
			filepath.Join(controllerDir, "..", "dist", "linux-"+arch, "contextq"),
		}
		if path, lookupErr := exec.LookPath("contextq"); lookupErr == nil {
			candidates = append(candidates, path)
		}
		contextqPath = firstMatchingBinary(arch, candidates)
		if contextqPath == "" {
			return "", "", fmt.Errorf("no linux/%s contextq artifact found; run make release TARGET_GOARCH=%s or pass --contextq-binary", arch, arch)
		}
	}
	contextqPath, err = filepath.Abs(contextqPath)
	if err != nil {
		return "", "", err
	}
	for name, path := range map[string]string{"contextq-server": serverPath, "contextq": contextqPath} {
		binaryArch, err := linuxBinaryArch(path)
		if err != nil {
			return "", "", fmt.Errorf("inspect %s binary %q: %w", name, path, err)
		}
		if binaryArch != arch {
			return "", "", fmt.Errorf("%s binary %q targets linux/%s, but server is linux/%s", name, path, binaryArch, arch)
		}
	}
	return serverPath, contextqPath, nil
}

func firstMatchingBinary(arch string, candidates []string) string {
	for _, candidate := range candidates {
		if binaryArch, err := linuxBinaryArch(candidate); err == nil && binaryArch == arch {
			return candidate
		}
	}
	return ""
}

func linuxBinaryArch(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	header := make([]byte, 20)
	if _, err := io.ReadFull(f, header); err != nil {
		return "", err
	}
	if !bytes.Equal(header[:4], []byte{0x7f, 'E', 'L', 'F'}) {
		return "", fmt.Errorf("not an ELF executable")
	}
	var order binary.ByteOrder
	switch header[5] {
	case 1:
		order = binary.LittleEndian
	case 2:
		order = binary.BigEndian
	default:
		return "", fmt.Errorf("invalid ELF byte order")
	}
	switch order.Uint16(header[18:20]) {
	case 62:
		return "amd64", nil
	case 183:
		return "arm64", nil
	default:
		return "", fmt.Errorf("unsupported ELF machine")
	}
}
