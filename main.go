package main

import (
	"context"
	"errors"
	"fmt"
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
	ergoerrors "github.com/avamsi/ergo/errors"
	"github.com/avamsi/gobin/internal/client"
	"github.com/avamsi/gobin/internal/repo"
	"github.com/erikgeiser/promptkit"
	"github.com/erikgeiser/promptkit/selection"
)

// gobin is a Go package manager.
type gobin struct{}

type repository interface {
	Lookup(ctx context.Context, pkgPath string) (pkg repo.Pkg, err error)
	Search(ctx context.Context, q string) (pkgs []repo.Pkg, err error)
}

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

	installed = sync.OnceValue(func() repository {
		return repo.NewInstalled(gobinDir())
	})

	defaultClient = client.Hedge(&http.Client{Timeout: time.Minute}, time.Second)

	depsdev repository = repo.NewDepsdev(defaultClient, 100)
	pkgsite repository = repo.NewPkgsite(defaultClient, "https://pkg.go.dev", 100)

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
		err = errNoPkgsFound
	}
	return pkgs[:i:i], err
}

// Search for packages with the given name (suffix matched).
func (gb *gobin) Search(ctx context.Context, name string) (err error) {
	defer ergoerrors.Annotatef(&err, "gobin.Search(%q)", name)
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
	if strings.Contains(pkg.Version, "...") {
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

// Install the package with the given name (suffix matched).
func (*gobin) Install(ctx context.Context, name string) (err error) {
	defer ergoerrors.Annotatef(&err, "gobin.Install(%q)", name)
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

// List all installed packages.
func (gb *gobin) List(ctx context.Context) (err error) {
	defer ergoerrors.Annotate(&err, "gobin.List")
	var (
		pkgs, merr = installed().Search(ctx, "")
		wg         sync.WaitGroup
		mutex      sync.Mutex
	)
	for _, pkg := range pkgs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			latestPkg, err := depsdev.Lookup(ctx, pkg.Path)
			mutex.Lock()
			defer mutex.Unlock()
			switch v := latestPkg.Version; v {
			case "":
				fmt.Println(pkg)
			case pkg.Version:
				fmt.Printf("%s (already up-to-date)\n", pkg)
			default:
				fmt.Printf("%s (update available: %s)\n", pkg, v)
			}
			merr = ergoerrors.Join(merr, err)
		}()
	}
	wg.Wait()
	return merr
}

// Uninstall the package with the given name.
//
//cli:aliases remove, rm
func (gb *gobin) Uninstall(name string) error {
	return os.Remove(filepath.Join(gobinDir(), name))
}

//go:generate go run github.com/avamsi/climate/cmd/cligen --out=md.cli
//go:embed md.cli
var md []byte

func main() {
	climate.RunAndExit(climate.Struct[gobin](), climate.WithMetadata(md))
}
