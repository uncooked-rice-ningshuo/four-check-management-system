package dictionary

import (
	"context"
	"errors"
	"testing"

	"four-check-management-system/server/internal/audit"
)

type memoryRepository struct {
	types     []Type
	items     []Item
	mutations []audit.Mutation
}

func (m *memoryRepository) ListDictionaryTypes(context.Context) ([]Type, error) { return m.types, nil }
func (m *memoryRepository) ListDictionaryItems(_ context.Context, typeID string) ([]Item, error) {
	var result []Item
	for _, item := range m.items {
		if item.TypeID == typeID {
			result = append(result, item)
		}
	}
	return result, nil
}
func (m *memoryRepository) FindDictionaryType(_ context.Context, id string) (Type, error) {
	for _, value := range m.types {
		if value.ID == id {
			return value, nil
		}
	}
	return Type{}, ErrNotFound
}
func (m *memoryRepository) CreateDictionaryType(_ context.Context, value Type, mutation audit.Mutation) (Type, error) {
	m.types = append(m.types, value)
	m.mutations = append(m.mutations, mutation)
	return value, nil
}
func (m *memoryRepository) UpdateDictionaryType(_ context.Context, value Type, mutation audit.Mutation) (Type, error) {
	m.mutations = append(m.mutations, mutation)
	return value, nil
}
func (m *memoryRepository) CreateDictionaryItem(_ context.Context, value Item, mutation audit.Mutation) (Item, error) {
	for _, existing := range m.items {
		if existing.TypeID == value.TypeID && existing.Value == value.Value {
			return Item{}, ErrDuplicateValue
		}
	}
	m.items = append(m.items, value)
	m.mutations = append(m.mutations, mutation)
	return value, nil
}
func (m *memoryRepository) UpdateDictionaryItem(_ context.Context, value Item, mutation audit.Mutation) (Item, error) {
	m.mutations = append(m.mutations, mutation)
	return value, nil
}

func TestCreateTypeNormalizesCode(t *testing.T) {
	created, err := NewService(&memoryRepository{}).CreateType(context.Background(), Type{Code: " gender ", Name: " 性别 "})
	if err != nil {
		t.Fatal(err)
	}
	if created.Code != "GENDER" || created.Name != "性别" || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
}

func TestCreateTypeProvidesDictionaryAuditMutation(t *testing.T) {
	repository := &memoryRepository{}
	created, err := NewService(repository).CreateType(context.Background(), Type{Code: "CUSTOM", Name: "自定义"}, audit.Metadata{ActorID: "admin-1", RequestID: "dictionary-request"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repository.mutations) != 1 {
		t.Fatalf("audit mutations = %d", len(repository.mutations))
	}
	mutation := repository.mutations[0]
	if mutation.Metadata.ActorID != "admin-1" || mutation.Entry.Action != audit.ActionDictionaryChanged || mutation.Entry.ResourceID != created.ID {
		t.Fatalf("mutation = %+v", mutation)
	}
}

func TestCreateItemNormalizesValueAndPreservesOrder(t *testing.T) {
	repo := &memoryRepository{types: []Type{{ID: "type-1", Code: "GENDER", Name: "性别", Enabled: true}}}
	created, err := NewService(repo).CreateItem(context.Background(), Item{TypeID: "type-1", Value: " male ", Label: " 男 ", SortOrder: 20})
	if err != nil {
		t.Fatal(err)
	}
	if created.Value != "MALE" || created.Label != "男" || created.SortOrder != 20 || !created.Enabled {
		t.Fatalf("created = %+v", created)
	}
}

func TestCreateItemRejectsDuplicateValue(t *testing.T) {
	repo := &memoryRepository{types: []Type{{ID: "type-1", Enabled: true}}, items: []Item{{ID: "one", TypeID: "type-1", Value: "MALE"}}}
	_, err := NewService(repo).CreateItem(context.Background(), Item{TypeID: "type-1", Value: "male", Label: "男"})
	if !errors.Is(err, ErrDuplicateValue) {
		t.Fatalf("error = %v", err)
	}
}

func TestSystemTypeCannotBeDisabledOrRenamed(t *testing.T) {
	repo := &memoryRepository{types: []Type{{ID: "type-1", Code: "GENDER", Name: "性别", System: true, Enabled: true}}}
	_, err := NewService(repo).UpdateType(context.Background(), Type{ID: "type-1", Code: "OTHER", Name: "其他", System: true, Enabled: false})
	if !errors.Is(err, ErrSystemTypeProtected) {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateItemRejectsMissingType(t *testing.T) {
	_, err := NewService(&memoryRepository{}).CreateItem(context.Background(), Item{TypeID: "missing", Value: "A", Label: "A"})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}
