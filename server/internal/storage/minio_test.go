package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/minio/minio-go/v7"
)

type fakeMinIOAPI struct {
	putInfo    minio.UploadInfo
	putErr     error
	openBody   io.ReadCloser
	objectInfo minio.ObjectInfo
	openErr    error
	statErr    error
	removeErr  error
	bucketOK   bool
	bucketErr  error
	putBucket  string
	putKey     string
	putType    string
	removedKey string
	blockStat  bool
	openCtx    context.Context
}

func (f *fakeMinIOAPI) put(_ context.Context, bucket, key string, _ io.Reader, _ int64, contentType string) (minio.UploadInfo, error) {
	f.putBucket, f.putKey, f.putType = bucket, key, contentType
	return f.putInfo, f.putErr
}

func (f *fakeMinIOAPI) open(ctx context.Context, _, _ string) (io.ReadCloser, minio.ObjectInfo, error) {
	f.openCtx = ctx
	return f.openBody, f.objectInfo, f.openErr
}

func (f *fakeMinIOAPI) stat(ctx context.Context, _, _ string) (minio.ObjectInfo, error) {
	if f.blockStat {
		<-ctx.Done()
		return minio.ObjectInfo{}, ctx.Err()
	}
	return f.objectInfo, f.statErr
}

func (f *fakeMinIOAPI) remove(_ context.Context, _, key string) error {
	f.removedKey = key
	return f.removeErr
}

func (f *fakeMinIOAPI) bucketExists(_ context.Context, _ string) (bool, error) {
	return f.bucketOK, f.bucketErr
}

func TestMinIOPutUsesConfiguredPrivateBucket(t *testing.T) {
	fake := &fakeMinIOAPI{putInfo: minio.UploadInfo{Size: 3, ETag: "etag"}}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	info, err := storage.Put(context.Background(), "system/file-id", strings.NewReader("abc"), 3, "text/plain")
	if err != nil {
		t.Fatal(err)
	}
	if fake.putBucket != "private-files" || fake.putKey != "system/file-id" || fake.putType != "text/plain" {
		t.Fatalf("put target = %q/%q (%q)", fake.putBucket, fake.putKey, fake.putType)
	}
	if info.Size != 3 || info.ETag != "etag" {
		t.Fatalf("put info = %#v", info)
	}
}

func TestMinIOOpenReturnsReadableStreamAndMetadata(t *testing.T) {
	fake := &fakeMinIOAPI{
		openBody:   io.NopCloser(bytes.NewBufferString("content")),
		objectInfo: minio.ObjectInfo{Size: 7, ETag: "etag", ContentType: "text/plain"},
	}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	object, err := storage.Open(context.Background(), "system/file-id")
	if err != nil {
		t.Fatal(err)
	}
	defer object.Body.Close()
	body, err := io.ReadAll(object.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "content" || object.Info.Size != 7 || object.Info.ContentType != "text/plain" {
		t.Fatalf("open result = %q %#v", body, object.Info)
	}
}

func TestMinIOOpenKeepsContextAliveUntilStreamClose(t *testing.T) {
	fake := &fakeMinIOAPI{
		openBody:   io.NopCloser(bytes.NewBufferString("content")),
		objectInfo: minio.ObjectInfo{Size: 7},
	}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	object, err := storage.Open(context.Background(), "system/file-id")
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.openCtx.Done():
		t.Fatal("Open() canceled the operation context before the stream was consumed")
	default:
	}
	if err := object.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-fake.openCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("closing the stream did not release the operation context")
	}
}

func TestMinIOStatMapsMissingObjectToStableError(t *testing.T) {
	fake := &fakeMinIOAPI{statErr: minio.ErrorResponse{Code: "NoSuchKey", Message: "internal endpoint detail"}}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	_, err := storage.Stat(context.Background(), "missing")
	if !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("Stat() error = %v", err)
	}
	if strings.Contains(err.Error(), "endpoint") {
		t.Fatalf("Stat() leaked internal detail: %v", err)
	}
}

func TestMinIOPutRejectsReportedSizeMismatch(t *testing.T) {
	fake := &fakeMinIOAPI{putInfo: minio.UploadInfo{Size: 2}}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	_, err := storage.Put(context.Background(), "system/file-id", strings.NewReader("abc"), 3, "text/plain")
	if !errors.Is(err, ErrIntegrityMismatch) {
		t.Fatalf("Put() error = %v", err)
	}
}

func TestMinIORemoveIsAvailableForInternalCompensation(t *testing.T) {
	fake := &fakeMinIOAPI{}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	if err := storage.Remove(context.Background(), "system/file-id"); err != nil {
		t.Fatal(err)
	}
	if fake.removedKey != "system/file-id" {
		t.Fatalf("removed key = %q", fake.removedKey)
	}
}

func TestMinIOOperationTimeoutIsBounded(t *testing.T) {
	fake := &fakeMinIOAPI{blockStat: true}
	storage := newMinIOWithClient("private-files", 10*time.Millisecond, fake)

	started := time.Now()
	_, err := storage.Stat(context.Background(), "slow")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Stat() error = %v", err)
	}
	if time.Since(started) > time.Second {
		t.Fatal("Stat() did not honor the operation timeout")
	}
}

func TestMinIOWrongCredentialsFailWithoutLeakingDetails(t *testing.T) {
	fake := &fakeMinIOAPI{statErr: minio.ErrorResponse{Code: "InvalidAccessKeyId", Message: "http://secret-internal:9000"}}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	_, err := storage.Stat(context.Background(), "system/file-id")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Stat() error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-internal") {
		t.Fatalf("Stat() leaked endpoint: %v", err)
	}
}

func TestMinIOReadyRequiresAccessibleBucket(t *testing.T) {
	fake := &fakeMinIOAPI{bucketOK: false}
	storage := newMinIOWithClient("private-files", time.Second, fake)

	if err := storage.Ready(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Ready() error = %v", err)
	}
}
