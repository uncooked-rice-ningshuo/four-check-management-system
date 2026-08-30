package dictionary

import (
	"context"
	"errors"
	"strings"

	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
)

var (
	ErrInvalidDictionary   = errors.New("invalid dictionary")
	ErrDuplicateValue      = errors.New("duplicate dictionary value")
	ErrSystemTypeProtected = errors.New("system dictionary type is protected")
	ErrNotFound            = errors.New("dictionary not found")
)

type Type struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description"`
	System      bool   `json:"system"`
	Enabled     bool   `json:"enabled"`
}

type Item struct {
	ID          string `json:"id"`
	TypeID      string `json:"typeId"`
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description"`
	SortOrder   int    `json:"sortOrder"`
	Enabled     bool   `json:"enabled"`
}

type Catalog struct {
	Types []Type `json:"types"`
	Items []Item `json:"items"`
}

type Repository interface {
	ListDictionaryTypes(context.Context) ([]Type, error)
	ListDictionaryItems(context.Context, string) ([]Item, error)
	FindDictionaryType(context.Context, string) (Type, error)
	CreateDictionaryType(context.Context, Type, audit.Mutation) (Type, error)
	UpdateDictionaryType(context.Context, Type, audit.Mutation) (Type, error)
	CreateDictionaryItem(context.Context, Item, audit.Mutation) (Item, error)
	UpdateDictionaryItem(context.Context, Item, audit.Mutation) (Item, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

func (s *Service) List(ctx context.Context, typeID string) (Catalog, error) {
	types, err := s.repository.ListDictionaryTypes(ctx)
	if err != nil {
		return Catalog{}, err
	}
	if types == nil {
		types = make([]Type, 0)
	}
	items := make([]Item, 0)
	if typeID != "" {
		items, err = s.repository.ListDictionaryItems(ctx, typeID)
		if items == nil {
			items = make([]Item, 0)
		}
	}
	return Catalog{Types: types, Items: items}, err
}

func (s *Service) CreateType(ctx context.Context, value Type, metadata ...audit.Metadata) (Type, error) {
	normalizeType(&value)
	if value.Code == "" || value.Name == "" {
		return Type{}, ErrInvalidDictionary
	}
	value.ID = uuid.NewString()
	value.System = false
	value.Enabled = true
	return s.repository.CreateDictionaryType(ctx, value, dictionaryMutation(metadata, "dictionaryType", value.ID, nil, value))
}

func (s *Service) UpdateType(ctx context.Context, value Type, metadata ...audit.Metadata) (Type, error) {
	if value.ID == "" {
		return Type{}, ErrInvalidDictionary
	}
	existing, err := s.repository.FindDictionaryType(ctx, value.ID)
	if err != nil {
		return Type{}, err
	}
	normalizeType(&value)
	if existing.System && (existing.Code != value.Code || existing.Name != value.Name || !value.Enabled) {
		return Type{}, ErrSystemTypeProtected
	}
	if value.Code == "" || value.Name == "" {
		return Type{}, ErrInvalidDictionary
	}
	value.System = existing.System
	return s.repository.UpdateDictionaryType(ctx, value, dictionaryMutation(metadata, "dictionaryType", value.ID, existing, value))
}

func (s *Service) CreateItem(ctx context.Context, value Item, metadata ...audit.Metadata) (Item, error) {
	if _, err := s.repository.FindDictionaryType(ctx, value.TypeID); err != nil {
		return Item{}, err
	}
	normalizeItem(&value)
	if value.TypeID == "" || value.Value == "" || value.Label == "" {
		return Item{}, ErrInvalidDictionary
	}
	value.ID = uuid.NewString()
	value.Enabled = true
	return s.repository.CreateDictionaryItem(ctx, value, dictionaryMutation(metadata, "dictionaryItem", value.ID, nil, value))
}

func (s *Service) UpdateItem(ctx context.Context, value Item, metadata ...audit.Metadata) (Item, error) {
	normalizeItem(&value)
	if value.ID == "" || value.TypeID == "" || value.Value == "" || value.Label == "" {
		return Item{}, ErrInvalidDictionary
	}
	if _, err := s.repository.FindDictionaryType(ctx, value.TypeID); err != nil {
		return Item{}, err
	}
	items, err := s.repository.ListDictionaryItems(ctx, value.TypeID)
	if err != nil {
		return Item{}, err
	}
	var before any
	for _, existing := range items {
		if existing.ID == value.ID {
			before = existing
			break
		}
	}
	return s.repository.UpdateDictionaryItem(ctx, value, dictionaryMutation(metadata, "dictionaryItem", value.ID, before, value))
}

func dictionaryMutation(metadata []audit.Metadata, resourceType, resourceID string, before, after any) audit.Mutation {
	request := audit.Metadata{}
	if len(metadata) > 0 {
		request = metadata[0]
	}
	return audit.Mutation{Metadata: request, Entry: audit.Entry{
		Action: audit.ActionDictionaryChanged, ResourceType: resourceType, ResourceID: resourceID, Before: before, After: after,
	}}
}

func normalizeType(value *Type) {
	value.Code = strings.ToUpper(strings.TrimSpace(value.Code))
	value.Name = strings.TrimSpace(value.Name)
	value.Description = strings.TrimSpace(value.Description)
}

func normalizeItem(value *Item) {
	value.Value = strings.ToUpper(strings.TrimSpace(value.Value))
	value.Label = strings.TrimSpace(value.Label)
	value.Description = strings.TrimSpace(value.Description)
}
