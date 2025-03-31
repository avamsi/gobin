package repo

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/avamsi/ergo/assert"
	"github.com/avamsi/ergo/errors"
	"github.com/avamsi/ergo/group"
	"github.com/avamsi/ergo/pair"
	"golang.org/x/net/html"
)

type Pkgsite struct {
	client  Client
	baseURL string
	limit   int
}

func NewPkgsite(client Client, baseURL string, limit int) *Pkgsite {
	return &Pkgsite{client, baseURL, limit}
}

func (p *Pkgsite) searchURL(q string) string {
	return fmt.Sprintf(
		"%s/search?limit=%d&m=package&q=%s", p.baseURL, p.limit, url.QueryEscape(q))
}

func (p *Pkgsite) doc(ctx context.Context, url string) (_ *html.Node, err error) {
	resp, err := p.client.Get(ctx, url)
	if err != nil {
		return nil, err
	}
	defer errors.Handlef(&err, "doc(%q)", url)
	return html.Parse(bytes.NewReader(resp))
}

var (
	selAnchorElement       = assert.Ok(cascadia.Parse("a"))
	selPackageHeader       = assert.Ok(cascadia.Parse("div.go-Main-headerContent"))
	selPackageHeaderDetail = assert.Ok(cascadia.Parse("span.go-Main-headerDetailItem"))
	selPackageHeaderTitle  = assert.Ok(cascadia.Parse("div.go-Main-headerTitle"))
	selSearchSnippet       = assert.Ok(cascadia.Parse("div.SearchSnippet"))
	selSearchSnippetHeader = assert.Ok(cascadia.Parse("div.SearchSnippet-headerContainer"))
	selSearchSnippetInfo   = assert.Ok(cascadia.Parse("div.SearchSnippet-infoLabel"))
	selSearchSubSnippet    = assert.Ok(cascadia.Parse("div.SearchSnippet-sub"))
	selSpanElement         = assert.Ok(cascadia.Parse("span"))
	selStrongElement       = assert.Ok(cascadia.Parse("strong"))
)

func containsCommandChip(n *html.Node) bool {
	for _, span := range cascadia.QueryAll(n, selSpanElement) {
		if span.FirstChild.Data == "command" {
			return true
		}
	}
	return false
}

func href(a *html.Node) string {
	for _, attr := range a.Attr {
		if attr.Key == "href" {
			return attr.Val
		}
	}
	panic(fmt.Sprint("no href attribute in", a.Attr))
}

func (p *Pkgsite) Lookup(ctx context.Context, pkgPath string) (_ Pkg, err error) {
	defer errors.Handlef(&err, "Pkgsite.Lookup(%q)", pkgPath)
	doc, err := p.doc(ctx, p.baseURL+"/"+pkgPath)
	if err != nil {
		return Pkg{Path: pkgPath}, err
	}
	header := cascadia.Query(doc, selPackageHeader)
	if !containsCommandChip(cascadia.Query(header, selPackageHeaderTitle)) {
		return Pkg{}, nil
	}
	var (
		version = cascadia.Query(header, selPackageHeaderDetail)
		a       = version.FirstChild.NextSibling
	)
	return Pkg{pkgPath, strings.TrimSpace(a.LastChild.Data)}, nil
}

type rank = <-chan pair.Pair[Pkg, error]

func (p *Pkgsite) parseSnippet(ctx context.Context, q string, snippet *html.Node) rank {
	var (
		// There's only at most 5 packages in any subsection, which is really
		// unfortunate as we could be missing out on some commands.
		c           = group.NewCollector(make(chan pair.Pair[Pkg, error], 5))
		subSnippets = cascadia.Query(snippet, selSearchSubSnippet)
	)
	if subSnippets != nil {
		for _, a := range cascadia.QueryAll(subSnippets, selAnchorElement) {
			c.Go(func() {
				// These seem to be of the form "/$pkgPath".
				pkgPath := href(a)[1:]
				if !strings.HasSuffix(pkgPath, q) {
					return
				}
				c.Collect(pair.New(p.Lookup(ctx, pkgPath)))
			})
		}
	}
	header := cascadia.Query(snippet, selSearchSnippetHeader)
	if containsCommandChip(header) {
		// Again, these seem to be of the form "/$pkgPath".
		pkgPath := href(cascadia.Query(header, selAnchorElement))[1:]
		if !strings.HasSuffix(pkgPath, q) {
			return c.Close()
		}
		info := cascadia.Query(snippet, selSearchSnippetInfo)
		// These are dependents, version and published date (in that order).
		version := cascadia.QueryAll(info, selStrongElement)[1]
		c.Collect(pair.New[Pkg, error](Pkg{pkgPath, version.FirstChild.Data}, nil))
	}
	return c.Close()
}

func (p *Pkgsite) parseDoc(ctx context.Context, q string, doc *html.Node) []rank {
	var (
		snippets = cascadia.QueryAll(doc, selSearchSnippet)
		ranks    = make([]rank, len(snippets))
	)
	for i, s := range snippets {
		ranks[i] = p.parseSnippet(ctx, q, s)
	}
	return ranks
}

func (p *Pkgsite) Search(ctx context.Context, q string) (_ []Pkg, err error) {
	defer errors.Handlef(&err, "Pkgsite.Search(%q)", q)
	doc, err := p.doc(ctx, p.searchURL(q))
	if err != nil {
		return nil, err
	}
	// Pkgsite is of course not designed to be consumed programmatically, so
	// this is a bit of a mess. First things first, we can only search for
	// packages and not commands specifically, so we do the filtering ourselves
	// (using the "command" chip in the header). Second, results (or "snippets")
	// under a module are clubbed under a subsection, so we parse those as well.
	// These are not decorated with the "command" chip and the version
	// information, so we hit their respective package pages.
	var (
		pkgs []Pkg
		merr error
	)
	for _, rank := range p.parseDoc(ctx, q, doc) {
		for res := range rank {
			pkg, err := res.Unpack()
			if pkg.Path != "" {
				pkgs = append(pkgs, pkg)
			}
			if err != nil {
				merr = errors.Join(merr, err)
			}
		}
	}
	return pkgs, merr
}
