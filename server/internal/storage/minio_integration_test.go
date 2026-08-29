package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestMinIOContractAgainstDisposableServer(t *testing.T) {
	endpoint := os.Getenv("TEST_MINIO_ENDPOINT")
	accessKey := os.Getenv("TEST_MINIO_ACCESS_KEY")
	secretKey := os.Getenv("TEST_MINIO_SECRET_KEY")
	if endpoint == "" || accessKey == "" || secretKey == "" {
		t.Skip("disposable MinIO configuration is not available")
	}
	ctx := context.Background()
	bucket := "contract-" + strings.ToLower(strings.ReplaceAll(uuid.NewString(), "-", ""))
	admin, err := minio.New(endpoint, &minio.Options{Creds: credentials.NewStaticV4(accessKey, secretKey, "")})
	if err != nil {
		t.Fatal(err)
	}
	if err := admin.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
		t.Fatal(err)
	}
	defer admin.RemoveBucket(ctx, bucket)

	client, err := NewMinIO(MinIOConfig{
		Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket,
		ConnectTimeout: time.Second, OperationTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	key := "system/contract-object"
	content := []byte("disposable MinIO contract")
	if _, err := client.Put(ctx, key, bytes.NewReader(content), int64(len(content)), "text/plain"); err != nil {
		t.Fatal(err)
	}
	defer client.Remove(ctx, key)
	info, err := client.Stat(ctx, key)
	if err != nil || info.Size != int64(len(content)) {
		t.Fatalf("Stat() info=%#v error=%v", info, err)
	}
	object, err := client.Open(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	read, err := io.ReadAll(object.Body)
	closeErr := object.Body.Close()
	if err != nil || closeErr != nil || !bytes.Equal(read, content) {
		t.Fatalf("Open() content=%q readError=%v closeError=%v", read, err, closeErr)
	}
	if err := client.Remove(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Stat(ctx, key); !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("missing Stat() error = %v", err)
	}

	wrongCredentials, err := NewMinIO(MinIOConfig{
		Endpoint: endpoint, AccessKey: "wrong-access-key", SecretKey: "wrong-secret-key", Bucket: bucket,
		ConnectTimeout: time.Second, OperationTimeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := wrongCredentials.Ready(ctx); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), endpoint) {
		t.Fatalf("wrong-credential Ready() error = %v", err)
	}

	unavailable, err := NewMinIO(MinIOConfig{
		Endpoint: "127.0.0.1:1", AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket,
		ConnectTimeout: 100 * time.Millisecond, OperationTimeout: 200 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := unavailable.Ready(ctx); !errors.Is(err, ErrUnavailable) || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("unavailable Ready() error = %v", err)
	}
}
