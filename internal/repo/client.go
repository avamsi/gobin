package repo

import "context"

type Client interface {
	Get(ctx context.Context, url string) (resp []byte, err error)
}
