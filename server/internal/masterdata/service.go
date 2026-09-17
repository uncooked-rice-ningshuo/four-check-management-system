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
	KindFaculty           = "faculties"
	KindGrade             = "grades"
	KindDormitoryArea     = "dormitory-areas"
	KindDormitoryBuilding = "dormitory-buildings"
	KindTeachingBuilding  = "teaching-buildings"
	KindMajor             = "majors"
	KindStaff             = "staff"
	KindClass             = "classes"
	KindDormitory         = "dormitories"
	KindClassroom         = "classrooms"
	KindCourse            = "courses"
	KindStudent           = "students"
)

var (
	ErrInvalidKind     = errors.New("invalid master data kind")
	ErrInvalidRecord   = errors.New("invalid master data record")
	ErrInvalidRelation = errors.New("invalid master data relation")
	ErrInvalidSchedule = errors.New("invalid or overlapping course schedule")
	ErrImmutableCode   = errors.New("master data code is immutable")
	ErrDuplicateCode   = errors.New("master data code already exists")
	ErrDuplicateName   = errors.New("master data name already exists")
	ErrNotFound        = errors.New("master data record not found")
)

type Record struct {
	ID                  string           `json:"id"`
	Kind                string           `json:"kind"`
	Code                string           `json:"code,omitempty"`
	Name                string           `json:"name"`
	Enabled             bool             `json:"enabled"`
	FacultyID           string           `json:"facultyId,omitempty"`
	MajorID             string           `json:"majorId,omitempty"`
	HeadTeacherID       string           `json:"headTeacherId,omitempty"`
	TeacherID           string           `json:"teacherId,omitempty"`
	ClassID             string           `json:"classId,omitempty"`
	DormitoryID         string           `json:"dormitoryId,omitempty"`
	ClassroomID         string           `json:"classroomId,omitempty"`
	TeachingBuildingID  string           `json:"teachingBuildingId,omitempty"`
	DormitoryAreaID     string           `json:"dormitoryAreaId,omitempty"`
	DormitoryBuildingID string           `json:"dormitoryBuildingId,omitempty"`
	Gender              string           `json:"gender,omitempty"`
	Phone               string           `json:"phone,omitempty"`
	Grade               string           `json:"grade,omitempty"`
	GradeID             string           `json:"gradeId,omitempty"`
	ClassIDs            []string         `json:"classIds,omitempty"`
	Schedules           []CourseSchedule `json:"schedules,omitempty"`
	EnrolledSize        int              `json:"enrolledSize,omitempty"`
	CampusArea          string           `json:"campusArea,omitempty"`
	BuildingNo          string           `json:"buildingNo,omitempty"`
	RoomNo              string           `json:"roomNo,omitempty"`
	ScheduleText        string           `json:"scheduleText,omitempty"`
	EnrollmentStatus    string           `json:"enrollmentStatus,omitempty"`
	CreatedAt           time.Time        `json:"createdAt,omitempty"`
	UpdatedAt           time.Time        `json:"updatedAt,omitempty"`
}

// CourseSchedule is one structured weekly meeting for a course. Weekday uses
// ISO weekday numbering (Monday=1); every section represents 45 minutes.
type CourseSchedule struct {
	ID           string `json:"id"`
	SemesterID   string `json:"semesterId"`
	ClassroomID  string `json:"classroomId"`
	Weekday      int    `json:"weekday"`
	StartSection int    `json:"startSection"`
	EndSection   int    `json:"endSection"`
}

type Query struct {
	Kind                string
	Keyword             string
	Code                string
	Name                string
	Enabled             *bool
	FacultyID           string
	MajorID             string
	HeadTeacherID       string
	TeacherID           string
	ClassID             string
	DormitoryID         string
	ClassroomID         string
	TeachingBuildingID  string
	Grade               string
	GradeID             string
	Gender              string
	EnrollmentStatus    string
	CampusArea          string
	BuildingNo          string
	DormitoryAreaID     string
	DormitoryBuildingID string
	SemesterID          string
	Page                int
	PageSize            int
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
	AcademicSemesterExists(context.Context, string) (bool, error)
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
	query.Code = strings.TrimSpace(query.Code)
	query.Name = strings.TrimSpace(query.Name)
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
	normalize(&record)
	if !validKind(record.Kind) {
		return Record{}, ErrInvalidKind
	}
	if record.Kind == KindCourse && !validCourseSchedules(record.Schedules) {
		return Record{}, ErrInvalidSchedule
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
	normalize(&record)
	if requiresBusinessIdentifier(record.Kind) && record.Code != existing.Code {
		return Record{}, ErrImmutableCode
	}
	if record.Kind == KindCourse && !validCourseSchedules(record.Schedules) {
		return Record{}, ErrInvalidSchedule
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
		{KindTeachingBuilding, record.TeachingBuildingID, record.Kind == KindClassroom},
		{KindGrade, record.GradeID, record.Kind == KindClass || record.Kind == KindStudent},
		{KindDormitoryArea, record.DormitoryAreaID, record.Kind == KindDormitoryBuilding},
		{KindDormitoryBuilding, record.DormitoryBuildingID, record.Kind == KindDormitory},
	}
	for _, classID := range record.ClassIDs {
		relations = append(relations, struct {
			kind, id string
			required bool
		}{KindClass, classID, true})
	}
	for _, schedule := range record.Schedules {
		relations = append(relations, struct {
			kind, id string
			required bool
		}{KindClassroom, schedule.ClassroomID, true})
		exists, err := s.repository.AcademicSemesterExists(ctx, schedule.SemesterID)
		if err != nil {
			return err
		}
		if !exists {
			return ErrInvalidRelation
		}
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
	if record.Kind == KindStudent && record.ClassID != "" {
		class, err := s.repository.FindMasterData(ctx, KindClass, record.ClassID)
		if err != nil {
			return err
		}
		if class.GradeID != record.GradeID {
			return ErrInvalidRelation
		}
	}
	return nil
}

func validKind(kind string) bool {
	switch kind {
	case KindFaculty, KindGrade, KindDormitoryArea, KindDormitoryBuilding, KindTeachingBuilding, KindMajor, KindStaff, KindClass, KindDormitory, KindClassroom, KindCourse, KindStudent:
		return true
	default:
		return false
	}
}

func validRecord(record Record) bool {
	if record.Name == "" || requiresBusinessIdentifier(record.Kind) && record.Code == "" {
		return false
	}
	switch record.Kind {
	case KindGrade:
		return validGradeName(record.Name)
	case KindDormitoryBuilding:
		return record.DormitoryAreaID != ""
	case KindClass:
		return record.GradeID != "" && record.EnrolledSize >= 0
	case KindDormitory:
		return record.DormitoryBuildingID != "" && record.RoomNo != ""
	case KindClassroom:
		return record.TeachingBuildingID != "" && record.RoomNo != ""
	case KindStudent:
		return record.GradeID != ""
	case KindCourse:
		return len(record.ClassIDs) > 0 && len(record.Schedules) > 0 && validCourseSchedules(record.Schedules)
	default:
		return true
	}
}

func normalize(record *Record) {
	record.Name = strings.TrimSpace(record.Name)
	record.Code = strings.ToUpper(strings.TrimSpace(record.Code))
	if record.Kind == KindGrade {
		year := strings.TrimSpace(strings.TrimSuffix(record.Name, "级"))
		record.Name = year + "级"
	}
	if !requiresBusinessIdentifier(record.Kind) {
		record.Code = ""
	}
	record.Gender = strings.ToUpper(strings.TrimSpace(record.Gender))
	record.Phone = strings.TrimSpace(record.Phone)
	record.GradeID = strings.TrimSpace(record.GradeID)
	record.CampusArea = strings.TrimSpace(record.CampusArea)
	record.BuildingNo = strings.TrimSpace(record.BuildingNo)
	record.RoomNo = strings.TrimSpace(record.RoomNo)
	record.ScheduleText = strings.TrimSpace(record.ScheduleText)
	record.EnrollmentStatus = strings.ToUpper(strings.TrimSpace(record.EnrollmentStatus))
	classIDs := make([]string, 0, len(record.ClassIDs))
	seenClassIDs := make(map[string]struct{}, len(record.ClassIDs))
	for _, classID := range record.ClassIDs {
		classID = strings.TrimSpace(classID)
		if classID == "" {
			continue
		}
		if _, exists := seenClassIDs[classID]; exists {
			continue
		}
		seenClassIDs[classID] = struct{}{}
		classIDs = append(classIDs, classID)
	}
	record.ClassIDs = classIDs
	for index := range record.Schedules {
		// Schedule IDs are technical identifiers and are never accepted from clients.
		record.Schedules[index].ID = uuid.NewString()
		record.Schedules[index].SemesterID = strings.TrimSpace(record.Schedules[index].SemesterID)
		record.Schedules[index].ClassroomID = strings.TrimSpace(record.Schedules[index].ClassroomID)
	}
	if record.Kind == KindDormitory {
		record.Name = record.RoomNo
	}
	if record.Kind == KindClassroom {
		record.Name = record.RoomNo
	}
	if record.Gender == "" {
		record.Gender = "UNKNOWN"
	}
	if record.Kind == KindStudent && record.EnrollmentStatus == "" {
		record.EnrollmentStatus = "ENROLLED"
	}
}

func validGradeName(name string) bool {
	year := strings.TrimSuffix(name, "级")
	if len(year) != 4 {
		return false
	}
	for _, digit := range year {
		if digit < '0' || digit > '9' {
			return false
		}
	}
	return true
}

func requiresBusinessIdentifier(kind string) bool {
	return kind == KindStaff || kind == KindStudent
}

func validCourseSchedules(schedules []CourseSchedule) bool {
	for index, schedule := range schedules {
		if schedule.SemesterID == "" || schedule.ClassroomID == "" || schedule.Weekday < 1 || schedule.Weekday > 7 ||
			schedule.StartSection < 1 || schedule.StartSection > 10 || schedule.EndSection < schedule.StartSection || schedule.EndSection > 10 {
			return false
		}
		for otherIndex := index + 1; otherIndex < len(schedules); otherIndex++ {
			other := schedules[otherIndex]
			if schedule.SemesterID == other.SemesterID && schedule.Weekday == other.Weekday &&
				schedule.StartSection <= other.EndSection && other.StartSection <= schedule.EndSection {
				return false
			}
		}
	}
	return true
}
