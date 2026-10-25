package workflow

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/permission"
	"github.com/google/uuid"
)

var (
	ErrInvalidSubmit     = errors.New("invalid department submission")
	ErrSubmissionScope   = errors.New("submission scope unavailable")
	ErrRecordUnavailable = errors.New("submission record unavailable")
	ErrVersionConflict   = errors.New("submission version conflict")
	ErrIdempotencyReuse  = errors.New("submission idempotency key reused with different records")
	ErrSubmissionMissing = errors.New("department submission not found")
	ErrInvalidDecision   = errors.New("invalid submission decision")
)

// SubmissionKind distinguishes an ordinary record batch from an explicit
// attestation that a configured reporting cell was checked and had no rows.
type SubmissionKind string

const (
	SubmissionKindRecords    SubmissionKind = "RECORDS"
	SubmissionKindZeroRecord SubmissionKind = "ZERO_RECORD"
)

// Scope fixes a submission batch to one week and inspection type. An empty
// DepartmentID denotes an independent college-level batch.
type Scope struct {
	SemesterID       string `json:"semesterId"`
	WeekID           string `json:"weekId"`
	DepartmentID     string `json:"departmentId"`
	InspectionTypeID string `json:"inspectionTypeId"`
}

type VersionedRecord struct {
	ID      string `json:"id"`
	Version int    `json:"version"`
}

type SubmitInput struct {
	Scope          Scope             `json:"scope"`
	Kind           SubmissionKind    `json:"kind"`
	Records        []VersionedRecord `json:"records"`
	IdempotencyKey string            `json:"idempotencyKey"`
	Explanation    string            `json:"explanation"`
}

type Submission struct {
	ID             string             `json:"id"`
	Scope          Scope              `json:"scope"`
	SemesterName   string             `json:"semesterName,omitempty"`
	WeekNo         int                `json:"weekNo,omitempty"`
	DepartmentName string             `json:"departmentName,omitempty"`
	Status         State              `json:"status"`
	Kind           SubmissionKind     `json:"kind"`
	RevisionNo     int                `json:"revisionNo"`
	RowVersion     int                `json:"rowVersion"`
	CreatedBy      string             `json:"createdBy"`
	SubmittedBy    string             `json:"submittedBy"`
	ReviewedBy     string             `json:"reviewedBy,omitempty"`
	ReturnReason   string             `json:"returnReason,omitempty"`
	CreatedAt      time.Time          `json:"createdAt"`
	SubmittedAt    time.Time          `json:"submittedAt"`
	ReviewedAt     *time.Time         `json:"reviewedAt,omitempty"`
	RecordIDs      []string           `json:"recordIds"`
	Records        []SubmissionRecord `json:"records"`
}

type SubmissionRecord struct {
	ID         string `json:"id"`
	RowVersion int    `json:"rowVersion"`
	Status     string `json:"status"`
	CreatedBy  string `json:"createdBy"`
}

type ListQuery struct {
	CollegeOnly      bool   `form:"-"`
	SemesterID       string `form:"semesterId"`
	WeekID           string `form:"weekId"`
	DepartmentID     string `form:"departmentId"`
	InspectionTypeID string `form:"inspectionTypeId"`
	Status           State  `form:"status"`
	Page             int    `form:"page"`
	PageSize         int    `form:"pageSize"`
}

type Page struct {
	Items    []Submission `json:"items"`
	Total    int          `json:"total"`
	Page     int          `json:"page"`
	PageSize int          `json:"pageSize"`
}

type ViewAccess struct {
	ActorID      string
	Unrestricted bool
	Permissions  []permission.EffectivePermission
}

type Actor struct {
	ID           string
	IsSuperAdmin bool
}

// SubmitCommand carries validated input and the actor's effective rules to the
// repository, which applies them inside SQL before locking each record.
type SubmitCommand struct {
	ActorID        string
	Unrestricted   bool
	Permissions    []permission.EffectivePermission
	Scope          Scope
	Kind           SubmissionKind
	Records        []VersionedRecord
	IdempotencyKey string
	Fingerprint    string
	Explanation    string
	Metadata       audit.Metadata
}

type Repository interface {
	SubmitDepartmentBatch(context.Context, SubmitCommand) (Submission, error)
	DecideDepartmentBatch(context.Context, DecisionCommand) (Submission, error)
	ResubmitDepartmentBatch(context.Context, ResubmitCommand) (Submission, error)
	ListDepartmentBatches(context.Context, ViewAccess, ListQuery) (Page, error)
	GetDepartmentBatch(context.Context, ViewAccess, string) (Submission, error)
}

type ResubmitInput struct {
	Version     int               `json:"version"`
	Kind        SubmissionKind    `json:"kind"`
	Records     []VersionedRecord `json:"records"`
	Explanation string            `json:"explanation"`
}

type ResubmitCommand struct {
	ActorID      string
	Unrestricted bool
	Permissions  []permission.EffectivePermission
	SubmissionID string
	Version      int
	Kind         SubmissionKind
	Records      []VersionedRecord
	Explanation  string
	Metadata     audit.Metadata
}

type DecisionInput struct {
	Version int    `json:"version"`
	Reason  string `json:"reason"`
}

type DecisionCommand struct {
	ActorID      string
	Unrestricted bool
	Permissions  []permission.EffectivePermission
	SubmissionID string
	Version      int
	Action       Action
	Reason       string
	Metadata     audit.Metadata
}

type permissionService interface {
	Effective(context.Context, string) (permission.EffectiveResult, error)
}

type Service struct {
	repository  Repository
	permissions permissionService
}

func NewService(repository Repository, permissions permissionService) *Service {
	return &Service{repository: repository, permissions: permissions}
}

// Submit validates the batch envelope and delegates its atomic, scoped record
// lock, state change, revision snapshot and audit to the repository.
func (s *Service) Submit(ctx context.Context, actor Actor, input SubmitInput, metadata audit.Metadata) (Submission, error) {
	if _, err := uuid.Parse(actor.ID); err != nil {
		return Submission{}, ErrInvalidSubmit
	}
	for _, value := range []*string{&input.Scope.SemesterID, &input.Scope.WeekID, &input.Scope.InspectionTypeID} {
		id, err := uuid.Parse(*value)
		if err != nil {
			return Submission{}, ErrInvalidSubmit
		}
		*value = id.String()
	}
	if input.Scope.DepartmentID == inspection.CollegeDepartmentOptionID {
		input.Scope.DepartmentID = ""
	}
	if input.Scope.DepartmentID != "" {
		id, err := uuid.Parse(input.Scope.DepartmentID)
		if err != nil {
			return Submission{}, ErrInvalidSubmit
		}
		input.Scope.DepartmentID = id.String()
	}
	if input.Kind == "" {
		input.Kind = SubmissionKindRecords
	}
	if input.Kind != SubmissionKindRecords && input.Kind != SubmissionKindZeroRecord {
		return Submission{}, ErrInvalidSubmit
	}
	input.Explanation = strings.TrimSpace(input.Explanation)
	input.IdempotencyKey = strings.TrimSpace(input.IdempotencyKey)
	if len(input.IdempotencyKey) == 0 || len(input.IdempotencyKey) > 128 || len(input.Explanation) > 500 ||
		len(input.Records) > 100 || (input.Kind == SubmissionKindRecords && len(input.Records) == 0) ||
		(input.Kind == SubmissionKindZeroRecord && len(input.Records) != 0) {
		return Submission{}, ErrInvalidSubmit
	}
	records := append([]VersionedRecord(nil), input.Records...)
	for index := range records {
		id, err := uuid.Parse(records[index].ID)
		if err != nil || records[index].Version < 1 {
			return Submission{}, ErrInvalidSubmit
		}
		records[index].ID = id.String()
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	for index := 1; index < len(records); index++ {
		if records[index].ID == records[index-1].ID {
			return Submission{}, ErrInvalidSubmit
		}
	}
	permissions := []permission.EffectivePermission(nil)
	if !actor.IsSuperAdmin {
		if s.permissions == nil {
			return Submission{}, ErrSubmissionScope
		}
		effective, err := s.permissions.Effective(ctx, actor.ID)
		if err != nil {
			return Submission{}, err
		}
		permissions = effective.Permissions
	}
	hash := sha256.New()
	fmt.Fprintf(hash, "%s/%s/%s/%s/%s/%s", input.Kind, input.Scope.SemesterID, input.Scope.WeekID,
		input.Scope.DepartmentID, input.Scope.InspectionTypeID, input.Explanation)
	for _, record := range records {
		fmt.Fprintf(hash, "/%s:%d", record.ID, record.Version)
	}
	metadata.ActorID = actor.ID
	metadata.Reason = input.Explanation
	return s.repository.SubmitDepartmentBatch(ctx, SubmitCommand{ActorID: actor.ID, Unrestricted: actor.IsSuperAdmin,
		Permissions: permissions, Scope: input.Scope, Kind: input.Kind, Records: records, IdempotencyKey: input.IdempotencyKey,
		Explanation: input.Explanation,
		Fingerprint: hex.EncodeToString(hash.Sum(nil)), Metadata: metadata})
}

// Decide applies an explicit review transition to a whole submission batch.
func (s *Service) Decide(ctx context.Context, actor Actor, id string, action Action, input DecisionInput, metadata audit.Metadata) (Submission, error) {
	actorID, actorErr := uuid.Parse(actor.ID)
	submissionID, idErr := uuid.Parse(id)
	reason := strings.TrimSpace(input.Reason)
	if actorErr != nil || idErr != nil || input.Version < 1 || len(reason) > 500 ||
		(action != Return && action != Confirm) || (action == Return && reason == "") {
		return Submission{}, ErrInvalidDecision
	}
	permissions := []permission.EffectivePermission(nil)
	if !actor.IsSuperAdmin {
		if s.permissions == nil {
			return Submission{}, ErrSubmissionScope
		}
		effective, err := s.permissions.Effective(ctx, actorID.String())
		if err != nil {
			return Submission{}, err
		}
		permissions = effective.Permissions
	}
	metadata.ActorID, metadata.Reason = actorID.String(), reason
	return s.repository.DecideDepartmentBatch(ctx, DecisionCommand{ActorID: actorID.String(), Unrestricted: actor.IsSuperAdmin,
		Permissions: permissions, SubmissionID: submissionID.String(), Version: input.Version, Action: action,
		Reason: reason, Metadata: metadata})
}

// Resubmit retains the batch identity and advances its revision using every
// returned record's current version. The reviewer may permit an unchanged retry.
func (s *Service) Resubmit(ctx context.Context, actor Actor, id string, input ResubmitInput, metadata audit.Metadata) (Submission, error) {
	actorID, actorErr := uuid.Parse(actor.ID)
	submissionID, idErr := uuid.Parse(id)
	if input.Kind == "" {
		input.Kind = SubmissionKindRecords
	}
	input.Explanation = strings.TrimSpace(input.Explanation)
	if actorErr != nil || idErr != nil || input.Version < 1 || len(input.Explanation) > 500 || len(input.Records) > 100 ||
		(input.Kind != SubmissionKindRecords && input.Kind != SubmissionKindZeroRecord) ||
		(input.Kind == SubmissionKindRecords && len(input.Records) == 0) ||
		(input.Kind == SubmissionKindZeroRecord && len(input.Records) != 0) {
		return Submission{}, ErrInvalidSubmit
	}
	records := append([]VersionedRecord(nil), input.Records...)
	for index := range records {
		parsed, err := uuid.Parse(records[index].ID)
		if err != nil || records[index].Version < 1 {
			return Submission{}, ErrInvalidSubmit
		}
		records[index].ID = parsed.String()
	}
	sort.Slice(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	for index := 1; index < len(records); index++ {
		if records[index].ID == records[index-1].ID {
			return Submission{}, ErrInvalidSubmit
		}
	}
	permissions := []permission.EffectivePermission(nil)
	if !actor.IsSuperAdmin {
		if s.permissions == nil {
			return Submission{}, ErrSubmissionScope
		}
		effective, err := s.permissions.Effective(ctx, actorID.String())
		if err != nil {
			return Submission{}, err
		}
		permissions = effective.Permissions
	}
	metadata.ActorID, metadata.Reason = actorID.String(), input.Explanation
	return s.repository.ResubmitDepartmentBatch(ctx, ResubmitCommand{ActorID: actorID.String(), Unrestricted: actor.IsSuperAdmin,
		Permissions: permissions, SubmissionID: submissionID.String(), Version: input.Version, Kind: input.Kind,
		Records: records, Explanation: input.Explanation, Metadata: metadata})
}

func (s *Service) viewAccess(ctx context.Context, actor Actor) (ViewAccess, error) {
	actorID, err := uuid.Parse(actor.ID)
	if err != nil {
		return ViewAccess{}, ErrInvalidSubmit
	}
	access := ViewAccess{ActorID: actorID.String(), Unrestricted: actor.IsSuperAdmin}
	if !actor.IsSuperAdmin {
		if s.permissions == nil {
			return ViewAccess{}, ErrSubmissionScope
		}
		effective, err := s.permissions.Effective(ctx, actorID.String())
		if err != nil {
			return ViewAccess{}, err
		}
		access.Permissions = effective.Permissions
	}
	return access, nil
}

// List returns only batches whose entire record set remains visible to the actor.
func (s *Service) List(ctx context.Context, actor Actor, query ListQuery) (Page, error) {
	if query.DepartmentID == inspection.CollegeDepartmentOptionID {
		query.DepartmentID = ""
		query.CollegeOnly = true
	}
	for _, value := range []*string{&query.SemesterID, &query.WeekID, &query.DepartmentID, &query.InspectionTypeID} {
		if *value == "" {
			continue
		}
		id, err := uuid.Parse(*value)
		if err != nil {
			return Page{}, ErrInvalidSubmit
		}
		*value = id.String()
	}
	if query.Status != "" && query.Status != Draft && query.Status != PendingReview && query.Status != Returned && query.Status != Confirmed {
		return Page{}, ErrInvalidSubmit
	}
	if query.Page == 0 {
		query.Page = 1
	}
	if query.PageSize == 0 {
		query.PageSize = 20
	}
	if query.Page < 1 || query.Page > 1000000 || query.PageSize < 1 || query.PageSize > 100 {
		return Page{}, ErrInvalidSubmit
	}
	access, err := s.viewAccess(ctx, actor)
	if err != nil {
		return Page{}, err
	}
	return s.repository.ListDepartmentBatches(ctx, access, query)
}

func (s *Service) Detail(ctx context.Context, actor Actor, id string) (Submission, error) {
	submissionID, err := uuid.Parse(id)
	if err != nil {
		return Submission{}, ErrInvalidSubmit
	}
	access, err := s.viewAccess(ctx, actor)
	if err != nil {
		return Submission{}, err
	}
	return s.repository.GetDepartmentBatch(ctx, access, submissionID.String())
}
