package storage

import (
	"context"
	"errors"
	"io"
)

var (
	// ErrUnavailable hides infrastructure and credential details from callers.
	ErrUnavailable = errors.New("object storage unavailable")
	// ErrObjectMissing identifies an object that no longer exists in the private bucket.
	ErrObjectMissing = errors.New("object missing")
	// ErrIntegrityMismatch identifies an uploaded object whose observed metadata differs from the expected content.
	ErrIntegrityMismatch = errors.New("object integrity mismatch")
)

// ObjectInfo contains only metadata needed by the storage application service.
type ObjectInfo struct {
	Size        int64
	ETag        string
	ContentType string
}

// Object is a caller-owned read stream and its verified object metadata.
type Object struct {
	Body io.ReadCloser
	Info ObjectInfo
}

// ObjectStorage is the internal private-bucket boundary. Remove is reserved for compensation.
type ObjectStorage interface {
	Bucket() string
	Put(context.Context, string, io.Reader, int64, string) (ObjectInfo, error)
	Open(context.Context, string) (Object, error)
	Stat(context.Context, string) (ObjectInfo, error)
	Remove(context.Context, string) error
	Ready(context.Context) error
}
