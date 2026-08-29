package storage

import (
	"bufio"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

const (
	StatusPending   = "PENDING"
	StatusAvailable = "AVAILABLE"
	StatusFailed    = "FAILED"
	StatusMissing   = "MISSING"

	RetentionTemporary    = "TEMPORARY"
	RetentionImportSource = "IMPORT_SOURCE"
	RetentionTemplate     = "TEMPLATE"
	RetentionReportDraft  = "REPORT_DRAFT"
	RetentionReportFinal  = "REPORT_FINAL"
	RetentionArchive      = "ARCHIVE"

	FailureStorageUnavailable = "STORAGE_UNAVAILABLE"
	FailureIntegrityMismatch  = "INTEGRITY_MISMATCH"
	FailureObjectMissing      = "OBJECT_MISSING"
)

var (
	ErrFileNotFound    = errors.New("file metadata not found")
	ErrFileUnavailable = errors.New("file is not available")
	ErrFileTooLarge    = errors.New("file exceeds configured size limit")
	ErrInvalidFile     = errors.New("invalid file metadata")
)

// File is the PostgreSQL metadata for one private MinIO object.
type File struct {
	ID                  string
	StorageProvider     string
	Bucket              string
	ObjectKey           string
	OriginalName        string
	DeclaredContentType string
	DetectedContentType string
	SizeBytes           int64
	SHA256              string
	Status              string
	RetentionType       string
	CreatedBy           string
	FailureCode         string
	FailureMessage      string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

// StoreInput contains content already authorized by a concrete business use case.
type StoreInput struct {
	OriginalName        string
	DeclaredContentType string
	Size                int64
	RetentionType       string
	CreatedBy           string
	Body                io.Reader
}

type ReconcileResult struct {
	Recovered int
	Cleaned   int
	Failed    int
}

// FileRepository persists file state transitions and their audit entries.
type FileRepository interface {
	CreateFile(context.Context, File) error
	FindFile(context.Context, string) (File, error)
	MarkFileAvailable(context.Context, File, audit.Mutation) error
	MarkFileFailed(context.Context, File, audit.Mutation) error
	MarkFileMissing(context.Context, File, audit.Mutation) error
	ListFileReconciliationCandidates(context.Context, time.Time, int) ([]File, error)
	RecordFileAudit(context.Context, audit.Mutation) error
}

// Service coordinates PostgreSQL metadata with private object storage using explicit states and compensation.
type Service struct {
	repository FileRepository
	objects    ObjectStorage
	maxSize    int64
	now        func() time.Time
}

func NewService(repository FileRepository, objects ObjectStorage, maxSize int64) *Service {
	return &Service{repository: repository, objects: objects, maxSize: maxSize, now: time.Now}
}

func (s *Service) Store(ctx context.Context, input StoreInput, metadata audit.Metadata) (File, error) {
	if err := s.validateStoreInput(input); err != nil {
		return File{}, err
	}
	now := s.now().UTC()
	file := File{
		ID: uuid.NewString(), StorageProvider: "MINIO", Bucket: s.objects.Bucket(), OriginalName: input.OriginalName,
		DeclaredContentType: input.DeclaredContentType, SizeBytes: input.Size, Status: StatusPending,
		RetentionType: input.RetentionType, CreatedBy: input.CreatedBy, CreatedAt: now, UpdatedAt: now,
	}
	file.ObjectKey = fmt.Sprintf("system/%04d/%02d/%s", now.Year(), int(now.Month()), file.ID)
	if err := s.repository.CreateFile(ctx, file); err != nil {
		return File{}, err
	}

	buffered := bufio.NewReader(input.Body)
	detected, err := detectContentType(buffered)
	if err != nil {
		return s.failStore(ctx, file, err, metadata)
	}
	file.DetectedContentType = detected
	hash := sha256.New()
	reader := io.TeeReader(buffered, hash)
	if _, err := s.objects.Put(ctx, file.ObjectKey, reader, input.Size, detected); err != nil {
		return s.failStore(ctx, file, err, metadata)
	}
	if _, err := buffered.ReadByte(); err == nil {
		return s.failStore(ctx, file, ErrIntegrityMismatch, metadata)
	} else if !errors.Is(err, io.EOF) {
		return s.failStore(ctx, file, err, metadata)
	}
	file.SHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	observed, err := s.objects.Stat(ctx, file.ObjectKey)
	if err != nil {
		return s.failStore(ctx, file, err, metadata)
	}
	if observed.Size != input.Size {
		return s.failStore(ctx, file, ErrIntegrityMismatch, metadata)
	}
	file.Status = StatusAvailable
	file.UpdatedAt = s.now().UTC()
	mutation := fileMutation(metadata, audit.ActionFileUploadSucceeded, file.ID, StatusPending, file)
	if err := s.repository.MarkFileAvailable(ctx, file, mutation); err != nil {
		_ = s.objects.Remove(ctx, file.ObjectKey)
		return file, err
	}
	return file, nil
}

func (s *Service) Open(ctx context.Context, id string, metadata audit.Metadata) (File, Object, error) {
	file, err := s.repository.FindFile(ctx, id)
	if err != nil {
		return File{}, Object{}, err
	}
	if file.Status != StatusAvailable {
		return file, Object{}, ErrFileUnavailable
	}
	object, err := s.objects.Open(ctx, file.ObjectKey)
	if errors.Is(err, ErrObjectMissing) {
		file.Status, file.FailureCode, file.FailureMessage = StatusMissing, FailureObjectMissing, "对象不存在"
		mutation := fileMutation(metadata, audit.ActionFileIntegrityAnomaly, file.ID, StatusAvailable, file)
		if markErr := s.repository.MarkFileMissing(ctx, file, mutation); markErr != nil {
			return file, Object{}, markErr
		}
		return file, Object{}, ErrObjectMissing
	}
	if err != nil {
		return file, Object{}, err
	}
	mutation := fileMutation(metadata, audit.ActionFileRead, file.ID, StatusAvailable, file)
	if err := s.repository.RecordFileAudit(ctx, mutation); err != nil {
		_ = object.Body.Close()
		return file, Object{}, err
	}
	return file, object, nil
}

// Reconcile targets only stale rows selected by PostgreSQL; it never enumerates the entire bucket.
func (s *Service) Reconcile(ctx context.Context, staleBefore time.Time, limit int, metadata audit.Metadata) (ReconcileResult, error) {
	var result ReconcileResult
	if limit <= 0 {
		return result, ErrInvalidFile
	}
	files, err := s.repository.ListFileReconciliationCandidates(ctx, staleBefore, limit)
	if err != nil {
		return result, err
	}
	for _, file := range files {
		switch file.Status {
		case StatusFailed:
			if err := s.objects.Remove(ctx, file.ObjectKey); err != nil {
				result.Failed++
			} else {
				result.Cleaned++
			}
		case StatusPending:
			if err := s.recoverPending(ctx, file, metadata); err != nil {
				result.Failed++
			} else {
				result.Recovered++
			}
		}
	}
	return result, nil
}

func (s *Service) recoverPending(ctx context.Context, file File, metadata audit.Metadata) error {
	object, err := s.objects.Open(ctx, file.ObjectKey)
	if err != nil {
		_, failureErr := s.failStore(ctx, file, err, metadata)
		return failureErr
	}
	defer object.Body.Close()
	buffered := bufio.NewReader(object.Body)
	detected, err := detectContentType(buffered)
	if err != nil {
		_, failureErr := s.failStore(ctx, file, err, metadata)
		return failureErr
	}
	hash := sha256.New()
	read, err := io.Copy(hash, buffered)
	if err != nil || read != file.SizeBytes || object.Info.Size != file.SizeBytes {
		if err == nil {
			err = ErrIntegrityMismatch
		}
		_, failureErr := s.failStore(ctx, file, err, metadata)
		return failureErr
	}
	file.DetectedContentType = detected
	file.SHA256 = fmt.Sprintf("%x", hash.Sum(nil))
	file.Status = StatusAvailable
	file.UpdatedAt = s.now().UTC()
	return s.repository.MarkFileAvailable(ctx, file, fileMutation(metadata, audit.ActionFileUploadSucceeded, file.ID, StatusPending, file))
}

func (s *Service) failStore(ctx context.Context, file File, cause error, metadata audit.Metadata) (File, error) {
	file.Status = StatusFailed
	file.FailureCode, file.FailureMessage = stableFailure(cause)
	file.UpdatedAt = s.now().UTC()
	mutation := fileMutation(metadata, audit.ActionFileUploadFailed, file.ID, StatusPending, file)
	if err := s.repository.MarkFileFailed(ctx, file, mutation); err != nil {
		_ = s.objects.Remove(ctx, file.ObjectKey)
		return file, err
	}
	_ = s.objects.Remove(ctx, file.ObjectKey)
	return file, cause
}

func (s *Service) validateStoreInput(input StoreInput) error {
	if strings.TrimSpace(input.OriginalName) == "" || utf8.RuneCountInString(input.OriginalName) > 255 || input.Body == nil || input.Size < 0 || !validRetention(input.RetentionType) || s.maxSize <= 0 {
		return ErrInvalidFile
	}
	if input.Size > s.maxSize {
		return ErrFileTooLarge
	}
	return nil
}

func validRetention(value string) bool {
	switch value {
	case RetentionTemporary, RetentionImportSource, RetentionTemplate, RetentionReportDraft, RetentionReportFinal, RetentionArchive:
		return true
	default:
		return false
	}
}

func detectContentType(reader *bufio.Reader) (string, error) {
	prefix, err := reader.Peek(512)
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, bufio.ErrBufferFull) {
		return "", err
	}
	return http.DetectContentType(prefix), nil
}

func stableFailure(err error) (string, string) {
	switch {
	case errors.Is(err, ErrObjectMissing):
		return FailureObjectMissing, "对象不存在"
	case errors.Is(err, ErrIntegrityMismatch):
		return FailureIntegrityMismatch, "对象完整性校验失败"
	default:
		return FailureStorageUnavailable, "对象存储不可用"
	}
}

func fileMutation(metadata audit.Metadata, action audit.Action, id, beforeStatus string, file File) audit.Mutation {
	return audit.Mutation{Metadata: metadata, Entry: audit.Entry{
		Action: action, ResourceType: "file", ResourceID: id,
		Before: map[string]any{"status": beforeStatus},
		After:  map[string]any{"status": file.Status, "sizeBytes": file.SizeBytes, "sha256": file.SHA256, "failureCode": file.FailureCode},
	}}
}
