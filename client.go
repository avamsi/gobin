package main

import (
	"context"
	"io"
	"net/http"
	"time"

	"github.com/avamsi/ergo/pair"
)

type hedgingClient struct {
	delegate http.Client
	delay    time.Duration
}

var defaultClient = &hedgingClient{
	http.Client{Timeout: time.Minute},
	time.Second,
}

type statusError struct {
	code int
	s    string
}

func (serr *statusError) Error() string {
	return serr.s
}

func (client *hedgingClient) get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	var (
		resp = make(chan pair.Pair[[]byte, error])
		do   = func() {
			resp <- pair.New(client.doUnhedged(req.Clone(ctx)))
		}
	)
	go do() // first request
	select {
	case r := <-resp:
		return r.Unpack()
	case <-time.After(client.delay):
		go do() // second request (to hedge the first)
	}
	// Use the first response that comes back and cancel the other request
	// (implicitly, via the deferred cancel on ctx at the beginning).
	return (<-resp).Unpack()
}

func (client *hedgingClient) doUnhedged(req *http.Request) ([]byte, error) {
	resp, err := client.delegate.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return io.ReadAll(resp.Body)
	}
	return nil, &statusError{resp.StatusCode, resp.Status}
}
