package storage

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
)

type memoryFileRepository struct {
	files      map[string]File
	created    []File
	available  []File
	failed     []File
	missing    []File
	candidates []File
	audits     []audit.Mutation
}

func newMemoryFileRepository() *memoryFileRepository {
	return &memoryFileRepository{files: map[string]File{}}
}

func (r *memoryFileRepository) CreateFile(_ context.Context, file File) error {
	r.files[file.ID] = file
	r.created = append(r.created, file)
	return nil
}

func (r *memoryFileRepository) FindFile(_ context.Context, id string) (File, error) {
	file, ok := r.files[id]
	if !ok {
		return File{}, ErrFileNotFound
	}
	return file, nil
}

func (r *memoryFileRepository) MarkFileAvailable(_ context.Context, file File, _ audit.Mutation) error {
	file.Status = StatusAvailable
	r.files[file.ID] = file
	r.available = append(r.available, file)
	return nil
}

func (r *memoryFileRepository) MarkFileFailed(_ context.Context, file File, _ audit.Mutation) error {
	file.Status = StatusFailed
	r.files[file.ID] = file
	r.failed = append(r.failed, file)
	return nil
}

func (r *memoryFileRepository) MarkFileMissing(_ context.Context, file File, _ audit.Mutation) error {
	file.Status = StatusMissing
	r.files[file.ID] = file
	r.missing = append(r.missing, file)
	return nil
}

func (r *memoryFileRepository) ListFileReconciliationCandidates(_ context.Context, _ time.Time, _ int) ([]File, error) {
	return r.candidates, nil
}

func (r *memoryFileRepository) RecordFileAudit(_ context.Context, mutation audit.Mutation) error {
	r.audits = append(r.audits, mutation)
	return nil
}

type memoryObjectStorage struct {
	bucket    string
	data      map[string][]byte
	putErr    error
	statErr   error
	openErr   error
	removed   []string
	openCalls int
	putType   string
	statSize  *int64
	readyErr  error
}

func newMemoryObjectStorage() *memoryObjectStorage {
	return &memoryObjectStorage{bucket: "private-files", data: map[string][]byte{}}
}

func (s *memoryObjectStorage) Bucket() string { return s.bucket }

func (s *memoryObjectStorage) Put(_ context.Context, key string, reader io.Reader, size int64, contentType string) (ObjectInfo, error) {
	if s.putErr != nil {
		return ObjectInfo{}, s.putErr
	}
	s.putType = contentType
	var target bytes.Buffer
	if _, err := io.CopyBuffer(&target, io.LimitReader(reader, size), make([]byte, 7)); err != nil {
		return ObjectInfo{}, err
	}
	s.data[key] = target.Bytes()
	return ObjectInfo{Size: size, ETag: "etag", ContentType: contentType}, nil
}

func (s *memoryObjectStorage) Open(_ context.Context, key string) (Object, error) {
	s.openCalls++
	if s.openErr != nil {
		return Object{}, s.openErr
	}
	data, ok := s.data[key]
	if !ok {
		return Object{}, ErrObjectMissing
	}
	return Object{Body: io.NopCloser(bytes.NewReader(data)), Info: ObjectInfo{Size: int64(len(data))}}, nil
}

func (s *memoryObjectStorage) Stat(_ context.Context, key string) (ObjectInfo, error) {
	if s.statErr != nil {
		return ObjectInfo{}, s.statErr
	}
	data, ok := s.data[key]
	if !ok {
		return ObjectInfo{}, ErrObjectMissing
	}
	size := int64(len(data))
	if s.statSize != nil {
		size = *s.statSize
	}
	return ObjectInfo{Size: size, ETag: "etag"}, nil
}

func (s *memoryObjectStorage) Remove(_ context.Context, key string) error {
	s.removed = append(s.removed, key)
	delete(s.data, key)
	return nil
}

func (s *memoryObjectStorage) Ready(_ context.Context) error { return s.readyErr }

func TestStoreGeneratesKeyAndPersistsVerifiedMetadata(t *testing.T) {
	repository := newMemoryFileRepository()
	objects := newMemoryObjectStorage()
	service := NewService(repository, objects, 1024)
	content := []byte("hello storage")

	file, err := service.Store(context.Background(), StoreInput{
		OriginalName: "../../成绩.xlsx", DeclaredContentType: "application/octet-stream", Size: int64(len(content)),
		RetentionType: RetentionImportSource, CreatedBy: "actor-id", Body: bytes.NewReader(content),
	}, audit.Metadata{ActorID: "actor-id"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(file.ObjectKey, "成绩") || !strings.HasPrefix(file.ObjectKey, "system/") || !strings.HasSuffix(file.ObjectKey, file.ID) {
		t.Fatalf("generated object key = %q", file.ObjectKey)
	}
	wantSHA := fmt.Sprintf("%x", sha256.Sum256(content))
	if file.Status != StatusAvailable || file.SHA256 != wantSHA || file.SizeBytes != int64(len(content)) {
		t.Fatalf("stored file = %#v", file)
	}
	if file.DetectedContentType != "text/plain; charset=utf-8" || objects.putType != file.DetectedContentType {
		t.Fatalf("detected type = %q, uploaded type = %q", file.DetectedContentType, objects.putType)
	}
	if len(repository.created) != 1 || repository.created[0].Status != StatusPending || len(repository.available) != 1 {
		t.Fatalf("transitions created=%#v available=%#v", repository.created, repository.available)
	}
}

func TestStoreRejectsFileOverConfiguredLimitBeforeCreatingMetadata(t *testing.T) {
	repository := newMemoryFileRepository()
	service := NewService(repository, newMemoryObjectStorage(), 4)

	_, err := service.Store(context.Background(), StoreInput{OriginalName: "large.bin", Size: 5, RetentionType: RetentionTemporary, Body: strings.NewReader("12345")}, audit.Metadata{})
	if !errors.Is(err, ErrFileTooLarge) {
		t.Fatalf("Store() error = %v", err)
	}
	if len(repository.created) != 0 {
		t.Fatal("oversized file created metadata")
	}
}

func TestStoreRejectsContentLongerThanDeclaredSize(t *testing.T) {
	repository := newMemoryFileRepository()
	objects := newMemoryObjectStorage()
	service := NewService(repository, objects, 1024)

	_, err := service.Store(context.Background(), StoreInput{
		OriginalName: "mismatch.bin", Size: 3, RetentionType: RetentionTemporary, Body: strings.NewReader("abcdef"),
	}, audit.Metadata{})
	if !errors.Is(err, ErrIntegrityMismatch) {
		t.Fatalf("Store() error = %v", err)
	}
	if len(repository.failed) != 1 || repository.failed[0].FailureCode != FailureIntegrityMismatch {
		t.Fatalf("failed transitions = %#v", repository.failed)
	}
}

func TestStoreMarksFailureAndAttemptsCleanup(t *testing.T) {
	repository := newMemoryFileRepository()
	objects := newMemoryObjectStorage()
	objects.putErr = ErrUnavailable
	service := NewService(repository, objects, 1024)

	file, err := service.Store(context.Background(), StoreInput{OriginalName: "file.bin", Size: 3, RetentionType: RetentionTemporary, Body: strings.NewReader("abc")}, audit.Metadata{})
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Store() error = %v", err)
	}
	if len(repository.failed) != 1 || repository.failed[0].FailureCode != FailureStorageUnavailable {
		t.Fatalf("failed transitions = %#v", repository.failed)
	}
	if len(objects.removed) != 1 || objects.removed[0] != file.ObjectKey {
		t.Fatalf("cleanup keys = %#v", objects.removed)
	}
}

func TestDiscardMarksAvailableImportSourceRecoverableBeforeRemoval(t *testing.T) {
	repository := newMemoryFileRepository()
	repository.files["file-id"] = File{ID: "file-id", ObjectKey: "system/file-id", Status: StatusAvailable}
	objects := newMemoryObjectStorage()
	objects.data["system/file-id"] = []byte("private import")
	if err := NewService(repository, objects, 1024).Discard(context.Background(), "file-id", audit.Metadata{ActorID: "actor-id"}); err != nil {
		t.Fatal(err)
	}
	if len(repository.failed) != 1 || repository.failed[0].FailureCode != FailureOrphanedImport || len(objects.removed) != 1 {
		t.Fatalf("failed=%#v removed=%#v", repository.failed, objects.removed)
	}
}

func TestOpenRefusesNonAvailableFile(t *testing.T) {
	repository := newMemoryFileRepository()
	repository.files["file-id"] = File{ID: "file-id", ObjectKey: "system/file-id", Status: StatusPending}
	objects := newMemoryObjectStorage()
	service := NewService(repository, objects, 1024)

	_, _, err := service.Open(context.Background(), "file-id", audit.Metadata{})
	if !errors.Is(err, ErrFileUnavailable) {
		t.Fatalf("Open() error = %v", err)
	}
	if objects.openCalls != 0 {
		t.Fatal("Open() contacted object storage for non-AVAILABLE metadata")
	}
}

func TestOpenMarksAvailableMetadataMissingWhenObjectDisappears(t *testing.T) {
	repository := newMemoryFileRepository()
	repository.files["file-id"] = File{ID: "file-id", ObjectKey: "system/file-id", Status: StatusAvailable}
	objects := newMemoryObjectStorage()
	objects.openErr = ErrObjectMissing
	service := NewService(repository, objects, 1024)

	_, _, err := service.Open(context.Background(), "file-id", audit.Metadata{})
	if !errors.Is(err, ErrObjectMissing) {
		t.Fatalf("Open() error = %v", err)
	}
	if len(repository.missing) != 1 || repository.missing[0].FailureCode != FailureObjectMissing {
		t.Fatalf("missing transitions = %#v", repository.missing)
	}
}

func TestReadVerifiedReturnsContentOnlyAfterSHAAndSizeMatch(t *testing.T) {
	repository := newMemoryFileRepository()
	content := []byte("verified import workbook")
	wantSHA := fmt.Sprintf("%x", sha256.Sum256(content))
	repository.files["file-id"] = File{ID: "file-id", ObjectKey: "system/file-id", Status: StatusAvailable, SizeBytes: int64(len(content)), SHA256: wantSHA}
	objects := newMemoryObjectStorage()
	objects.data["system/file-id"] = content

	file, got, err := NewService(repository, objects, 1024).ReadVerified(context.Background(), "file-id", audit.Metadata{ActorID: "actor-id"})
	if err != nil {
		t.Fatal(err)
	}
	if file.ID != "file-id" || !bytes.Equal(got, content) || len(repository.audits) != 1 || repository.audits[0].Entry.Action != audit.ActionFileRead {
		t.Fatalf("verified read = %#v, %q, audits %#v", file, got, repository.audits)
	}
}

func TestReadVerifiedRejectsUnavailableOrMismatchedContent(t *testing.T) {
	repository := newMemoryFileRepository()
	repository.files["pending"] = File{ID: "pending", ObjectKey: "system/pending", Status: StatusPending}
	repository.files["mismatch"] = File{ID: "mismatch", ObjectKey: "system/mismatch", Status: StatusAvailable, SizeBytes: 3, SHA256: strings.Repeat("0", 64)}
	objects := newMemoryObjectStorage()
	objects.data["system/mismatch"] = []byte("abc")
	service := NewService(repository, objects, 1024)

	if _, _, err := service.ReadVerified(context.Background(), "pending", audit.Metadata{}); !errors.Is(err, ErrFileUnavailable) {
		t.Fatalf("pending error = %v", err)
	}
	if _, _, err := service.ReadVerified(context.Background(), "mismatch", audit.Metadata{}); !errors.Is(err, ErrIntegrityMismatch) {
		t.Fatalf("mismatch error = %v", err)
	}
	if len(repository.missing) != 1 || repository.missing[0].FailureCode != FailureIntegrityMismatch {
		t.Fatalf("integrity transition = %#v", repository.missing)
	}
}

func TestReconcileRepairsStalePendingAndCleansFailedObject(t *testing.T) {
	repository := newMemoryFileRepository()
	pending := File{ID: "pending", Bucket: "private-files", ObjectKey: "system/pending", OriginalName: "p.txt", SizeBytes: 3, Status: StatusPending}
	failed := File{ID: "failed", Bucket: "private-files", ObjectKey: "system/failed", Status: StatusFailed}
	repository.files[pending.ID], repository.files[failed.ID] = pending, failed
	repository.candidates = []File{pending, failed}
	objects := newMemoryObjectStorage()
	objects.data[pending.ObjectKey] = []byte("abc")
	objects.data[failed.ObjectKey] = []byte("partial")
	service := NewService(repository, objects, 1024)

	result, err := service.Reconcile(context.Background(), time.Now(), 10, audit.Metadata{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Recovered != 1 || result.Cleaned != 1 || len(repository.available) != 1 {
		t.Fatalf("reconcile result = %#v, available = %#v", result, repository.available)
	}
	if len(objects.removed) != 1 || objects.removed[0] != failed.ObjectKey {
		t.Fatalf("cleanup keys = %#v", objects.removed)
	}
}
