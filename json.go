package main

import (
	"encoding/json"

	"github.com/avamsi/ergo/assert"
)

func jsonUnmarshal[T any](b []byte) T {
	var v T
	assert.Nil(json.Unmarshal(b, &v))
	return v
}
