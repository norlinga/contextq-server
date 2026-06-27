package setup

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/norlinga/contextq-server/internal/target"
)

func TestBootstrapScriptContainsServiceAndCaddyContract(t *testing.T) {
	script, err := BootstrapScript(BootstrapOptions{
		TargetName: "longship",
		Target: target.Target{
			URL:       "https://q.longship.dev",
			Namespace: "agents",
			SSHHost:   "longship.dev",
		},
		StagedServer:   "/tmp/stage/contextq-server",
		StagedContextq: "/tmp/stage/contextq",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		"User=contextq",
		"ExecStart=/usr/local/bin/contextq-server serve --listen 127.0.0.1:8787 --data-root /var/contextq --contextq-bin /usr/local/bin/contextq",
		"ReadWritePaths=/var/contextq",
		"q.longship.dev {",
		"reverse_proxy 127.0.0.1:8787",
		"import /etc/caddy/contextq.d/*.caddy",
		"caddy validate",
		"systemctl restart contextq-server.service",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("script missing %q", required)
		}
	}
	command := exec.Command("sh", "-n")
	command.Stdin = strings.NewReader(script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated script is not valid POSIX shell: %v: %s", err, output)
	}
}

func TestBootstrapRejectsPublicListener(t *testing.T) {
	_, err := BootstrapScript(BootstrapOptions{
		TargetName: "longship",
		Listen:     "0.0.0.0:8787",
		Target: target.Target{
			URL:       "https://q.longship.dev",
			Namespace: "agents",
			SSHHost:   "longship.dev",
		},
	})
	if err == nil {
		t.Fatal("expected public listener to be rejected")
	}
}
