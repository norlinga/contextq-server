package app

import (
	"fmt"
	"io"

	"github.com/norlinga/contextq-server/internal/buildinfo"
)

func runVersion(args []string, stdout, stderr io.Writer) error {
	flags := newFlagSet("version", stderr)
	asJSON := flags.Bool("json", false, "emit JSON")
	if err := parseFlags(flags, args, 0); err != nil {
		return err
	}
	info := buildinfo.Current()
	if *asJSON {
		return writeJSON(stdout, info)
	}
	_, err := fmt.Fprintf(stdout, "contextq-server %s\ncommit: %s\nbuilt: %s\ngo: %s\ncontextq: %s\n",
		info.Version, info.Commit, info.BuildDate, info.GoVersion, info.ContextqVersion)
	return err
}
