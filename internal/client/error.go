package client

import (
	"fmt"
	"net/url"
)

type HttpError struct {
	method     string
	url        *url.URL
	status     string
	StatusCode int
}

func (err *HttpError) Error() string {
	return fmt.Sprintf("%s %q: %s", err.method, err.url, err.status)
}
