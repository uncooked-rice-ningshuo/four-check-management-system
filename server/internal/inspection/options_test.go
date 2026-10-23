package inspection

import (
	"context"
	"errors"
	"testing"
)

func TestEntryOptionsRejectInvalidScopeBeforeRepository(t *testing.T) {
	s := NewEntryOptionService(nil, nil)
	base := EntryOptionQuery{SemesterID: detailID, WeekID: detailID, InspectionTypeID: detailID, DepartmentID: detailID, Kind: "classes"}
	for _, change := range []func(*EntryOptionQuery){
		func(q *EntryOptionQuery) { q.WeekID = "" },
		func(q *EntryOptionQuery) { q.Kind = "accounts" },
		func(q *EntryOptionQuery) { q.Page = -1 },
		func(q *EntryOptionQuery) { q.PageSize = 201 },
		func(q *EntryOptionQuery) { q.Kind = "students" },
		func(q *EntryOptionQuery) { q.Kind = "classrooms"; q.ClassID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "dormitories"; q.ClassID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "dormitories"; q.StudentID = detailID },
		func(q *EntryOptionQuery) { q.StudentID = "invalid" },
		func(q *EntryOptionQuery) { q.Kind = "dormitory-objects" },
		func(q *EntryOptionQuery) { q.Kind = "reason-codes"; q.StudentID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "classroom-objects" },
		func(q *EntryOptionQuery) { q.Kind = "classroom-areas"; q.ClassID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "responsible-dormitories"; q.ClassID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "public-areas"; q.AreaID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "reason-codes"; q.ClassID = detailID; q.AreaID = detailID },
		func(q *EntryOptionQuery) { q.Kind = "courses"; q.ClassID = detailID; q.Date = "2026-02-30" },
		func(q *EntryOptionQuery) {
			q.Kind = "periods"
			q.ClassID = detailID
			q.CourseID = detailID
			q.ClassroomID = detailID
		},
		func(q *EntryOptionQuery) { q.Date = "2026-06-17" },
	} {
		q := base
		change(&q)
		if _, err := s.EntryOptions(context.Background(), ManualActor{ID: detailID, IsSuperAdmin: true}, q); !errors.Is(err, ErrInspectionRequest) {
			t.Fatalf("invalid query accepted: %v", err)
		}
	}
}

type optionRepositoryStub struct {
	query  EntryOptionQuery
	access MutationAccess
	err    error
}

func (r *optionRepositoryStub) ListEntryOptions(_ context.Context, q EntryOptionQuery, a MutationAccess) (EntryOptionPage, error) {
	r.query, r.access = q, a
	return EntryOptionPage{Items: []EntryOption{}, Page: q.Page, PageSize: q.PageSize}, r.err
}
func TestEntryOptionsNormalizeAndPropagateFailure(t *testing.T) {
	r := &optionRepositoryStub{}
	s := NewEntryOptionService(r, nil)
	q := EntryOptionQuery{SemesterID: detailID, WeekID: detailID, InspectionTypeID: detailID, DepartmentID: detailID, Kind: "classes"}
	page, err := s.EntryOptions(context.Background(), ManualActor{ID: detailID, IsSuperAdmin: true}, q)
	if err != nil || page.Page != 1 || page.PageSize != 20 || !r.access.Unrestricted {
		t.Fatal("defaults or access", err)
	}
	r.err = errors.New("repository failure")
	if _, err = s.EntryOptions(context.Background(), ManualActor{ID: detailID, IsSuperAdmin: true}, q); !errors.Is(err, r.err) {
		t.Fatal("failure hidden", err)
	}
	if _, err = s.EntryOptions(context.Background(), ManualActor{}, q); !errors.Is(err, ErrScopeMismatch) {
		t.Fatal("invalid actor", err)
	}
}
