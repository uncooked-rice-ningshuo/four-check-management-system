package inspection

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type contextOptionStub struct {
	query ContextOptionQuery
	err   error
}

func (r *contextOptionStub) ListContextOptions(_ context.Context, q ContextOptionQuery, _ MutationAccess) (ContextOptionPage, error) {
	r.query = q
	return ContextOptionPage{EntryOptionPage: EntryOptionPage{Items: []EntryOption{}, Page: q.Page, PageSize: q.PageSize}}, r.err
}
func TestContextOptionsValidation(t *testing.T) {
	r := &contextOptionStub{}
	s := NewContextOptionService(r, nil)
	actor := ManualActor{ID: detailID, IsSuperAdmin: true}
	base := ContextOptionQuery{TypeCode: InspectionTypeUnattended, Purpose: "view", Kind: "semesters"}
	for _, change := range []func(*ContextOptionQuery){
		func(q *ContextOptionQuery) { q.TypeCode = "unknown" },
		func(q *ContextOptionQuery) { q.Purpose = "manage" },
		func(q *ContextOptionQuery) { q.Kind = "students" },
		func(q *ContextOptionQuery) { q.Grade = strings.Repeat("年", 65) },
		func(q *ContextOptionQuery) { q.Kind = "weeks" },
		func(q *ContextOptionQuery) { q.Purpose = "entry"; q.Kind = "departments" },
		func(q *ContextOptionQuery) { q.SemesterID = "bad" },
		func(q *ContextOptionQuery) { q.PageSize = 201 },
		func(q *ContextOptionQuery) { q.Page = -1 },
	} {
		q := base
		change(&q)
		if _, err := s.ContextOptions(context.Background(), actor, q); !errors.Is(err, ErrInspectionRequest) {
			t.Fatal("invalid context query accepted", err)
		}
	}
	page, err := s.ContextOptions(context.Background(), actor, base)
	if err != nil || page.Page != 1 || page.PageSize != 20 {
		t.Fatal("pagination defaults", err)
	}
	base.TypeCode = InspectionTypeMorning
	if _, err := s.ContextOptions(context.Background(), actor, base); err != nil {
		t.Fatal("morning context", err)
	}
	base.TypeCode = InspectionTypeLateReturn
	if _, err := s.ContextOptions(context.Background(), actor, base); err != nil {
		t.Fatal("late return context", err)
	}
}

func TestContextOptionsFailure(t *testing.T) {
	r := &contextOptionStub{err: errors.New("unavailable")}
	s := NewContextOptionService(r, nil)
	q := ContextOptionQuery{TypeCode: InspectionTypeUnattended, Purpose: "view", Kind: "semesters"}
	if _, err := s.ContextOptions(context.Background(), ManualActor{ID: detailID, IsSuperAdmin: true}, q); !errors.Is(err, r.err) {
		t.Fatal("repository error lost", err)
	}
	if _, err := s.ContextOptions(context.Background(), ManualActor{}, q); !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("invalid actor accepted", err)
	}
}
