package masterdata

import (
	"context"
	"errors"
	"testing"

	"four-check-management-system/server/internal/audit"
)

type memoryRepository struct {
	records   map[string]Record
	changes   []Change
	mutations []audit.Mutation
}

func newMemoryRepository() *memoryRepository { return &memoryRepository{records: map[string]Record{}} }
func key(kind, id string) string             { return kind + ":" + id }
func (m *memoryRepository) ListMasterData(_ context.Context, query Query) (Page, error) {
	result := Page{Page: query.Page, PageSize: query.PageSize}
	for _, record := range m.records {
		if record.Kind == query.Kind {
			result.Items = append(result.Items, record)
		}
	}
	result.Total = len(result.Items)
	return result, nil
}
func (m *memoryRepository) FindMasterData(_ context.Context, kind, id string) (Record, error) {
	value, ok := m.records[key(kind, id)]
	if !ok {
		return Record{}, ErrNotFound
	}
	return value, nil
}
func (m *memoryRepository) MasterDataExists(_ context.Context, kind, id string) (bool, error) {
	_, ok := m.records[key(kind, id)]
	return ok, nil
}
func (m *memoryRepository) CreateMasterData(_ context.Context, actorID string, record Record, mutation audit.Mutation) (Record, error) {
	m.records[key(record.Kind, record.ID)] = record
	m.changes = append(m.changes, Change{EntityType: record.Kind, EntityID: record.ID, Action: "CREATE", ActorID: actorID})
	m.mutations = append(m.mutations, mutation)
	return record, nil
}
func (m *memoryRepository) UpdateMasterData(_ context.Context, actorID string, record Record, mutation audit.Mutation) (Record, error) {
	m.records[key(record.Kind, record.ID)] = record
	m.changes = append(m.changes, Change{EntityType: record.Kind, EntityID: record.ID, Action: "UPDATE", ActorID: actorID})
	m.mutations = append(m.mutations, mutation)
	return record, nil
}
func (m *memoryRepository) ListMasterDataChanges(context.Context, string, string) ([]Change, error) {
	return m.changes, nil
}
func (m *memoryRepository) HygieneAreaInspectionScope(context.Context, string, string) (string, error) {
	return "30000000-0000-0000-0000-000000000003", nil
}

func TestCreateValidatesEntitySpecificRequiredFields(t *testing.T) {
	svc := NewService(newMemoryRepository())
	if _, err := svc.Create(context.Background(), "admin", Record{Kind: KindClass, Name: "计科 2501"}); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("class without grade error = %v", err)
	}
	if _, err := svc.Create(context.Background(), "admin", Record{Kind: "UNKNOWN", Code: "X", Name: "未知"}); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("unknown kind error = %v", err)
	}
}

func TestCreateUsesBackendGeneratedIDWithoutArtificialCode(t *testing.T) {
	repo := newMemoryRepository()
	created, err := NewService(repo).Create(context.Background(), "admin-1", Record{Kind: KindFaculty, Name: " 计算机学院 "})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Code != "" || created.Name != "计算机学院" || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
	if len(repo.changes) != 1 || repo.changes[0].ActorID != "admin-1" {
		t.Fatalf("changes = %+v", repo.changes)
	}
}

func TestGradeIsSupportedAsMasterData(t *testing.T) {
	created, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{Kind: KindGrade, Name: " 2026 "})
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != KindGrade || created.Code != "" || created.Name != "2026级" || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
}

func TestGradeRejectsValuesThatAreNotFourDigitYears(t *testing.T) {
	for _, name := range []string{"26", "202A", "20260"} {
		_, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{Kind: KindGrade, Name: name})
		if !errors.Is(err, ErrInvalidRecord) {
			t.Fatalf("grade %q error = %v, want %v", name, err, ErrInvalidRecord)
		}
	}
}

func TestDormitoryBuildingRequiresExistingArea(t *testing.T) {
	_, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{
		Kind: KindDormitoryBuilding, Name: "一栋", DormitoryAreaID: "missing",
	})
	if !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("error = %v", err)
	}
}

func TestDormitoryRequiresExistingBuilding(t *testing.T) {
	_, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{
		Kind: KindDormitory, DormitoryBuildingID: "missing", RoomNo: "101",
	})
	if !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("error = %v", err)
	}
}

func TestTeachingBuildingIsSupportedAsMasterData(t *testing.T) {
	created, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{
		Kind: "teaching-buildings", Name: "博学楼",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Kind != "teaching-buildings" || created.Name != "博学楼" || created.ID == "" {
		t.Fatalf("created = %+v", created)
	}
}

func TestCreateProvidesMasterDataAuditMutation(t *testing.T) {
	repository := newMemoryRepository()
	created, err := NewService(repository).Create(context.Background(), "admin-1", Record{
		Kind: KindFaculty, Code: "CS", Name: "计算机学院",
	}, audit.Metadata{RequestID: "master-data-request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.mutations) != 1 {
		t.Fatalf("audit mutations = %d", len(repository.mutations))
	}
	mutation := repository.mutations[0]
	if mutation.Metadata.ActorID != "admin-1" || mutation.Entry.Action != audit.ActionMasterDataChanged || mutation.Entry.ResourceID != created.ID {
		t.Fatalf("mutation = %+v", mutation)
	}
}

func TestMajorRequiresExistingFaculty(t *testing.T) {
	repo := newMemoryRepository()
	_, err := NewService(repo).Create(context.Background(), "admin", Record{Kind: KindMajor, Code: "CS", Name: "计算机科学", FacultyID: "missing"})
	if !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateRejectsImmutableCode(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	created, _ := svc.Create(context.Background(), "admin", Record{Kind: KindStaff, Code: "T001", Name: "张老师"})
	created.Code = "MATH"
	if _, err := svc.Update(context.Background(), "admin", created); !errors.Is(err, ErrImmutableCode) {
		t.Fatalf("error = %v", err)
	}
}

func TestCourseRequiresOnlyANameAndIgnoresLegacyRelationships(t *testing.T) {
	created, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{
		Kind: KindCourse, Name: " 数据结构 ", FacultyID: "legacy-faculty", ClassID: "legacy-class", ClassroomID: "legacy-room",
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "数据结构" || created.FacultyID != "" || created.ClassID != "" || created.ClassroomID != "" {
		t.Fatalf("created = %+v", created)
	}
}

func TestCourseRejectsBlankOrOverlongNames(t *testing.T) {
	for _, name := range []string{"", "   ", string(make([]rune, 129))} {
		if _, err := NewService(newMemoryRepository()).Create(context.Background(), "admin", Record{Kind: KindCourse, Name: name}); !errors.Is(err, ErrInvalidRecord) {
			t.Errorf("course name length %d error = %v, want %v", len([]rune(name)), err, ErrInvalidRecord)
		}
	}
}

func TestStudentClassMustBelongToSelectedGrade(t *testing.T) {
	repo := newMemoryRepository()
	repo.records[key(KindGrade, "grade-2025")] = Record{ID: "grade-2025", Kind: KindGrade, Name: "2025级", Enabled: true}
	repo.records[key(KindGrade, "grade-2026")] = Record{ID: "grade-2026", Kind: KindGrade, Name: "2026级", Enabled: true}
	repo.records[key(KindClass, "class-2025")] = Record{ID: "class-2025", Kind: KindClass, Name: "软件工程一班", GradeID: "grade-2025", Enabled: true}

	_, err := NewService(repo).Create(context.Background(), "admin", Record{
		Kind: KindStudent, Code: "20260001", Name: "测试学生", GradeID: "grade-2026", ClassID: "class-2025",
	})
	if !errors.Is(err, ErrInvalidRelation) {
		t.Fatalf("error = %v, want %v", err, ErrInvalidRelation)
	}
}

func TestUpdateCanDisableRecordAndPreservesHistory(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	created, _ := svc.Create(context.Background(), "admin", Record{Kind: KindFaculty, Name: "计算机学院"})
	created.Enabled = false
	if _, err := svc.Update(context.Background(), "admin", created); err != nil {
		t.Fatal(err)
	}
	if len(repo.changes) != 2 || repo.changes[1].Action != "UPDATE" {
		t.Fatalf("changes = %+v", repo.changes)
	}
}

func TestListNormalizesPagination(t *testing.T) {
	page, err := NewService(newMemoryRepository()).List(context.Background(), Query{Kind: KindStudent, Page: 0, PageSize: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if page.Page != 1 || page.PageSize != 20 {
		t.Fatalf("page = %+v", page)
	}
}

func TestListReturnsAnEmptyItemsArray(t *testing.T) {
	page, err := NewService(newMemoryRepository()).List(context.Background(), Query{Kind: KindClass})
	if err != nil {
		t.Fatal(err)
	}
	if page.Items == nil {
		t.Fatal("empty master-data pages must serialize items as [] instead of null")
	}
}
