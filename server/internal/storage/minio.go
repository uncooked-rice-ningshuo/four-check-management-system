package storage

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// MinIOConfig contains static private-bucket connection settings.
type MinIOConfig struct {
	Endpoint         string
	AccessKey        string
	SecretKey        string
	Bucket           string
	Region           string
	UseTLS           bool
	ConnectTimeout   time.Duration
	OperationTimeout time.Duration
}

type minioAPI interface {
	put(context.Context, string, string, io.Reader, int64, string) (minio.UploadInfo, error)
	open(context.Context, string, string) (io.ReadCloser, minio.ObjectInfo, error)
	stat(context.Context, string, string) (minio.ObjectInfo, error)
	remove(context.Context, string, string) error
	bucketExists(context.Context, string) (bool, error)
}

type minioSDK struct{ core *minio.Core }

func (s minioSDK) put(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (minio.UploadInfo, error) {
	return s.core.PutObject(ctx, bucket, key, reader, size, "", "", minio.PutObjectOptions{ContentType: contentType})
}

func (s minioSDK) open(ctx context.Context, bucket, key string) (io.ReadCloser, minio.ObjectInfo, error) {
	body, info, _, err := s.core.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	return body, info, err
}

func (s minioSDK) stat(ctx context.Context, bucket, key string) (minio.ObjectInfo, error) {
	return s.core.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
}

func (s minioSDK) remove(ctx context.Context, bucket, key string) error {
	return s.core.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

func (s minioSDK) bucketExists(ctx context.Context, bucket string) (bool, error) {
	return s.core.BucketExists(ctx, bucket)
}

// MinIO implements ObjectStorage for one configured private bucket.
type MinIO struct {
	bucket           string
	operationTimeout time.Duration
	client           minioAPI
}

// NewMinIO validates static configuration and constructs a bounded MinIO client without performing I/O.
func NewMinIO(config MinIOConfig) (*MinIO, error) {
	if config.Endpoint == "" || config.AccessKey == "" || config.SecretKey == "" || config.Bucket == "" || config.ConnectTimeout <= 0 || config.OperationTimeout <= 0 {
		return nil, ErrUnavailable
	}
	dialer := &net.Dialer{Timeout: config.ConnectTimeout, KeepAlive: 30 * time.Second}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	transport.TLSHandshakeTimeout = config.ConnectTimeout
	transport.ResponseHeaderTimeout = config.OperationTimeout
	transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	core, err := minio.NewCore(config.Endpoint, &minio.Options{
		Creds:     credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure:    config.UseTLS,
		Region:    config.Region,
		Transport: transport,
	})
	if err != nil {
		return nil, ErrUnavailable
	}
	return newMinIOWithClient(config.Bucket, config.OperationTimeout, minioSDK{core: core}), nil
}

func newMinIOWithClient(bucket string, operationTimeout time.Duration, client minioAPI) *MinIO {
	return &MinIO{bucket: bucket, operationTimeout: operationTimeout, client: client}
}

func (m *MinIO) Bucket() string { return m.bucket }

func (m *MinIO) Put(ctx context.Context, key string, reader io.Reader, size int64, contentType string) (ObjectInfo, error) {
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	info, err := m.client.put(ctx, m.bucket, key, reader, size, contentType)
	if err != nil {
		return ObjectInfo{}, mapMinIOError(err)
	}
	if info.Size != size {
		return ObjectInfo{}, ErrIntegrityMismatch
	}
	return ObjectInfo{Size: info.Size, ETag: info.ETag}, nil
}

func (m *MinIO) Open(ctx context.Context, key string) (Object, error) {
	ctx, cancel := m.operationContext(ctx)
	body, info, err := m.client.open(ctx, m.bucket, key)
	if err != nil {
		cancel()
		if body != nil {
			_ = body.Close()
		}
		return Object{}, mapMinIOError(err)
	}
	return Object{Body: cancelReadCloser{ReadCloser: body, cancel: cancel}, Info: objectInfo(info)}, nil
}

func (m *MinIO) Stat(ctx context.Context, key string) (ObjectInfo, error) {
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	info, err := m.client.stat(ctx, m.bucket, key)
	if err != nil {
		return ObjectInfo{}, mapMinIOError(err)
	}
	return objectInfo(info), nil
}

func (m *MinIO) Remove(ctx context.Context, key string) error {
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	if err := m.client.remove(ctx, m.bucket, key); err != nil {
		mapped := mapMinIOError(err)
		if mapped == ErrObjectMissing {
			return nil
		}
		return mapped
	}
	return nil
}

func (m *MinIO) Ready(ctx context.Context) error {
	ctx, cancel := m.operationContext(ctx)
	defer cancel()
	exists, err := m.client.bucketExists(ctx, m.bucket)
	if err != nil || !exists {
		return ErrUnavailable
	}
	return nil
}

func (m *MinIO) operationContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, m.operationTimeout)
}

func objectInfo(info minio.ObjectInfo) ObjectInfo {
	return ObjectInfo{Size: info.Size, ETag: info.ETag, ContentType: info.ContentType}
}

func mapMinIOError(err error) error {
	if err == nil {
		return nil
	}
	code := minio.ToErrorResponse(err).Code
	switch code {
	case "NoSuchKey", "NoSuchObject", "NotFound":
		return ErrObjectMissing
	default:
		return ErrUnavailable
	}
}

type cancelReadCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r cancelReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.cancel()
	return err
}
