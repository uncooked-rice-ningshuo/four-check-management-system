package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/httpapi"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"four-check-management-system/server/internal/store"
)

type storageRuntimeConfig struct {
	MinIO       storage.MinIOConfig
	MaxFileSize int64
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	db, err := store.Open(ctx, mustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	storageConfig, err := loadStorageConfig(os.Getenv)
	if err != nil {
		log.Fatal(err)
	}
	objectStorage, err := storage.NewMinIO(storageConfig.MinIO)
	if err != nil {
		log.Fatal("invalid MinIO configuration")
	}
	storageService := storage.NewService(db, objectStorage, storageConfig.MaxFileSize)
	svc := auth.NewService(db, db, []byte(mustEnv("JWT_SECRET")), 15*time.Minute, 7*24*time.Hour)
	if _, err := svc.BootstrapAdmin(ctx, envOr("BOOTSTRAP_ADMIN_LOGIN", "admin"), envOr("BOOTSTRAP_ADMIN_NAME", "系统管理员"), mustEnv("BOOTSTRAP_ADMIN_PASSWORD")); err != nil {
		log.Fatal(err)
	}
	services := newApplicationServices(db, storageService, storageConfig.MaxFileSize)
	services.PostgresReady = db
	services.ObjectStorageReady = objectStorage
	r := httpapi.NewRouter(svc, os.Getenv("COOKIE_SECURE") == "true", services)
	if err := httpapi.ConfigureTrustedProxies(r, os.Getenv("TRUSTED_PROXY_CIDRS")); err != nil {
		log.Fatal(err)
	}
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("api listening on %s", addr)
	log.Fatal(r.Run(addr))
}

func newApplicationServices(db *store.Postgres, storageService *storage.Service, maxUploadBytes int64) httpapi.AdminServices {
	return httpapi.AdminServices{
		Organization: organization.NewService(db), Academic: academic.NewService(db), Permission: permission.NewService(db),
		MasterData: masterdata.NewService(db), Dictionary: dictionary.NewService(db), Storage: storageService,
		ImportJobs: importjob.NewService(db, storageService, importjob.NewAttendanceXLSXParser(maxUploadBytes), importjob.NewAttendanceAdapter(db)),
		Attendance: inspection.NewService(db), ImportFiles: storageService, ImportMaxUploadBytes: maxUploadBytes,
	}
}

func loadStorageConfig(getenv func(string) string) (storageRuntimeConfig, error) {
	required := func(key string) (string, error) {
		value := getenv(key)
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%s is required", key)
		}
		return value, nil
	}
	endpoint, err := required("MINIO_ENDPOINT")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	accessKey, err := required("MINIO_ACCESS_KEY")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	secretKey, err := required("MINIO_SECRET_KEY")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	bucket, err := required("MINIO_BUCKET")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	useTLSRaw, err := required("MINIO_USE_TLS")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	useTLS, err := strconv.ParseBool(useTLSRaw)
	if err != nil {
		return storageRuntimeConfig{}, fmt.Errorf("MINIO_USE_TLS must be true or false")
	}
	connectTimeout, err := positiveDuration(getenv("MINIO_CONNECT_TIMEOUT"), "MINIO_CONNECT_TIMEOUT")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	operationTimeout, err := positiveDuration(getenv("MINIO_OPERATION_TIMEOUT"), "MINIO_OPERATION_TIMEOUT")
	if err != nil {
		return storageRuntimeConfig{}, err
	}
	maxFileSize, err := strconv.ParseInt(getenv("MINIO_MAX_FILE_SIZE_BYTES"), 10, 64)
	if err != nil || maxFileSize <= 0 {
		return storageRuntimeConfig{}, fmt.Errorf("MINIO_MAX_FILE_SIZE_BYTES must be a positive integer")
	}
	region := getenv("MINIO_REGION")
	if region == "" {
		region = "us-east-1"
	}
	return storageRuntimeConfig{MinIO: storage.MinIOConfig{
		Endpoint: endpoint, AccessKey: accessKey, SecretKey: secretKey, Bucket: bucket, Region: region,
		UseTLS: useTLS, ConnectTimeout: connectTimeout, OperationTimeout: operationTimeout,
	}, MaxFileSize: maxFileSize}, nil
}

func positiveDuration(raw, key string) (time.Duration, error) {
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", key)
	}
	return value, nil
}
func envOr(k, fallback string) string {
	if value := os.Getenv(k); value != "" {
		return value
	}
	return fallback
}
func mustEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("%s is required", k)
	}
	return v
}
