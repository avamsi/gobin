package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "embed"

	"github.com/avamsi/climate"
	"github.com/avamsi/ergo/assert"
	"github.com/avamsi/ergo/deref"
	ergoerrors "github.com/avamsi/ergo/errors"
	"github.com/avamsi/ergo/group"
	"github.com/avamsi/gobin/internal/client"
	"github.com/avamsi/gobin/internal/repo"
	"github.com/erikgeiser/promptkit"
	"github.com/erikgeiser/promptkit/selection"
)

// gobin is a Go package manager.
type gobin struct{}

var (
	gobinDir = sync.OnceValue(func() string {
		if v, ok := os.LookupEnv("GOBIN"); ok {
			return v
		}
		if v, ok := os.LookupEnv("GOPATH"); ok {
			return filepath.Join(v, "bin")
		}
		return filepath.Join(assert.Ok(os.UserHomeDir()), "go/bin")
	})

	installed = sync.OnceValue(func() *repo.Installed {
		return repo.NewInstalled(gobinDir())
	})

	defaultClient = client.Hedge(&http.Client{Timeout: time.Minute}, time.Second)

	depsdev = repo.NewDepsdev(defaultClient, 100)
	pkgsite = repo.NewPkgsite(defaultClient, "https://pkg.go.dev", 100)

	errNoPkgsFound = errors.New("no packages found")
)

func search(ctx context.Context, q string) ([]repo.Pkg, error) {
	var (
		pkgs, err = pkgsite.Search(ctx, q)
		i         = 0
	)
	for _, pkg := range pkgs {
		if strings.HasSuffix(pkg.Path, q) {
			pkgs[i] = pkg
			i++
		}
	}
	if i == 0 && err == nil { // if _no_ error
		return nil, errNoPkgsFound
	}
	return pkgs[:i:i], err
}

// Search for packages with the given name (suffix matched).
func (*gobin) Search(ctx context.Context, name string) (err error) {
	defer ergoerrors.Handlef(&err, "gobin.Search(%q)", name)
	pkgs, merr := search(ctx, name)
	for _, pkg := range pkgs {
		localPkg, err := installed().Lookup(ctx, pkg.Path)
		if err != nil {
			merr = ergoerrors.Join(merr, err)
		}
		switch v := localPkg.Version; v {
		case "":
			fmt.Println(pkg)
		case pkg.Version:
			fmt.Printf("%s (already installed)\n", pkg)
		default:
			fmt.Printf("%s (installed: %s)\n", pkg, v)
		}
	}
	return merr
}

func install(ctx context.Context, pkg repo.Pkg) error {
	// Pkgsite truncates long versions with "...", and those won't work with
	// `go install`, so we use "latest" instead.
	if pkg.Version == "" || strings.Contains(pkg.Version, "...") {
		pkg = pkg.Latest()
	}
	// #nosec G204 -- G204 doesn't like pkg.String here, but it should be fine
	// as we still own that type (and its content sources).
	cmd := exec.CommandContext(ctx, "go", "install", pkg.String())
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	fmt.Println("$", cmd.String())
	return cmd.Run()
}

// Install the package with the given name.
//
// Given name is suffix matched against package paths (via pkg.go.dev).
// If multiple matches are found, the user is prompted to select one.
//
//cli:aliases add
func (*gobin) Install(ctx context.Context, name string) (err error) {
	defer ergoerrors.Handlef(&err, "gobin.Install(%q)", name)
	pkgs, err := search(ctx, name)
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

// Update the installed package(s) with the given name.
//
// Given name is suffix matched against names of the installed packages.
// As such, if no name is given, all installed packages are updated.
//
//cli:aliases upgrade
func (*gobin) Update(ctx context.Context, name *string) (err error) {
	namestr := deref.Or(name, "")
	defer ergoerrors.Handlef(&err, "gobin.Update(%q)", namestr)
	pkgs, merr := installed().Search(ctx, namestr)
	for _, pkg := range pkgs {
		fmt.Println("📦", pkg.Name())
		latestPkg, err := depsdev.Lookup(ctx, pkg.Path)
		merr = ergoerrors.Join(merr, err)
		if pkg.Version != "" && pkg.Version == latestPkg.Version {
			fmt.Printf("%s (already up-to-date)\n", pkg)
			continue
		}
		merr = ergoerrors.Join(merr, install(ctx, latestPkg))
	}
	return merr
}

// Uninstall the package with the given name.
//
//cli:aliases remove, rm
func (*gobin) Uninstall(name string) error {
	return os.Remove(filepath.Join(gobinDir(), name))
}

// List all installed packages.
//
//cli:aliases ls
func (*gobin) List(ctx context.Context) (err error) {
	defer ergoerrors.Handle(&err, "gobin.List")
	c := group.NewCollector(make(chan error, 1))
	pkgs, err := installed().Search(ctx, "")
	c.Collect(err)
	stdout := log.New(os.Stdout, "", 0)
	for _, pkg := range pkgs {
		c.Go(func() {
			latestPkg, err := depsdev.Lookup(ctx, pkg.Path)
			c.Collect(err)
			switch v := latestPkg.Version; v {
			case "":
				stdout.Println(pkg)
			case pkg.Version:
				stdout.Printf("%s (already up-to-date)\n", pkg)
			default:
				stdout.Printf("%s (update available: %s)\n", pkg, v)
			}
		})
	}
	var merr error
	for err := range c.Close() {
		merr = ergoerrors.Join(merr, err)
	}
	return merr
}

//go:generate go tool cligen md.cli
//go:embed md.cli
var md []byte

func main() {
	climate.RunAndExit(climate.Struct[gobin](), climate.WithMetadata(md))
}
