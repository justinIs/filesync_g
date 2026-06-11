// Package store defines the interface and implementations for remote file stores
package store

import (
	"context"
	"io"
)

type FileStore interface {
	Put(ctx context.Context, key string, r io.Reader) error
	Delete(ctx context.Context, key string) error
}
