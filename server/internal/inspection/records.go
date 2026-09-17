package inspection

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
)

// InspectionInput contains client fields only. Type code and snapshots are resolved by the server.
type InspectionInput struct {
	SemesterID       string          `json:"semesterId"`
	WeekID           string          `json:"weekId"`
	InspectionTypeID string          `json:"inspectionTypeId"`
	DepartmentID     string          `json:"departmentId"`
	Details          json.RawMessage `json:"details"`
}

// InspectionEdit replaces business fields without changing provenance or inspection type.
type InspectionEdit struct {
	Version int             `json:"version"`
	Reason  string          `json:"reason"`
	Record  InspectionInput `json:"record"`
}

// InspectionRecord is the shared API aggregate, with typed details and immutable provenance.
type InspectionRecord struct {
	ID               string               `json:"id"`
	SemesterID       string               `json:"semesterId"`
	WeekID           string               `json:"weekId"`
	InspectionTypeID string               `json:"inspectionTypeId"`
	TypeCode         string               `json:"typeCode"`
	DepartmentID     string               `json:"departmentId"`
	Details          InspectionDetails    `json:"details"`
	Status           string               `json:"status"`
	Source           string               `json:"source"`
	ImportBatchID    string               `json:"importBatchId,omitempty"`
	RowVersion       int                  `json:"rowVersion"`
	CreatedBy        string               `json:"createdBy"`
	CreatedAt        time.Time            `json:"createdAt"`
	UpdatedAt        time.Time            `json:"updatedAt"`
	HasSubmission    bool                 `json:"-"`
	HasReportLock    bool                 `json:"-"`
	Normalized       ValidatedDetails     `json:"-"`
	References       InspectionReferences `json:"-"`
}

// ClassReference and StudentReference are server-owned snapshots used for scope and persistence.
type ClassReference struct{ ID, Name, Grade string }
type StudentReference struct{ ID, ClassID, Number, Name string }
type InspectionReferences struct {
	Classes  []ClassReference
	Students []StudentReference
}

// InspectionContext is locked in the write transaction to prevent configuration changes during validation.
type InspectionContext struct {
	DetailContext
	TypeCode, AcademicYearID string
}

// InspectionFilter supports bounded, structured list queries.
type InspectionFilter struct {
	SemesterID       string `form:"semesterId"`
	WeekID           string `form:"weekId"`
	InspectionTypeID string `form:"inspectionTypeId"`
	DepartmentID     string `form:"departmentId"`
	ClassID          string `form:"classId"`
	Grade            string `form:"grade"`
	Status           string `form:"status"`
	Source           string `form:"source"`
	Sort             string `form:"sort"`
	Page             int    `form:"page"`
	PageSize         int    `form:"pageSize"`
}
type InspectionPage struct {
	Items    []InspectionRecord `json:"items"`
	Total    int                `json:"total"`
	Page     int                `json:"page"`
	PageSize int                `json:"pageSize"`
}

// DetailsValidationError contains safe domain issues, never the submitted payload.
type DetailsValidationError struct{ Issues []DetailIssue }

func (*DetailsValidationError) Error() string { return "invalid inspection details" }

var ErrInspectionRequest = errors.New("invalid inspection request")

// InspectionTransaction exposes aggregate operations; the service owns transaction orchestration.
type InspectionTransaction interface {
	Context(context.Context, InspectionInput) (InspectionContext, error)
	References(context.Context, InspectionRecord, InspectionContext, MutationAccess, string) (InspectionReferences, error)
	Read(context.Context, string, MutationAccess, string, bool) (InspectionRecord, error)
	Save(context.Context, InspectionRecord, *InspectionRecord, MutationAccess) (InspectionRecord, error)
	Delete(context.Context, InspectionRecord, MutationAccess) error
	Audit(context.Context, audit.Metadata, audit.Entry) error
}
type InspectionRepository interface {
	WithInspectionTransaction(context.Context, func(InspectionTransaction) error) error
	GetInspection(context.Context, string, MutationAccess) (InspectionRecord, error)
	ListInspections(context.Context, InspectionFilter, MutationAccess) (InspectionPage, error)
}

// RecordService implements scoped CRUD without widening the existing draft mutation rule.
type RecordService struct {
	repository  InspectionRepository
	permissions manualPermissions
}

func NewRecordService(r InspectionRepository, p manualPermissions) *RecordService {
	return &RecordService{r, p}
}
func (s *RecordService) access(ctx context.Context, a ManualActor) (MutationAccess, error) {
	result := MutationAccess{ActorID: a.ID, Unrestricted: a.IsSuperAdmin}
	if _, ok := canonicalDetailID(a.ID); !ok {
		return result, ErrScopeMismatch
	}
	if a.IsSuperAdmin {
		return result, nil
	}
	p, err := s.permissions.Effective(ctx, a.ID)
	result.Permissions = p.Permissions
	return result, err
}
func normalizeInspectionInput(i *InspectionInput) error {
	for _, p := range []*string{&i.SemesterID, &i.WeekID, &i.InspectionTypeID, &i.DepartmentID} {
		v, ok := canonicalDetailID(*p)
		if !ok {
			return ErrInspectionRequest
		}
		*p = v
	}
	return nil
}
func (s *RecordService) prepare(ctx context.Context, tx InspectionTransaction, i InspectionInput, a MutationAccess, owner string) (InspectionRecord, error) {
	c, err := tx.Context(ctx, i)
	if err != nil {
		return InspectionRecord{}, err
	}
	v, issues := ValidateDetails(c.TypeCode, c.DetailContext, i.Details)
	if len(issues) > 0 {
		return InspectionRecord{}, &DetailsValidationError{issues}
	}
	r := InspectionRecord{ID: uuid.NewString(), SemesterID: i.SemesterID, WeekID: i.WeekID, InspectionTypeID: i.InspectionTypeID, TypeCode: c.TypeCode, DepartmentID: i.DepartmentID, Details: v.Details, Normalized: v, Status: RecordStatusDraft, Source: RecordSourceManual, RowVersion: 1, CreatedBy: a.ActorID}
	refs, err := tx.References(ctx, r, c, a, owner)
	if err != nil {
		return InspectionRecord{}, err
	}
	r.References = refs
	return r, nil
}
func (s *RecordService) Create(ctx context.Context, actor ManualActor, i InspectionInput, m audit.Metadata) (InspectionRecord, error) {
	if err := normalizeInspectionInput(&i); err != nil {
		return InspectionRecord{}, err
	}
	a, err := s.access(ctx, actor)
	if err != nil {
		return InspectionRecord{}, err
	}
	var result InspectionRecord
	m.ActorID = actor.ID
	err = s.repository.WithInspectionTransaction(ctx, func(tx InspectionTransaction) error {
		r, e := s.prepare(ctx, tx, i, a, "")
		if e != nil {
			return e
		}
		result, e = tx.Save(ctx, r, nil, a)
		if e != nil {
			return e
		}
		return tx.Audit(ctx, m, audit.Entry{Action: audit.ActionInspectionRecordCreated, ResourceType: "inspectionRecord", ResourceID: result.ID, After: InspectionAuditSnapshot(result, false)})
	})
	if err != nil {
		return InspectionRecord{}, err
	}
	return result, nil
}
func (s *RecordService) Detail(ctx context.Context, actor ManualActor, id string) (InspectionRecord, error) {
	id, ok := canonicalDetailID(id)
	if !ok {
		return InspectionRecord{}, ErrInspectionRequest
	}
	a, err := s.access(ctx, actor)
	if err != nil {
		return InspectionRecord{}, err
	}
	return s.repository.GetInspection(ctx, id, a)
}
func (s *RecordService) List(ctx context.Context, actor ManualActor, f InspectionFilter) (InspectionPage, error) {
	for _, p := range []*string{&f.SemesterID, &f.WeekID, &f.InspectionTypeID, &f.DepartmentID, &f.ClassID} {
		if *p != "" {
			v, ok := canonicalDetailID(*p)
			if !ok {
				return InspectionPage{}, ErrInspectionRequest
			}
			*p = v
		}
	}
	if f.Page == 0 {
		f.Page = 1
	}
	if f.PageSize == 0 {
		f.PageSize = 20
	}
	if f.Sort == "" {
		f.Sort = SortOccurredAtDescending
	}
	if f.Page < 1 || f.Page > 1000000 || f.PageSize < 1 || f.PageSize > 100 || (f.Sort != SortOccurredAtAscending && f.Sort != SortOccurredAtDescending) || !validAttendanceStatus(f.Status) || !validAttendanceSource(f.Source) {
		return InspectionPage{}, ErrInspectionRequest
	}
	a, err := s.access(ctx, actor)
	if err != nil {
		return InspectionPage{}, err
	}
	if !a.Unrestricted {
		allowed := false
		for _, p := range a.Permissions {
			if p.Action == permission.ActionInspectionView && p.Effect == permission.EffectAllow {
				allowed = true
			}
		}
		if !allowed {
			return InspectionPage{}, ErrScopeMismatch
		}
	}
	return s.repository.ListInspections(ctx, f, a)
}
func validateInspectionMutation(r InspectionRecord, version int) error {
	return ValidateMutationRecord(AttendanceRecord{Status: r.Status, RowVersion: r.RowVersion, HasSubmission: r.HasSubmission, HasReportLock: r.HasReportLock}, version)
}
func (s *RecordService) Update(ctx context.Context, actor ManualActor, id string, i InspectionEdit, m audit.Metadata) (InspectionRecord, error) {
	id, ok := canonicalDetailID(id)
	if !ok || i.Version < 1 || !validMutationReason(i.Reason) {
		return InspectionRecord{}, ErrInspectionRequest
	}
	if err := normalizeInspectionInput(&i.Record); err != nil {
		return InspectionRecord{}, err
	}
	a, err := s.access(ctx, actor)
	if err != nil {
		return InspectionRecord{}, err
	}
	var result InspectionRecord
	m.ActorID = actor.ID
	m.Reason = strings.TrimSpace(i.Reason)
	err = s.repository.WithInspectionTransaction(ctx, func(tx InspectionTransaction) error {
		old, e := tx.Read(ctx, id, a, "mutate", true)
		if e != nil {
			return e
		}
		if e = validateInspectionMutation(old, i.Version); e != nil {
			return e
		}
		if old.InspectionTypeID != i.Record.InspectionTypeID {
			return ErrInspectionRequest
		}
		r, e := s.prepare(ctx, tx, i.Record, a, old.CreatedBy)
		if e != nil {
			return e
		}
		r.ID, r.CreatedBy, r.Source, r.ImportBatchID, r.CreatedAt = old.ID, old.CreatedBy, old.Source, old.ImportBatchID, old.CreatedAt
		result, e = tx.Save(ctx, r, &old, a)
		if e != nil {
			return e
		}
		return tx.Audit(ctx, m, audit.Entry{Action: audit.ActionInspectionRecordUpdated, ResourceType: "inspectionRecord", ResourceID: id, Before: InspectionAuditSnapshot(old, false), After: InspectionAuditSnapshot(result, false)})
	})
	if err != nil {
		return InspectionRecord{}, err
	}
	return result, nil
}
func (s *RecordService) Delete(ctx context.Context, actor ManualActor, i DeleteAttendanceInput, m audit.Metadata) error {
	if len(i.Items) < 1 || len(i.Items) > 100 || !validMutationReason(i.Reason) {
		return ErrInspectionRequest
	}
	items := append([]VersionedRecord(nil), i.Items...)
	seen := map[string]bool{}
	for n, item := range items {
		id, ok := canonicalDetailID(item.ID)
		if !ok || item.Version < 1 || seen[id] {
			return ErrInspectionRequest
		}
		seen[id] = true
		items[n].ID = id
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	a, err := s.access(ctx, actor)
	if err != nil {
		return err
	}
	m.ActorID = actor.ID
	m.Reason = strings.TrimSpace(i.Reason)
	return s.repository.WithInspectionTransaction(ctx, func(tx InspectionTransaction) error {
		records := make([]InspectionRecord, 0, len(items))
		for _, item := range items {
			r, e := tx.Read(ctx, item.ID, a, "mutate", true)
			if e != nil {
				return e
			}
			if e = validateInspectionMutation(r, item.Version); e != nil {
				return e
			}
			records = append(records, r)
		}
		for _, r := range records {
			if e := tx.Delete(ctx, r, a); e != nil {
				return e
			}
			after := r
			after.RowVersion++
			if e := tx.Audit(ctx, m, audit.Entry{Action: audit.ActionInspectionRecordDeleted, ResourceType: "inspectionRecord", ResourceID: r.ID, Before: InspectionAuditSnapshot(r, false), After: InspectionAuditSnapshot(after, true)}); e != nil {
				return e
			}
		}
		return nil
	})
}

// InspectionAuditSnapshot retains typed business values while redacting user-entered free text.
func InspectionAuditSnapshot(r InspectionRecord, deleted bool) map[string]any {
	raw, _ := json.Marshal(r.Details)
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	for _, key := range []string{"remark", "description", "period", "round"} {
		if _, ok := fields[key]; ok {
			fields[key] = json.RawMessage(`"[已脱敏]"`)
		}
	}
	return map[string]any{"details": fields, "semesterId": r.SemesterID, "weekId": r.WeekID, "inspectionTypeId": r.InspectionTypeID, "departmentId": r.DepartmentID, "source": r.Source, "status": r.Status, "rowVersion": r.RowVersion, "importBatchId": r.ImportBatchID, "deleted": deleted}
}

// DecodeStoredInspectionDetails decodes trusted storage without revalidating historical references.
func DecodeStoredInspectionDetails(code string, raw []byte) (InspectionDetails, error) {
	a, ok := detailAdapters[code]
	if !ok {
		return nil, ErrUnknownInspectionType
	}
	v := a.newDetails()
	if err := json.Unmarshal(raw, v); err != nil {
		return nil, err
	}
	return v, nil
}

// InspectionDetailReferences extracts identifiers from the closed model set.
func InspectionDetailReferences(d InspectionDetails) (student, class, course, classroom, dorm, area string, h *HygieneDetails, rooms []string) {
	switch v := d.(type) {
	case *AttendanceDetails:
		student, class, course, classroom = v.StudentID, v.ClassID, v.CourseID, v.ClassroomID
	case *UnattendedClassroomDetails:
		student, class, course, classroom = v.StudentID, v.ClassID, v.CourseID, v.ClassroomID
	case *MorningAttendanceDetails:
		student, class = v.StudentID, v.ClassID
	case *LateReturnDetails:
		student, class, dorm = v.StudentID, v.ClassID, v.DormitoryID
	case *DormitoryHygieneDetails:
		dorm, h = v.DormitoryID, &v.HygieneDetails
	case *ClassroomHygieneDetails:
		classroom, area, h = v.ClassroomID, v.AreaID, &v.HygieneDetails
	case *PublicDormitoryHygieneDetails:
		area, h, rooms = v.AreaID, &v.HygieneDetails, v.ResponsibleDormitoryIDs
	}
	return
}

// AllowsInspectionScope is evaluated for every responsibility class on writes.
func AllowsInspectionScope(a MutationAccess, scope permission.Scope, owner string) bool {
	if a.Unrestricted {
		return true
	}
	p := permission.EffectiveResult{Permissions: a.Permissions}
	if owner == "" {
		return p.Allowed(permission.ActionInspectionEnter, scope)
	}
	return (owner == a.ActorID && p.Allowed(permission.ActionInspectionEnter, scope)) || p.Allowed(permission.ActionInspectionManage, scope)
}
