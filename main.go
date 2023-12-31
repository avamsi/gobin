package main

import (
	"context"
	"debug/buildinfo"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	_ "embed"

	"github.com/avamsi/climate"
	"github.com/avamsi/ergo/assert"
	"github.com/avamsi/ergo/group"
	"github.com/erikgeiser/promptkit"
	"github.com/erikgeiser/promptkit/selection"
	"golang.org/x/sync/errgroup"
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
			fmt.Printf("%s (installed: %s)\n", pkg, v)
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

func readdirnames(d string) ([]string, error) {
	f, err := os.Open(d)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return f.Readdirnames(-1)
}

const versionsURL = "https://api.deps.dev/v3alpha/systems/GO/packages/"

type versionsResponse struct {
	PackageKey struct {
		System string
		Name   string
	}
	Versions []struct {
		VersionKey struct {
			System  string
			Name    string
			Version string
		}
		IsDefault bool
	}
}

func availableVersion(ctx context.Context, pkgPath string) string {
	var (
		url    = versionsURL + url.PathEscape(pkgPath)
		b, err = defaultClient.get(ctx, url)
	)
	if err != nil {
		return ""
	}
	resp := jsonUnmarshal[versionsResponse](b)
	// TODO: compare versions semantically (and return the latest).
	for _, v := range resp.Versions {
		if v.IsDefault {
			return v.VersionKey.Version
		}
	}
	return ""
}

// List all installed packages.
func (gb *gobin) List(ctx context.Context) error {
	var (
		gobin      = gb.path()
		names, err = readdirnames(gobin)
	)
	if err != nil {
		return err
	}
	var (
		g      errgroup.Group
		stdout = group.NewWriter(os.Stdout, len(names))
		stderr = group.NewWriter(os.Stderr, len(names))
	)
	for i, name := range names {
		var (
			path   = filepath.Join(gobin, name)
			stdout = stdout.Section(i)
			stderr = stderr.Section(i)
		)
		g.Go(func() error {
			defer stdout.Close()
			defer stderr.Close()
			info, err := buildinfo.ReadFile(path)
			if err != nil {
				fmt.Fprintf(stderr, "Skipping %s: %v\n", path, err)
			}
			pkg := pkge{info.Path, info.Main.Version}
			switch v := availableVersion(ctx, pkg.path); v {
			case "":
				fmt.Fprintln(stdout, pkg)
			case pkg.version:
				fmt.Fprintf(stdout, "%s (already up-to-date)\n", pkg)
			default:
				fmt.Fprintf(stdout, "%s (update available: %s)\n", pkg, v)
			}
			return nil
		})
	}
	return errors.Join(g.Wait(), stdout.Close(), stderr.Close())
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
