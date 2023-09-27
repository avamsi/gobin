package main

import (
	"context"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/avamsi/ergo/pair"
)

type hedgingClient struct {
	delegate http.Client
}

func newHedgingClient() *hedgingClient {
	return &hedgingClient{http.Client{Timeout: time.Minute}}
}

func (client *hedgingClient) get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		resp = make(chan pair.Pair[[]byte, error])
		get  = func() {
			resp <- pair.New(client.getUnhedged(ctx, url))
		}
	)
	go get() // first request
	select {
	case r := <-resp:
		return r.Unpack()
	case <-time.After(time.Second):
		go get() // second request (to hedge the first)
	}
	// Use the first response that comes back and cancel the other request
	// (implicitly, via the deferred cancel on ctx at the beginning).
	return (<-resp).Unpack()
}

func (client *hedgingClient) getUnhedged(ctx context.Context, url string) ([]byte, error) {
	resp, err := client.delegate.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return io.ReadAll(resp.Body)
	case 404:
		return nil, nil
	default:
		return nil, errors.New(resp.Status)
	}
}
