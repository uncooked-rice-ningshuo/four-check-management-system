package main

import (
	"testing"
	"time"
)

func TestLoadStorageConfigValidatesAndParsesEnvironment(t *testing.T) {
	values := map[string]string{
		"MINIO_ENDPOINT":            "storage.internal:9000",
		"MINIO_ACCESS_KEY":          "application-account",
		"MINIO_SECRET_KEY":          "application-secret",
		"MINIO_BUCKET":              "private-files",
		"MINIO_REGION":              "us-east-1",
		"MINIO_USE_TLS":             "true",
		"MINIO_CONNECT_TIMEOUT":     "4s",
		"MINIO_OPERATION_TIMEOUT":   "25s",
		"MINIO_MAX_FILE_SIZE_BYTES": "1048576",
	}
	config, err := loadStorageConfig(func(key string) string { return values[key] })
	if err != nil {
		t.Fatal(err)
	}
	if !config.MinIO.UseTLS || config.MinIO.ConnectTimeout != 4*time.Second || config.MinIO.OperationTimeout != 25*time.Second {
		t.Fatalf("MinIO config = %#v", config.MinIO)
	}
	if config.MaxFileSize != 1048576 {
		t.Fatalf("max file size = %d", config.MaxFileSize)
	}
}

func TestLoadStorageConfigRejectsMissingCredentialsWithoutEchoingValues(t *testing.T) {
	values := map[string]string{
		"MINIO_ENDPOINT":            "secret-storage.internal:9000",
		"MINIO_ACCESS_KEY":          "application-account",
		"MINIO_BUCKET":              "private-files",
		"MINIO_CONNECT_TIMEOUT":     "4s",
		"MINIO_OPERATION_TIMEOUT":   "25s",
		"MINIO_MAX_FILE_SIZE_BYTES": "1048576",
	}
	_, err := loadStorageConfig(func(key string) string { return values[key] })
	if err == nil {
		t.Fatal("loadStorageConfig() error = nil")
	}
	if err.Error() == values["MINIO_ENDPOINT"] || err.Error() == values["MINIO_ACCESS_KEY"] {
		t.Fatalf("configuration error leaked a value: %v", err)
	}
}
