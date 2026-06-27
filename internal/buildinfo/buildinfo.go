package buildinfo

import (
	"runtime"
	"runtime/debug"
)

var (
	Version         = "dev"
	Commit          = "unknown"
	BuildDate       = "unknown"
	ContextqVersion = "unknown"
)

type Info struct {
	Version         string `json:"version"`
	Commit          string `json:"commit"`
	BuildDate       string `json:"build_date"`
	GoVersion       string `json:"go_version"`
	ContextqVersion string `json:"contextq_version"`
}

func Current() Info {
	version := Version
	if version == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			version = info.Main.Version
		}
	}
	return Info{
		Version:         version,
		Commit:          Commit,
		BuildDate:       BuildDate,
		GoVersion:       runtime.Version(),
		ContextqVersion: ContextqVersion,
	}
}
