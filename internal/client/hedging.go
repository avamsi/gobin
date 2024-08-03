package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/avamsi/ergo/pair"
)

type Hedging struct {
	client *http.Client
	after  time.Duration
}

func Hedge(client *http.Client, after time.Duration) *Hedging {
	return &Hedging{client, after}
}

func (h *Hedging) doUnhedged(req *http.Request) ([]byte, error) {
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s %q: %s", req.Method, req.URL, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func (h *Hedging) Get(ctx context.Context, url string) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	var (
		resp = make(chan pair.Pair[[]byte, error])
		do   = func() {
			// Normally, we'd want to deep copy the request here but it looks
			// like we should be okay given body is always nil.
			resp <- pair.New(h.doUnhedged(req))
		}
	)
	go do() // first request
	select {
	case r := <-resp:
		return r.Unpack()
	case <-time.After(h.after):
		go do() // second request (to hedge the first)
	}
	// Use the first response that comes back and cancel the other request
	// (implicitly, via the deferred cancel on ctx at the beginning).
	return (<-resp).Unpack()
}
