package main

import (
	"debug/buildinfo"
	"path/filepath"
	"runtime/debug"
)

type installed struct {
	gobin string
	m     map[string]*debug.BuildInfo
}

func newInstalled(gobin string) *installed {
	return &installed{gobin, make(map[string]*debug.BuildInfo)}
}

func (in *installed) version(pkg pkge) string {
	var (
		name     = pkg.name()
		info, ok = in.m[name]
	)
	if !ok {
		// Ignore the error and so, possibly store nil if there's an error.
		info, _ = buildinfo.ReadFile(filepath.Join(in.gobin, name))
		in.m[name] = info
	}
	if info != nil && pkg.path == info.Path {
		return info.Main.Version
	}
	return ""
}
