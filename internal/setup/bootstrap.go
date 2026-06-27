package setup

import (
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/norlinga/contextq-server/internal/target"
)

const ServiceName = "contextq-server.service"

var targetNamePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,62}$`)

type BootstrapOptions struct {
	Target          target.Target
	TargetName      string
	Listen          string
	StagedServer    string
	StagedContextq  string
	SystemdUnitPath string
}

func BootstrapScript(options BootstrapOptions) (string, error) {
	t := options.Target
	if err := t.Validate(); err != nil {
		return "", err
	}
	if !targetNamePattern.MatchString(options.TargetName) {
		return "", fmt.Errorf("invalid target name %q", options.TargetName)
	}
	if options.Listen == "" {
		options.Listen = "127.0.0.1:8787"
	}
	listenHost, listenPort, err := net.SplitHostPort(options.Listen)
	if err != nil {
		return "", fmt.Errorf("invalid listen address %q", options.Listen)
	}
	port, err := strconv.Atoi(listenPort)
	ip := net.ParseIP(listenHost)
	if err != nil || port < 1 || port > 65535 || (listenHost != "localhost" && (ip == nil || !ip.IsLoopback())) {
		return "", fmt.Errorf("listen address must use a loopback host and valid port")
	}
	if options.StagedServer == "" {
		options.StagedServer = "/tmp/contextq-install/contextq-server"
	}
	if options.StagedContextq == "" {
		options.StagedContextq = "/tmp/contextq-install/contextq"
	}
	if options.SystemdUnitPath == "" {
		options.SystemdUnitPath = "/etc/systemd/system/" + ServiceName
	}
	for name, value := range map[string]string{
		"staged server":   options.StagedServer,
		"staged contextq": options.StagedContextq,
		"systemd unit":    options.SystemdUnitPath,
	} {
		if !filepath.IsAbs(value) || strings.ContainsAny(value, "\r\n\x00") {
			return "", fmt.Errorf("%s path is invalid", name)
		}
	}
	u, _ := url.Parse(t.URL)
	site := u.Host
	snippetPath := filepath.Join(t.SnippetDir, options.TargetName+".caddy")
	importGlob := filepath.Join(t.SnippetDir, "*.caddy")

	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	b.WriteString("# contextq-server remote-init — idempotent host bootstrap\n")
	b.WriteString("set -eu\n\n")
	fmt.Fprintf(&b, "SERVICE_USER=%s\n", shellQuote(t.ServiceUser))
	fmt.Fprintf(&b, "DATA_ROOT=%s\n", shellQuote(t.DataRoot))
	fmt.Fprintf(&b, "SERVER_SOURCE=%s\n", shellQuote(options.StagedServer))
	fmt.Fprintf(&b, "CONTEXTQ_SOURCE=%s\n", shellQuote(options.StagedContextq))
	fmt.Fprintf(&b, "SERVER_BIN=%s\n", shellQuote(t.RemoteBin))
	fmt.Fprintf(&b, "CONTEXTQ_BIN=%s\n", shellQuote(t.ContextqBin))
	fmt.Fprintf(&b, "UNIT_PATH=%s\n", shellQuote(options.SystemdUnitPath))
	fmt.Fprintf(&b, "CADDYFILE=%s\n", shellQuote(t.Caddyfile))
	fmt.Fprintf(&b, "SNIPPET_DIR=%s\n\n", shellQuote(t.SnippetDir))
	fmt.Fprintf(&b, "SNIPPET_PATH=%s\n\n", shellQuote(snippetPath))
	if filepath.Dir(options.StagedServer) == filepath.Dir(options.StagedContextq) {
		fmt.Fprintf(&b, "STAGE_DIR=%s\n", shellQuote(filepath.Dir(options.StagedServer)))
		b.WriteString("trap 'rm -rf \"$STAGE_DIR\"' EXIT\n\n")
	}

	b.WriteString("[ \"$(id -u)\" -eq 0 ] || { echo 'remote-init must run as root' >&2; exit 1; }\n")
	b.WriteString("for command in getent groupadd useradd install runuser systemctl caddy; do\n")
	b.WriteString("  command -v \"$command\" >/dev/null 2>&1 || { echo \"missing required command: $command\" >&2; exit 1; }\n")
	b.WriteString("done\n")
	b.WriteString("[ -f \"$SERVER_SOURCE\" ] || { echo \"missing staged binary: $SERVER_SOURCE\" >&2; exit 1; }\n")
	b.WriteString("[ -f \"$CONTEXTQ_SOURCE\" ] || { echo \"missing staged binary: $CONTEXTQ_SOURCE\" >&2; exit 1; }\n\n")

	b.WriteString("getent group \"$SERVICE_USER\" >/dev/null 2>&1 || groupadd --system \"$SERVICE_USER\"\n")
	b.WriteString("id \"$SERVICE_USER\" >/dev/null 2>&1 || useradd --system --gid \"$SERVICE_USER\" --home-dir \"$DATA_ROOT\" --no-create-home --shell /usr/sbin/nologin \"$SERVICE_USER\"\n")
	b.WriteString("install -d -m 0750 -o \"$SERVICE_USER\" -g \"$SERVICE_USER\" \"$DATA_ROOT\"\n\n")

	b.WriteString("install -D -m 0755 \"$SERVER_SOURCE\" \"$SERVER_BIN.new\"\n")
	b.WriteString("install -D -m 0755 \"$CONTEXTQ_SOURCE\" \"$CONTEXTQ_BIN.new\"\n")
	b.WriteString("mv -f \"$SERVER_BIN.new\" \"$SERVER_BIN\"\n")
	b.WriteString("mv -f \"$CONTEXTQ_BIN.new\" \"$CONTEXTQ_BIN\"\n\n")

	fmt.Fprintf(&b, "cat > \"$UNIT_PATH.new\" <<'SYSTEMD'\n[Unit]\nDescription=contextq HTTP command server\nAfter=network-online.target\nWants=network-online.target\n\n[Service]\nType=simple\nUser=%s\nGroup=%s\nWorkingDirectory=%s\nExecStart=%s serve --listen %s --data-root %s --contextq-bin %s\nRestart=on-failure\nRestartSec=2s\nUMask=0027\nNoNewPrivileges=true\nPrivateTmp=true\nProtectHome=true\nProtectSystem=strict\nReadWritePaths=%s\n\n[Install]\nWantedBy=multi-user.target\nSYSTEMD\nmv -f \"$UNIT_PATH.new\" \"$UNIT_PATH\"\n\n",
		t.ServiceUser, t.ServiceUser, t.DataRoot, t.RemoteBin, options.Listen, t.DataRoot, t.ContextqBin, t.DataRoot)

	b.WriteString("install -d -m 0755 \"$SNIPPET_DIR\"\n")
	fmt.Fprintf(&b, "cat > \"$SNIPPET_PATH.new\" <<'CADDY'\n%s {\n\treverse_proxy %s\n}\nCADDY\nmv -f \"$SNIPPET_PATH.new\" \"$SNIPPET_PATH\"\n", site, options.Listen)
	fmt.Fprintf(&b, "IMPORT_LINE=%s\n", shellQuote("import "+importGlob))
	b.WriteString("grep -qF \"$IMPORT_LINE\" \"$CADDYFILE\" 2>/dev/null || printf '\\n%s\\n' \"$IMPORT_LINE\" >> \"$CADDYFILE\"\n\n")

	b.WriteString("systemctl daemon-reload\n")
	b.WriteString("systemctl enable contextq-server.service >/dev/null\n")
	b.WriteString("systemctl restart contextq-server.service\n")
	b.WriteString("if ! CADDY_OUTPUT=\"$(caddy validate --config \"$CADDYFILE\" 2>&1)\"; then\n")
	b.WriteString("  printf '%s\\n' \"$CADDY_OUTPUT\" >&2\n")
	b.WriteString("  exit 1\n")
	b.WriteString("fi\n")
	b.WriteString("systemctl reload caddy\n")
	b.WriteString("systemctl is-active --quiet contextq-server.service\n")
	b.WriteString("echo 'contextq-server bootstrap complete'\n")
	return b.String(), nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
