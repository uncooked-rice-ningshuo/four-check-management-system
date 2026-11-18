package disciplineconfig

import (
	"context"
	"errors"
)

var (
	ErrNotFound  = errors.New("discipline rule version not found")
	ErrConflict  = errors.New("discipline rule version conflict")
	ErrImmutable = errors.New("discipline rule version is immutable")
)

type Repository interface {
	ListInspectionTypes(context.Context) ([]InspectionType, error)
	ListDisciplineTierDefinitions(context.Context) ([]TierDefinition, error)
	Current(context.Context) (Version, bool, error)
	ListVersions(context.Context) ([]Version, error)
	GetVersion(context.Context, string) (Version, error)
	CreateDraft(context.Context, string) (Version, error)
	SaveDraft(context.Context, Version, int, string) (Version, error)
	Publish(context.Context, string, int, string) (Version, error)
}

type Service struct{ repository Repository }

func NewService(r Repository) *Service { return &Service{repository: r} }
func (s *Service) Current(ctx context.Context) (Version, bool, error) {
	return s.repository.Current(ctx)
}
func (s *Service) ListVersions(ctx context.Context) ([]Version, error) {
	return s.repository.ListVersions(ctx)
}
func (s *Service) GetVersion(ctx context.Context, id string) (Version, error) {
	return s.repository.GetVersion(ctx, id)
}
func (s *Service) CreateDraft(ctx context.Context, actor string) (Version, error) {
	return s.repository.CreateDraft(ctx, actor)
}
func (s *Service) SaveDraft(ctx context.Context, v Version, expected int, actor string) (Version, error) {
	types, err := s.repository.ListInspectionTypes(ctx)
	if err != nil {
		return Version{}, err
	}
	if v.Status != StatusDraft || v.RowVersion != expected {
		return Version{}, ErrInvalidConfig
	}
	v.Groups = NormalizeInspectionTypeNames(v.Groups, types)
	definitions, err := s.repository.ListDisciplineTierDefinitions(ctx)
	if err != nil {
		return Version{}, err
	}
	v.Groups = NormalizeWithTierDefinitions(v.Groups, types, definitions)
	if err := ValidateWithTierDefinitions(v.Groups, types, definitions, false); err != nil {
		return Version{}, err
	}
	return s.repository.SaveDraft(ctx, v, expected, actor)
}
func (s *Service) Validate(ctx context.Context, id string) error {
	v, err := s.repository.GetVersion(ctx, id)
	if err != nil {
		return err
	}
	if v.Status != StatusDraft {
		return ErrImmutable
	}
	types, err := s.repository.ListInspectionTypes(ctx)
	if err != nil {
		return err
	}
	definitions, err := s.repository.ListDisciplineTierDefinitions(ctx)
	if err != nil {
		return err
	}
	v.Groups = NormalizeInspectionTypeNames(v.Groups, types)
	v.Groups = NormalizeWithTierDefinitions(v.Groups, types, definitions)
	return ValidateWithTierDefinitions(v.Groups, types, definitions, true)
}
func (s *Service) Publish(ctx context.Context, id string, expected int, actor string) (Version, error) {
	v, err := s.repository.GetVersion(ctx, id)
	if err != nil {
		return Version{}, err
	}
	if v.Status != StatusDraft {
		return Version{}, ErrImmutable
	}
	types, err := s.repository.ListInspectionTypes(ctx)
	if err != nil {
		return Version{}, err
	}
	definitions, err := s.repository.ListDisciplineTierDefinitions(ctx)
	if err != nil {
		return Version{}, err
	}
	originalGroups := v.Groups
	v.Groups = NormalizeInspectionTypeNames(v.Groups, types)
	v.Groups = NormalizeWithTierDefinitions(v.Groups, types, definitions)
	if err := ValidateWithTierDefinitions(v.Groups, types, definitions, true); err != nil {
		return Version{}, err
	}
	if !groupsEqual(originalGroups, v.Groups) {
		v, err = s.repository.SaveDraft(ctx, v, expected, actor)
		if err != nil {
			return Version{}, err
		}
		expected = v.RowVersion
	}
	return s.repository.Publish(ctx, id, expected, actor)
}

func groupsEqual(a, b []Group) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Key != b[i].Key || a[i].Name != b[i].Name || a[i].Unit != b[i].Unit || len(a[i].Terms) != len(b[i].Terms) || len(a[i].Tiers) != len(b[i].Tiers) {
			return false
		}
		for j := range a[i].Terms {
			if a[i].Terms[j] != b[i].Terms[j] {
				return false
			}
		}
		for j := range a[i].Tiers {
			if a[i].Tiers[j].Code != b[i].Tiers[j].Code || a[i].Tiers[j].Name != b[i].Tiers[j].Name {
				return false
			}
		}
	}
	return true
}
