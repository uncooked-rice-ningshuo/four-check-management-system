package inspection

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"strings"
	"unicode/utf8"
)

var (
	ErrRecordNotFound        = errors.New("record missing or outside scope")
	ErrRecordVersionConflict = errors.New("record version conflict")
	ErrRecordNotEditable     = errors.New("record is not an unsubmitted draft")
)

// MutationAccess is compiled from server-side permissions, never request JSON.
type MutationAccess struct {
	ActorID      string
	Unrestricted bool
	Permissions  []permission.EffectivePermission
}

// VersionedRecord identifies one optimistic mutation target.
type VersionedRecord struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

// AttendanceEditInput requires a reason and a full validated replacement payload.
type AttendanceEditInput struct {
	Version int                   `json:"version"`
	Reason  string                `json:"reason"`
	Record  ManualAttendanceInput `json:"record"`
}

// DeleteAttendanceInput is all-or-nothing, including for a single record.
type DeleteAttendanceInput struct {
	Items  []VersionedRecord `json:"items"`
	Reason string            `json:"reason"`
}
type mutationRepository interface {
	ReadAttendanceForMutation(context.Context, string, MutationAccess) (AttendanceRecord, error)
	UpdateAttendance(context.Context, AttendanceRecord, int, MutationAccess, audit.Mutation) (AttendanceRecord, error)
	DeleteAttendance(context.Context, []VersionedRecord, MutationAccess, audit.Metadata) error
}

// MutationService protects original and destination scopes independently.
type MutationService struct {
	repository  mutationRepository
	manual      *ManualService
	permissions manualPermissions
}

func NewMutationService(r mutationRepository, m *ManualService, p manualPermissions) *MutationService {
	return &MutationService{r, m, p}
}
func (s *MutationService) access(ctx context.Context, actor ManualActor) (MutationAccess, error) {
	a := MutationAccess{ActorID: actor.ID, Unrestricted: actor.IsSuperAdmin}
	if actor.ID == "" {
		return a, ErrScopeMismatch
	}
	if actor.IsSuperAdmin {
		return a, nil
	}
	p, err := s.permissions.Effective(ctx, actor.ID)
	a.Permissions = p.Permissions
	return a, err
}
func (s *MutationService) Detail(ctx context.Context, actor ManualActor, id string) (AttendanceRecord, error) {
	a, err := s.access(ctx, actor)
	if err != nil {
		return AttendanceRecord{}, err
	}
	return s.repository.ReadAttendanceForMutation(ctx, id, a)
}
func (s *MutationService) Update(ctx context.Context, actor ManualActor, id string, input AttendanceEditInput, metadata audit.Metadata) (AttendanceRecord, error) {
	if input.Version < 1 || !validMutationReason(input.Reason) {
		return AttendanceRecord{}, ErrInvalidAttendanceRecord
	}
	access, err := s.access(ctx, actor)
	if err != nil {
		return AttendanceRecord{}, err
	}
	original, err := s.repository.ReadAttendanceForMutation(ctx, id, access)
	if err != nil {
		return AttendanceRecord{}, err
	}
	if err = validateMutationRecord(original, input.Version); err != nil {
		return AttendanceRecord{}, err
	}
	input.Record.ExcludeRecordID = original.ID
	updated, err := s.manual.prepare(ctx, actor, input.Record, original.CreatedBy)
	if err != nil {
		return AttendanceRecord{}, err
	}
	updated.ID, updated.CreatedBy, updated.Source, updated.ImportBatchID = original.ID, original.CreatedBy, original.Source, original.ImportBatchID
	updated.Status, updated.RowVersion, updated.CreatedAt = original.Status, original.RowVersion+1, original.CreatedAt
	metadata.ActorID, metadata.Reason = actor.ID, strings.TrimSpace(input.Reason)
	m := audit.Mutation{Metadata: metadata, Entry: audit.Entry{Action: audit.ActionInspectionRecordUpdated, ResourceType: "inspectionRecord", ResourceID: id, Before: MutationAuditSnapshot(original), After: MutationAuditSnapshot(updated)}}
	return s.repository.UpdateAttendance(ctx, updated, input.Version, access, m)
}
func (s *MutationService) Delete(ctx context.Context, actor ManualActor, input DeleteAttendanceInput, metadata audit.Metadata) error {
	if len(input.Items) == 0 || len(input.Items) > 100 || !validMutationReason(input.Reason) {
		return ErrInvalidAttendanceRecord
	}
	seen := map[string]bool{}
	for _, item := range input.Items {
		if item.ID == "" || item.Version < 1 || seen[item.ID] {
			return ErrInvalidAttendanceRecord
		}
		seen[item.ID] = true
	}
	access, err := s.access(ctx, actor)
	if err != nil {
		return err
	}
	metadata.ActorID, metadata.Reason = actor.ID, strings.TrimSpace(input.Reason)
	return s.repository.DeleteAttendance(ctx, input.Items, access, metadata)
}
func validMutationReason(v string) bool {
	return strings.TrimSpace(v) != "" && utf8.RuneCountInString(v) <= 255
}
func validateMutationRecord(r AttendanceRecord, version int) error {
	if r.Status != RecordStatusDraft || r.HasSubmission || r.HasReportLock {
		return ErrRecordNotEditable
	}
	if r.RowVersion != version {
		return ErrRecordVersionConflict
	}
	return nil
}

// ValidateMutationRecord repeats the state/version invariant under row locks.
func ValidateMutationRecord(r AttendanceRecord, version int) error {
	return validateMutationRecord(r, version)
}

// MutationAuditSnapshot keeps reproducible business changes without personal names/numbers.
func MutationAuditSnapshot(r AttendanceRecord) map[string]any {
	return map[string]any{
		"studentId": r.StudentID, "classId": r.ClassID, "grade": r.GradeSnapshot, "courseId": r.CourseID, "classroomId": r.ClassroomID,
		"semesterId": r.SemesterID, "weekId": r.WeekID, "departmentId": r.DepartmentID, "inspectionTypeId": r.InspectionTypeID,
		"occurredAt": r.OccurredAt, "absenceHours": r.AbsenceHours, "period": r.PeriodText, "remark": r.Remark,
		"source": r.Source, "status": r.Status, "rowVersion": r.RowVersion, "importBatchId": r.ImportBatchID,
	}
}
