package masterdata

import (
	"context"
	"errors"
	"strings"
	"time"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

const (
	KindFaculty   = "faculties"
	KindMajor     = "majors"
	KindStaff     = "staff"
	KindClass     = "classes"
	KindDormitory = "dormitories"
	KindClassroom = "classrooms"
	KindCourse    = "courses"
	KindStudent   = "students"
)

var (
	ErrInvalidKind     = errors.New("invalid master data kind")
	ErrInvalidRecord   = errors.New("invalid master data record")
	ErrInvalidRelation = errors.New("invalid master data relation")
	ErrImmutableCode   = errors.New("master data code is immutable")
	ErrNotFound        = errors.New("master data record not found")
)

type Record struct {
	ID               string    `json:"id"`
	Kind             string    `json:"kind"`
	Code             string    `json:"code"`
	Name             string    `json:"name"`
	Enabled          bool      `json:"enabled"`
	FacultyID        string    `json:"facultyId,omitempty"`
	MajorID          string    `json:"majorId,omitempty"`
	HeadTeacherID    string    `json:"headTeacherId,omitempty"`
	TeacherID        string    `json:"teacherId,omitempty"`
	ClassID          string    `json:"classId,omitempty"`
	DormitoryID      string    `json:"dormitoryId,omitempty"`
	ClassroomID      string    `json:"classroomId,omitempty"`
	Gender           string    `json:"gender,omitempty"`
	Phone            string    `json:"phone,omitempty"`
	Grade            string    `json:"grade,omitempty"`
	EnrolledSize     int       `json:"enrolledSize,omitempty"`
	CampusArea       string    `json:"campusArea,omitempty"`
	Building         string    `json:"building,omitempty"`
	BuildingNo       string    `json:"buildingNo,omitempty"`
	RoomNo           string    `json:"roomNo,omitempty"`
	Floor            int       `json:"floor,omitempty"`
	ScheduleText     string    `json:"scheduleText,omitempty"`
	EnrollmentStatus string    `json:"enrollmentStatus,omitempty"`
	CreatedAt        time.Time `json:"createdAt,omitempty"`
	UpdatedAt        time.Time `json:"updatedAt,omitempty"`
}

type Query struct {
	Kind     string
	Keyword  string
	Enabled  *bool
	Page     int
	PageSize int
}

type Page struct {
	Items    []Record `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

type Change struct {
	ID         string    `json:"id"`
	EntityType string    `json:"entityType"`
	EntityID   string    `json:"entityId"`
	Action     string    `json:"action"`
	BeforeJSON []byte    `json:"beforeJson,omitempty"`
	AfterJSON  []byte    `json:"afterJson,omitempty"`
	ActorID    string    `json:"actorId"`
	CreatedAt  time.Time `json:"createdAt"`
}

type Repository interface {
	ListMasterData(context.Context, Query) (Page, error)
	FindMasterData(context.Context, string, string) (Record, error)
	MasterDataExists(context.Context, string, string) (bool, error)
	CreateMasterData(context.Context, string, Record, audit.Mutation) (Record, error)
	UpdateMasterData(context.Context, string, Record, audit.Mutation) (Record, error)
	ListMasterDataChanges(context.Context, string, string) ([]Change, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, query Query) (Page, error) {
	if !validKind(query.Kind) {
		return Page{}, ErrInvalidKind
	}
	query.Keyword = strings.TrimSpace(query.Keyword)
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 20
	}
	page, err := s.repository.ListMasterData(ctx, query)
	if err != nil {
		return Page{}, err
	}
	if page.Items == nil {
		page.Items = make([]Record, 0)
	}
	return page, nil
}

func (s *Service) Create(ctx context.Context, actorID string, record Record, metadata ...audit.Metadata) (Record, error) {
	record.Kind = strings.TrimSpace(record.Kind)
	record.Code = strings.ToUpper(strings.TrimSpace(record.Code))
	record.Name = strings.TrimSpace(record.Name)
	normalize(&record)
	if !validKind(record.Kind) {
		return Record{}, ErrInvalidKind
	}
	if !validRecord(record) {
		return Record{}, ErrInvalidRecord
	}
	if err := s.validateRelations(ctx, record); err != nil {
		return Record{}, err
	}
	record.ID = uuid.NewString()
	record.Enabled = true
	return s.repository.CreateMasterData(ctx, actorID, record, masterDataMutation(metadata, actorID, record.ID, nil, record))
}

func (s *Service) Update(ctx context.Context, actorID string, record Record, metadata ...audit.Metadata) (Record, error) {
	if !validKind(record.Kind) || record.ID == "" {
		return Record{}, ErrInvalidKind
	}
	existing, err := s.repository.FindMasterData(ctx, record.Kind, record.ID)
	if err != nil {
		return Record{}, err
	}
	record.Code = strings.ToUpper(strings.TrimSpace(record.Code))
	record.Name = strings.TrimSpace(record.Name)
	normalize(&record)
	if record.Code != existing.Code {
		return Record{}, ErrImmutableCode
	}
	if !validRecord(record) {
		return Record{}, ErrInvalidRecord
	}
	if err := s.validateRelations(ctx, record); err != nil {
		return Record{}, err
	}
	return s.repository.UpdateMasterData(ctx, actorID, record, masterDataMutation(metadata, actorID, record.ID, existing, record))
}

func masterDataMutation(metadata []audit.Metadata, actorID, resourceID string, before, after any) audit.Mutation {
	request := audit.Metadata{}
	if len(metadata) > 0 {
		request = metadata[0]
	}
	if request.ActorID == "" {
		request.ActorID = actorID
	}
	return audit.Mutation{Metadata: request, Entry: audit.Entry{
		Action: audit.ActionMasterDataChanged, ResourceType: "masterData", ResourceID: resourceID, Before: before, After: after,
	}}
}

func (s *Service) ListChanges(ctx context.Context, kind, id string) ([]Change, error) {
	if !validKind(kind) || id == "" {
		return nil, ErrInvalidKind
	}
	return s.repository.ListMasterDataChanges(ctx, kind, id)
}

func (s *Service) validateRelations(ctx context.Context, record Record) error {
	relations := []struct {
		kind, id string
		required bool
	}{
		{KindFaculty, record.FacultyID, record.Kind == KindMajor},
		{KindMajor, record.MajorID, false},
		{KindStaff, record.HeadTeacherID, false},
		{KindStaff, record.TeacherID, false},
		{KindClass, record.ClassID, false},
		{KindDormitory, record.DormitoryID, false},
		{KindClassroom, record.ClassroomID, false},
	}
	for _, relation := range relations {
		if relation.id == "" {
			if relation.required {
				return ErrInvalidRelation
			}
			continue
		}
		exists, err := s.repository.MasterDataExists(ctx, relation.kind, relation.id)
		if err != nil {
			return err
		}
		if !exists {
			return ErrInvalidRelation
		}
	}
	return nil
}

func validKind(kind string) bool {
	switch kind {
	case KindFaculty, KindMajor, KindStaff, KindClass, KindDormitory, KindClassroom, KindCourse, KindStudent:
		return true
	default:
		return false
	}
}

func validRecord(record Record) bool {
	if record.Code == "" || record.Name == "" {
		return false
	}
	switch record.Kind {
	case KindClass:
		return record.Grade != "" && record.EnrolledSize >= 0
	case KindDormitory:
		return record.CampusArea != "" && record.BuildingNo != "" && record.RoomNo != ""
	case KindClassroom:
		return record.Building != "" && record.RoomNo != "" && record.Floor >= 0
	case KindStudent:
		return record.Grade != ""
	default:
		return true
	}
}

func normalize(record *Record) {
	record.Gender = strings.ToUpper(strings.TrimSpace(record.Gender))
	record.Phone = strings.TrimSpace(record.Phone)
	record.Grade = strings.TrimSpace(record.Grade)
	record.CampusArea = strings.TrimSpace(record.CampusArea)
	record.Building = strings.TrimSpace(record.Building)
	record.BuildingNo = strings.TrimSpace(record.BuildingNo)
	record.RoomNo = strings.TrimSpace(record.RoomNo)
	record.ScheduleText = strings.TrimSpace(record.ScheduleText)
	record.EnrollmentStatus = strings.ToUpper(strings.TrimSpace(record.EnrollmentStatus))
	if record.Gender == "" {
		record.Gender = "UNKNOWN"
	}
	if record.Kind == KindStudent && record.EnrollmentStatus == "" {
		record.EnrollmentStatus = "ENROLLED"
	}
}
