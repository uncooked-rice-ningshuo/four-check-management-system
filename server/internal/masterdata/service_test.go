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

func TestCreateValidatesEntitySpecificRequiredFields(t *testing.T) {
	svc := NewService(newMemoryRepository())
	if _, err := svc.Create(context.Background(), "admin", Record{Kind: KindClass, Code: "CS2025-1", Name: "计科 2501"}); !errors.Is(err, ErrInvalidRecord) {
		t.Fatalf("class without grade error = %v", err)
	}
	if _, err := svc.Create(context.Background(), "admin", Record{Kind: "UNKNOWN", Code: "X", Name: "未知"}); !errors.Is(err, ErrInvalidKind) {
		t.Fatalf("unknown kind error = %v", err)
	}
}

func TestCreateNormalizesCodeAndRecordsActor(t *testing.T) {
	repo := newMemoryRepository()
	created, err := NewService(repo).Create(context.Background(), "admin-1", Record{Kind: KindFaculty, Code: " cs ", Name: " 计算机学院 "})
	if err != nil {
		t.Fatal(err)
	}
	if created.Code != "CS" || created.Name != "计算机学院" || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
	if len(repo.changes) != 1 || repo.changes[0].ActorID != "admin-1" {
		t.Fatalf("changes = %+v", repo.changes)
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
	created, _ := svc.Create(context.Background(), "admin", Record{Kind: KindFaculty, Code: "CS", Name: "计算机学院"})
	created.Code = "MATH"
	if _, err := svc.Update(context.Background(), "admin", created); !errors.Is(err, ErrImmutableCode) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateCanDisableRecordAndPreservesHistory(t *testing.T) {
	repo := newMemoryRepository()
	svc := NewService(repo)
	created, _ := svc.Create(context.Background(), "admin", Record{Kind: KindClassroom, Code: "A101", Name: "A101", Building: "A", RoomNo: "101", Floor: 1})
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
