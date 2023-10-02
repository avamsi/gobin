package main

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "embed"

	"github.com/avamsi/climate"
	"github.com/avamsi/ergo/assert"
	"github.com/erikgeiser/promptkit"
	"github.com/erikgeiser/promptkit/selection"
)

// gobin is a Go package manager.
type gobin struct{}

func searchSuffix(ctx context.Context, q string) ([]string, error) {
	pkgs, err := search(ctx, q)
	if err != nil {
		return nil, err
	}
	i := 0
	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.Name, q) {
			pkgs[i] = pkg
			i++
		}
	}
	pkgs = pkgs[:i]
	if len(pkgs) == 0 {
		return nil, fmt.Errorf("no packages found for %q", q)
	}
	out := make([]string, len(pkgs))
	for i, pkg := range pkgs {
		out[i] = pkg.Name + "@" + pkg.DefaultVersion
	}
	return out, nil
}

// Search for packages with the given name (suffix matched).
func (*gobin) Search(ctx context.Context, name string) error {
	pkgs, err := searchSuffix(ctx, name)
	if err != nil {
		return err
	}
	for _, pkg := range pkgs {
		fmt.Println(pkg)
	}
	return nil
}

func install(ctx context.Context, pkg string) error {
	cmd := exec.CommandContext(ctx, "go", "install", pkg)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	fmt.Println("$", cmd.String())
	return cmd.Run()
}

// Install the package with the given name (suffix matched).
func (*gobin) Install(ctx context.Context, name string) error {
	pkgs, err := searchSuffix(ctx, name)
	if err != nil {
		return err
	}
	pkg := pkgs[0]
	if len(pkgs) > 1 {
		sp := selection.New("", pkgs)
		sp.FilterPlaceholder = ""
		sp.ResultTemplate = ""
		pkg, err = sp.RunPrompt()
		if errors.Is(err, promptkit.ErrAborted) {
			return climate.ErrExit(130)
		}
		assert.Nil(err)
	}
	return install(ctx, pkg)
}

func path() string {
	if p, ok := os.LookupEnv("GOBIN"); ok {
		return p
	}
	if p, ok := os.LookupEnv("GOPATH"); ok {
		return filepath.Join(p, "bin")
	}
	return filepath.Join(assert.Ok(os.UserHomeDir()), "go", "bin")
}

// List all installed packages.
func (*gobin) List(ctx context.Context) {
	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := buildinfo.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Skipping %s: %v\n", path, err)
		}
		// TODO: maybe we could also print any updates are available from
		// https://docs.deps.dev/api/v3alpha/#getpackage?
		fmt.Println(info.Path + "@" + info.Main.Version)
		return nil
	}
	assert.Nil(filepath.WalkDir(path(), walk))
}

// Uninstall the package with the given name.
//
//cli:aliases remove, rm
func (*gobin) Uninstall(name string) error {
	return os.Remove(filepath.Join(path(), name))
}

//go:generate go run github.com/avamsi/climate/cmd/climate --out=md.cli
//go:embed md.cli
var md []byte

func main() {
	climate.RunAndExit(climate.Struct[gobin](), climate.WithMetadata(md))
}
