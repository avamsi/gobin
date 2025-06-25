package repo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strings"

	ergoerrors "github.com/avamsi/ergo/errors"
	"github.com/avamsi/gobin/internal/client"
)

type Depsdev struct {
	client Client
	limit  int
}

func NewDepsdev(client Client, limit int) *Depsdev {
	return &Depsdev{client, limit}
}

func (d *Depsdev) packageURL(pkgPath string) string {
	return "https://api.deps.dev/v3/systems/go/packages/" + url.PathEscape(pkgPath)
}

type packageResponse struct {
	PackageKey struct {
		System, Name string
	}
	Versions []struct {
		VersionKey struct {
			System        string
			Name, Version string
		}
		PublishedAt string
		IsDefault   bool
	}
}

func get[T any](ctx context.Context, client Client, url string, v *T) error {
	b, err := client.Get(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, v); err != nil {
		return fmt.Errorf("get(%q): %w", url, err)
	}
	return nil
}

var errNotFound = errors.New("not found")

func (d *Depsdev) lookup(ctx context.Context, pkgPath string) (Pkg, error) {
	var resp packageResponse
	if err := get(ctx, d.client, d.packageURL(pkgPath), &resp); err != nil {
		return Pkg{Path: pkgPath}, err
	}
	for _, v := range resp.Versions {
		if v.IsDefault {
			return Pkg{v.VersionKey.Name, v.VersionKey.Version}, nil
		}
	}
	return Pkg{Path: pkgPath}, errNotFound
}

func isHTTP404(err error) bool {
	var herr *client.HttpError
	return errors.As(err, &herr) && herr.StatusCode == http.StatusNotFound
}

func (d *Depsdev) Lookup(ctx context.Context, pkgPath string) (_ Pkg, e error) {
	defer ergoerrors.Handlef(&e, "Depsdev.Lookup(%q)", pkgPath)
	if pkg, err := d.lookup(ctx, pkgPath); !isHTTP404(err) {
		return pkg, err
	}
	for pp := path.Dir(pkgPath); pp != "."; pp = path.Dir(pp) {
		if pkg, err := d.lookup(ctx, pp); err == nil { // if _no_ error
			return pkg, nil
		}
	}
	return Pkg{Path: pkgPath}, errNotFound
}

func (d *Depsdev) searchURL(q string) string {
	return fmt.Sprintf(
		"https://deps.dev/_/search?q=%s&kind=PACKAGE&system=GO&page=0&perPage=%d",
		url.QueryEscape(q), d.limit)
}

type searchResponse struct {
	Page         int
	TotalMatches int
	Results      []struct {
		Kind, System         string
		Name, DefaultVersion string
	}
}

func (d *Depsdev) Search(ctx context.Context, q string) (_ []Pkg, e error) {
	defer ergoerrors.Handlef(&e, "Depsdev.Search(%q)", q)
	var resp searchResponse
	if err := get(ctx, d.client, d.searchURL(q), &resp); err != nil {
		return nil, err
	}
	pkgs := make([]Pkg, 0, len(resp.Results))
	for _, r := range resp.Results {
		if strings.HasSuffix(r.Name, q) {
			pkgs = append(pkgs, Pkg{r.Name, r.DefaultVersion})
		}
	}
	return pkgs, nil
}
