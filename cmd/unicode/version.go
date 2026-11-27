package main

import "runtime/debug"

// version is what unicode --version prints. make build and the release
// binaries set it with -ldflags "-X main.version=…"; otherwise it is the
// module's version that go build or go install recorded, or dev.
var version string

func versionOf() string {
	if version != "" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return "dev"
}

// versionReport is the JSON of unicode --version.
type versionReport struct {
	Version string `json:"version"`
}
