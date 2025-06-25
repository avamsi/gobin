package repo

import (
	"context"
	"debug/buildinfo"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	ergoerrors "github.com/avamsi/ergo/errors"
)

type Installed struct {
	gobin string
}

func NewInstalled(gobin string) *Installed {
	return &Installed{gobin}
}

func (i *Installed) lookup(name string) (Pkg, error) {
	info, err := buildinfo.ReadFile(filepath.Join(i.gobin, name))
	if err != nil {
		return Pkg{}, err
	}
	return Pkg{info.Path, info.Main.Version}, nil
}

func (i *Installed) Lookup(ctx context.Context, pkgPath string) (_ Pkg, err error) {
	defer ergoerrors.Handlef(&err, "Installed.Lookup(%q)", pkgPath)
	pkg, err := i.lookup(path.Base(pkgPath))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
		} else {
			pkg.Path = pkgPath
		}
	}
	return pkg, err
}

func readdirnames(d string) ([]string, error) {
	f, err := os.Open(d)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
		return nil, err
	}
	return f.Readdirnames(-1)
}

func (i *Installed) Search(ctx context.Context, q string) (_ []Pkg, err error) {
	defer ergoerrors.Handlef(&err, "Installed.Search(%q)", q)
	names, err := readdirnames(i.gobin)
	if err != nil {
		return nil, err
	}
	var (
		pkgs []Pkg
		merr error
	)
	for _, name := range names {
		if !strings.HasSuffix(name, q) {
			continue
		}
		if pkg, err := i.lookup(name); err == nil { // if _no_ error
			pkgs = append(pkgs, pkg)
		} else {
			merr = ergoerrors.Join(merr, err)
		}
	}
	return pkgs, merr
}
