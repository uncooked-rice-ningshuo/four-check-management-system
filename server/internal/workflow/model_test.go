package workflow

import (
	"context"
	"errors"
	"testing"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/permission"
)

type submitRepository struct {
	called   bool
	last     SubmitCommand
	decision DecisionCommand
	resubmit ResubmitCommand
}

func (r *submitRepository) SubmitDepartmentBatch(_ context.Context, command SubmitCommand) (Submission, error) {
	r.called, r.last = true, command
	return Submission{ID: "batch"}, nil
}
func (r *submitRepository) DecideDepartmentBatch(_ context.Context, command DecisionCommand) (Submission, error) {
	r.decision = command
	return Submission{Status: Returned}, nil
}
func (r *submitRepository) ResubmitDepartmentBatch(_ context.Context, command ResubmitCommand) (Submission, error) {
	r.resubmit = command
	return Submission{Status: PendingReview, RevisionNo: 2}, nil
}
func (r *submitRepository) ListDepartmentBatches(_ context.Context, _ ViewAccess, query ListQuery) (Page, error) {
	return Page{Items: []Submission{}, Page: query.Page, PageSize: query.PageSize}, nil
}
func (r *submitRepository) GetDepartmentBatch(_ context.Context, _ ViewAccess, id string) (Submission, error) {
	return Submission{ID: id}, nil
}

type submitPermissions struct {
	err error
}

func (p submitPermissions) Effective(context.Context, string) (permission.EffectiveResult, error) {
	return permission.EffectiveResult{Permissions: []permission.EffectivePermission{{Action: permission.ActionInspectionEnter, Effect: permission.EffectAllow}}}, p.err
}

func TestSubmitBatchValidatesAndNormalizes(t *testing.T) {
	const actor = "10000000-0000-0000-0000-000000000001"
	scope := Scope{SemesterID: "10000000-0000-0000-0000-000000000002", WeekID: "10000000-0000-0000-0000-000000000003",
		DepartmentID: "10000000-0000-0000-0000-000000000004", InspectionTypeID: "10000000-0000-0000-0000-000000000005"}
	a := VersionedRecord{ID: "10000000-0000-0000-0000-000000000006", Version: 1}
	b := VersionedRecord{ID: "10000000-0000-0000-0000-000000000007", Version: 3}
	repo := &submitRepository{}
	svc := NewService(repo, submitPermissions{})
	for _, records := range [][]VersionedRecord{{b, a}, {a, b}} {
		if _, err := svc.Submit(context.Background(), Actor{ID: actor}, SubmitInput{Scope: scope, Records: records, IdempotencyKey: " request-1 "}, audit.Metadata{}); err != nil {
			t.Fatal(err)
		}
		if !repo.called || repo.last.Records[0].ID != a.ID || repo.last.Records[1].ID != b.ID || repo.last.IdempotencyKey != "request-1" || len(repo.last.Permissions) != 1 {
			t.Fatalf("unexpected command: %+v", repo.last)
		}
	}
	fingerprint := repo.last.Fingerprint
	if _, err := svc.Submit(context.Background(), Actor{ID: actor}, SubmitInput{Scope: scope, Records: []VersionedRecord{b, a}, IdempotencyKey: "request-1"}, audit.Metadata{}); err != nil || repo.last.Fingerprint != fingerprint {
		t.Fatalf("fingerprint changed with input order: %v", err)
	}
	for _, records := range [][]VersionedRecord{nil, {a, a}, {{ID: "bad", Version: 1}}, {{ID: a.ID, Version: 0}}} {
		repo.called = false
		_, err := svc.Submit(context.Background(), Actor{ID: actor}, SubmitInput{Scope: scope, Records: records, IdempotencyKey: "request-1"}, audit.Metadata{})
		if !errors.Is(err, ErrInvalidSubmit) || repo.called {
			t.Fatalf("invalid records %#v reached repository: %v", records, err)
		}
	}
	repo.called = false
	_, err := svc.Submit(context.Background(), Actor{ID: actor}, SubmitInput{Scope: scope, Records: []VersionedRecord{a}}, audit.Metadata{})
	if !errors.Is(err, ErrInvalidSubmit) || repo.called {
		t.Fatalf("missing idempotency key reached repository: %v", err)
	}
}

func TestSubmitBatchFailsClosedOnPermissionError(t *testing.T) {
	repo := &submitRepository{}
	want := errors.New("permission store unavailable")
	svc := NewService(repo, submitPermissions{err: want})
	_, err := svc.Submit(context.Background(), Actor{ID: "10000000-0000-0000-0000-000000000001"}, SubmitInput{
		Scope:   Scope{SemesterID: "10000000-0000-0000-0000-000000000002", WeekID: "10000000-0000-0000-0000-000000000003", DepartmentID: "10000000-0000-0000-0000-000000000004", InspectionTypeID: "10000000-0000-0000-0000-000000000005"},
		Records: []VersionedRecord{{ID: "10000000-0000-0000-0000-000000000006", Version: 1}}, IdempotencyKey: "request-1",
	}, audit.Metadata{})
	if !errors.Is(err, want) || repo.called {
		t.Fatalf("permission failure reached repository: %v", err)
	}
}

func TestDecideRequiresVersionAndReturnReason(t *testing.T) {
	repo := &submitRepository{}
	svc := NewService(repo, submitPermissions{})
	actor := Actor{ID: "10000000-0000-0000-0000-000000000001"}
	id := "10000000-0000-0000-0000-000000000002"
	for _, scenario := range []struct {
		action Action
		input  DecisionInput
	}{{Return, DecisionInput{Version: 1}}, {Confirm, DecisionInput{}}, {Submit, DecisionInput{Version: 1}}} {
		if _, err := svc.Decide(context.Background(), actor, id, scenario.action, scenario.input, audit.Metadata{}); !errors.Is(err, ErrInvalidDecision) {
			t.Fatalf("invalid decision %+v: %v", scenario, err)
		}
	}
	if _, err := svc.Decide(context.Background(), actor, id, Return, DecisionInput{Version: 1, Reason: "  核对  "}, audit.Metadata{}); err != nil || repo.decision.Reason != "核对" || repo.decision.Metadata.Reason != "核对" {
		t.Fatalf("valid return: %+v %v", repo.decision, err)
	}
}
