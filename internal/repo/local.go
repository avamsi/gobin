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

type Local struct {
	dir string
}

func NewLocal(dir string) *Local {
	return &Local{dir}
}

func (l *Local) lookup(name string) (Pkg, error) {
	info, err := buildinfo.ReadFile(filepath.Join(l.dir, name))
	if err != nil {
		return Pkg{}, err
	}
	return Pkg{info.Path, info.Main.Version}, nil
}

func ignore(err error, target error) error {
	if errors.Is(err, target) {
		return nil
	}
	return err
}

func (l *Local) Lookup(ctx context.Context, pkgPath string) (_ Pkg, e error) {
	defer ergoerrors.Handlef(&e, "Local.Lookup(%q)", pkgPath)
	if pkg, err := l.lookup(path.Base(pkgPath)); err == nil { // if _no_ error
		return pkg, nil
	} else {
		return Pkg{Path: pkgPath}, ignore(err, fs.ErrNotExist)
	}
}

func readdirnames(d string) ([]string, error) {
	if f, err := os.Open(d); err == nil { // if _no_ error
		return f.Readdirnames(-1)
	} else {
		return nil, ignore(err, fs.ErrNotExist)
	}
}

func (l *Local) Search(ctx context.Context, q string) (_ []Pkg, e error) {
	defer ergoerrors.Handlef(&e, "Local.Search(%q)", q)
	names, err := readdirnames(l.dir)
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
		if pkg, err := l.lookup(name); err == nil { // if _no_ error
			pkgs = append(pkgs, pkg)
		} else {
			merr = ergoerrors.Join(merr, err)
		}
	}
	return pkgs, merr
}
