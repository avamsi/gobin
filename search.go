package main

import (
	"context"
	"fmt"
	"net/url"

	"golang.org/x/sync/errgroup"
)

type searchResult struct {
	Kind           string
	Name           string
	System         string
	DefaultVersion string
}

func search(ctx context.Context, q string) ([]searchResult, error) {
	resps, err := searchParallelly(ctx, q)
	if err != nil {
		return nil, err
	}
	results := make([]searchResult, 0, resps[0].TotalMatches)
	for _, resp := range resps {
		results = append(results, resp.Results...)
	}
	return results, nil
}

type searchResponse struct {
	Page         int
	TotalMatches int
	Results      []searchResult
}

const searchURL = "https://deps.dev/_/search?q=%s&kind=PACKAGE&system=GO&page=%d&perPage=20"

func searchParallelly(ctx context.Context, q string) ([]searchResponse, error) {
	g, ctx := errgroup.WithContext(ctx)
	var (
		qEscaped = url.QueryEscape(q)
		n        = 5
		resps    = make([]searchResponse, n)
	)
	for i := 0; i < n; i++ {
		i := i // TODO: remove after Go 1.22.
		g.Go(func() error {
			var (
				url    = fmt.Sprintf(searchURL, qEscaped, i)
				b, err = defaultClient.get(ctx, url)
			)
			if err != nil {
				return err
			}
			resps[i] = jsonUnmarshal[searchResponse](b)
			return nil
		})
	}
	return resps, g.Wait()
}
