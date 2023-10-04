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

func searchSuffix(ctx context.Context, q string) ([]pkge, error) {
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
	out := make([]pkge, len(pkgs))
	for i, pkg := range pkgs {
		out[i] = pkge{pkg.Name, pkg.DefaultVersion}
	}
	return out, nil
}

func (*gobin) path() string {
	if v, ok := os.LookupEnv("GOBIN"); ok {
		return v
	}
	if v, ok := os.LookupEnv("GOPATH"); ok {
		return filepath.Join(v, "bin")
	}
	return filepath.Join(assert.Ok(os.UserHomeDir()), "go", "bin")
}

// Search for packages with the given name (suffix matched).
func (gb *gobin) Search(ctx context.Context, name string) error {
	pkgs, err := searchSuffix(ctx, name)
	if err != nil {
		return err
	}
	in := newInstalled(gb.path())
	for _, pkg := range pkgs {
		switch v := in.version(pkg); v {
		case "":
			fmt.Println(pkg)
		case pkg.version:
			fmt.Printf("%s (already installed)\n", pkg)
		default:
			fmt.Printf("%s (already installed: %s)\n", pkg, v)
		}
	}
	return nil
}

func install(ctx context.Context, pkg pkge) error {
	// #nosec G204 -- G204 doesn't like pkg.String here, but it should be fine
	// as we still own that type (and its content sources).
	cmd := exec.CommandContext(ctx, "go", "install", pkg.String())
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

// List all installed packages.
func (gb *gobin) List(ctx context.Context) {
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
		fmt.Println(pkge{info.Path, info.Main.Version})
		return nil
	}
	assert.Nil(filepath.WalkDir(gb.path(), walk))
}

// Uninstall the package with the given name.
//
//cli:aliases remove, rm
func (gb *gobin) Uninstall(name string) error {
	return os.Remove(filepath.Join(gb.path(), name))
}

//go:generate go run github.com/avamsi/climate/cmd/climate --out=md.cli
//go:embed md.cli
var md []byte

func main() {
	climate.RunAndExit(climate.Struct[gobin](), climate.WithMetadata(md))
}
