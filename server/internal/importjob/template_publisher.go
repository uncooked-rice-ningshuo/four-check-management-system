package importjob

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/storage"
)

var ErrTemplatePublicationConflict = errors.New("existing template requires explicit migration")

// TemplatePublicationRepository enforces actor, file and publication invariants.
// File upload is separate from the atomic template/audit transaction so failures
// can be retried using a verified, retained file instead of rewriting history.
type TemplatePublicationRepository interface {
	CheckTemplatePublisher(context.Context, string) error
	FindInspectionTemplate(context.Context, string) (Template, error)
	FindReusableTemplateFile(context.Context, string) (storage.File, error)
	PublishInspectionTemplate(context.Context, string, storage.File, []TemplateField, audit.Metadata) (Template, error)
}

type AttendanceTemplatePublisher struct {
	typeCode   string
	repository TemplatePublicationRepository
	files      *storage.Service
}

func NewAttendanceTemplatePublisher(repository TemplatePublicationRepository, files *storage.Service) *AttendanceTemplatePublisher {
	return &AttendanceTemplatePublisher{repository: repository, files: files, typeCode: "ATTENDANCE_RATE"}
}

// NewCourseTemplatePublisher uses an explicit supported inspection type.
func NewCourseTemplatePublisher(repository TemplatePublicationRepository, files *storage.Service, typeCode string) *AttendanceTemplatePublisher {
	return &AttendanceTemplatePublisher{repository: repository, files: files, typeCode: typeCode}
}

// NewInspectionTemplatePublisher supports all implemented inspection adapters.
func NewInspectionTemplatePublisher(repository TemplatePublicationRepository, files *storage.Service, typeCode string) *AttendanceTemplatePublisher {
	return &AttendanceTemplatePublisher{repository: repository, files: files, typeCode: typeCode}
}

// Publish installs only the supported v1 template, never overwriting an existing
// version. Even an idempotent retry rereads and verifies the private object.
func (p *AttendanceTemplatePublisher) Publish(ctx context.Context, metadata audit.Metadata) (Template, error) {
	definition, err := InspectionTemplate(p.typeCode)
	if err != nil {
		return Template{}, err
	}
	if err := p.repository.CheckTemplatePublisher(ctx, metadata.ActorID); err != nil {
		return Template{}, err
	}
	if strings.TrimSpace(metadata.Reason) == "" {
		metadata.Reason = "初始化" + definition.Name + " v1"
	}
	content, err := BuildInspectionTemplate(p.typeCode)
	if err != nil {
		return Template{}, err
	}
	parser, err := NewInspectionXLSXParser(p.typeCode, 16<<20)
	if err != nil {
		return Template{}, err
	}
	if _, err := parser.Parse(definition.Filename, content); err != nil {
		return Template{}, err
	}
	existing, err := p.repository.FindInspectionTemplate(ctx, p.typeCode)
	if err == nil {
		if existing.Code != definition.Code || existing.Version != 1 || existing.Status != "PUBLISHED" {
			return Template{}, ErrTemplatePublicationConflict
		}
		return p.verify(ctx, existing, content, metadata)
	}
	if !errors.Is(err, ErrTemplateNotFound) {
		return Template{}, err
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(content))
	file, err := p.repository.FindReusableTemplateFile(ctx, digest)
	if errors.Is(err, storage.ErrFileNotFound) {
		file, err = p.files.Store(ctx, storage.StoreInput{OriginalName: definition.Filename, DeclaredContentType: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", Size: int64(len(content)), RetentionType: storage.RetentionTemplate, CreatedBy: metadata.ActorID, Body: bytes.NewReader(content)}, metadata)
	}
	if err != nil {
		return Template{}, err
	}
	if _, stored, err := p.files.ReadVerified(ctx, file.ID, metadata); err != nil {
		return Template{}, err
	} else if !bytes.Equal(stored, content) {
		return Template{}, ErrTemplatePublicationConflict
	}
	published, err := p.repository.PublishInspectionTemplate(ctx, p.typeCode, file, parser.config.Fields, metadata)
	if err != nil {
		return Template{}, err
	}
	return p.verify(ctx, published, content, metadata)
}

func (p *AttendanceTemplatePublisher) verify(ctx context.Context, template Template, expected []byte, metadata audit.Metadata) (Template, error) {
	file, content, err := p.files.ReadVerified(ctx, template.FileID, metadata)
	if err != nil {
		return Template{}, err
	}
	if file.RetentionType != storage.RetentionTemplate || !bytes.Equal(content, expected) {
		return Template{}, ErrTemplatePublicationConflict
	}
	return template, nil
}
