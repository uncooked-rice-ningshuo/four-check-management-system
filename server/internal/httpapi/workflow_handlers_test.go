package httpapi

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/workflow"
	"github.com/google/uuid"
)

type workflowHTTPStub struct {
	calls    int
	err      error
	actor    workflow.Actor
	input    workflow.SubmitInput
	metadata audit.Metadata
}

func (s *workflowHTTPStub) Decide(_ context.Context, actor workflow.Actor, id string, action workflow.Action, input workflow.DecisionInput, metadata audit.Metadata) (workflow.Submission, error) {
	s.calls++
	s.actor, s.metadata = actor, metadata
	return workflow.Submission{ID: id, Status: workflow.Returned, RowVersion: input.Version + 1, ReturnReason: input.Reason}, s.err
}
func (s *workflowHTTPStub) Resubmit(_ context.Context, actor workflow.Actor, id string, input workflow.ResubmitInput, metadata audit.Metadata) (workflow.Submission, error) {
	s.calls++
	s.actor, s.metadata = actor, metadata
	return workflow.Submission{ID: id, Status: workflow.PendingReview, RevisionNo: 2, RowVersion: input.Version + 1}, s.err
}
func (s *workflowHTTPStub) List(_ context.Context, actor workflow.Actor, query workflow.ListQuery) (workflow.Page, error) {
	s.calls++
	s.actor = actor
	return workflow.Page{Items: []workflow.Submission{}, Page: query.Page, PageSize: query.PageSize}, s.err
}
func (s *workflowHTTPStub) Detail(_ context.Context, actor workflow.Actor, id string) (workflow.Submission, error) {
	s.calls++
	s.actor = actor
	return workflow.Submission{ID: id, RecordIDs: []string{}}, s.err
}

func (s *workflowHTTPStub) Submit(_ context.Context, actor workflow.Actor, input workflow.SubmitInput, metadata audit.Metadata) (workflow.Submission, error) {
	s.calls++
	s.actor, s.input, s.metadata = actor, input, metadata
	return workflow.Submission{ID: uuid.NewString(), Scope: input.Scope, Status: workflow.PendingReview, RevisionNo: 1}, s.err
}

func TestDepartmentSubmitHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	service := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, err := service.Register(context.Background(), auth.RegisterInput{StudentNo: "workflow-member", Name: "workflow", Password: "Password123", RequestedDepartmentID: "department", RequestedOrgRoleID: "member"})
	if err != nil {
		t.Fatal(err)
	}
	account := repo.values["workflow-member"]
	account.Status = auth.StatusActive
	repo.values[account.LoginName] = account
	tokens, err := service.Login(context.Background(), account.LoginName, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	stub := &workflowHTTPStub{}
	router := NewRouter(service, false, AdminServices{Workflow: stub})
	id := uuid.NewString()
	request := workflow.SubmitInput{Scope: workflow.Scope{SemesterID: id, WeekID: id, DepartmentID: id, InspectionTypeID: id},
		Records: []workflow.VersionedRecord{{ID: id, Version: 1}}, IdempotencyKey: "batch-1"}
	path := "/api/v1/department-submissions"
	if response := performJSON(router, "POST", path, request, ""); response.Code != 401 || stub.calls != 0 {
		t.Fatalf("unauthenticated submit: %d calls=%d", response.Code, stub.calls)
	}
	response := performJSON(router, "POST", path, request, tokens.AccessToken)
	if response.Code != 201 || !strings.Contains(response.Body.String(), `"revisionNo":1`) || stub.actor.ID != account.ID || stub.input.IdempotencyKey != "batch-1" || stub.metadata.RequestID == "" {
		t.Fatalf("submit response: %d %s", response.Code, response.Body.String())
	}
	for _, scenario := range []struct {
		err    error
		status int
		code   string
	}{
		{workflow.ErrInvalidSubmit, 400, "INVALID_REQUEST"},
		{workflow.ErrSubmissionScope, 403, "FORBIDDEN_SCOPE"},
		{workflow.ErrRecordUnavailable, 404, "INSPECTION_NOT_FOUND"},
		{workflow.ErrVersionConflict, 409, "VERSION_CONFLICT"},
		{workflow.ErrIdempotencyReuse, 409, "IDEMPOTENCY_CONFLICT"},
		{errors.New("private SQL failure"), 500, "SUBMISSION_FAILED"},
	} {
		stub.err = scenario.err
		response = performJSON(router, "POST", path, request, tokens.AccessToken)
		if response.Code != scenario.status || !strings.Contains(response.Body.String(), `"code":"`+scenario.code+`"`) || strings.Contains(response.Body.String(), "private SQL") {
			t.Fatalf("%v: %d %s", scenario.err, response.Code, response.Body.String())
		}
	}
	stub.err = nil
	response = performJSON(router, "POST", path, map[string]any{"scope": request.Scope, "records": request.Records, "idempotencyKey": "batch-1", "unexpected": true}, tokens.AccessToken)
	if response.Code != 400 {
		t.Fatalf("unknown input: %d %s", response.Code, response.Body.String())
	}
	account.MustChangePassword = true
	repo.values[account.LoginName] = account
	response = performJSON(router, "POST", path, request, tokens.AccessToken)
	if response.Code != 403 || !strings.Contains(response.Body.String(), "PASSWORD_CHANGE_REQUIRED") {
		t.Fatalf("password guard: %d %s", response.Code, response.Body.String())
	}
}

func TestDepartmentDecisionHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	service := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, err := service.Register(context.Background(), auth.RegisterInput{StudentNo: "workflow-reviewer", Name: "reviewer", Password: "Password123", RequestedDepartmentID: "department", RequestedOrgRoleID: "member"})
	if err != nil {
		t.Fatal(err)
	}
	account := repo.values["workflow-reviewer"]
	account.Status = auth.StatusActive
	repo.values[account.LoginName] = account
	tokens, err := service.Login(context.Background(), account.LoginName, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	stub := &workflowHTTPStub{}
	router := NewRouter(service, false, AdminServices{Workflow: stub})
	path := "/api/v1/department-submissions/" + uuid.NewString()
	for _, action := range []string{"return", "confirm"} {
		request := workflow.DecisionInput{Version: 1, Reason: "核对"}
		url := path + "/" + action
		previousCalls := stub.calls
		if response := performJSON(router, "POST", url, request, ""); response.Code != 401 || stub.calls != previousCalls {
			t.Fatalf("unauthenticated %s: %d calls=%d", action, response.Code, stub.calls)
		}
		response := performJSON(router, "POST", url, request, tokens.AccessToken)
		if response.Code != 200 || !strings.Contains(response.Body.String(), `"rowVersion":2`) || stub.actor.ID != account.ID {
			t.Fatalf("%s: %d %s", action, response.Code, response.Body.String())
		}
		for _, scenario := range []struct {
			err    error
			status int
			code   string
		}{
			{workflow.ErrInvalidDecision, 400, "INVALID_REQUEST"},
			{workflow.ErrSubmissionMissing, 404, "SUBMISSION_NOT_FOUND"},
			{workflow.ErrSubmissionScope, 403, "FORBIDDEN_SCOPE"},
			{workflow.ErrVersionConflict, 409, "VERSION_CONFLICT"},
			{workflow.ErrInvalidTransition, 409, "INVALID_SUBMISSION_STATE"},
			{errors.New("private SQL failure"), 500, "SUBMISSION_FAILED"},
		} {
			stub.err = scenario.err
			response := performJSON(router, "POST", url, request, tokens.AccessToken)
			if response.Code != scenario.status || !strings.Contains(response.Body.String(), `"code":"`+scenario.code+`"`) || strings.Contains(response.Body.String(), "private SQL") {
				t.Fatalf("%s %v: %d %s", action, scenario.err, response.Code, response.Body.String())
			}
		}
		stub.err = nil
	}
}

func TestDepartmentResubmitHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	service := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, err := service.Register(context.Background(), auth.RegisterInput{StudentNo: "workflow-resubmit", Name: "resubmit", Password: "Password123", RequestedDepartmentID: "department", RequestedOrgRoleID: "member"})
	if err != nil {
		t.Fatal(err)
	}
	account := repo.values["workflow-resubmit"]
	account.Status = auth.StatusActive
	repo.values[account.LoginName] = account
	tokens, err := service.Login(context.Background(), account.LoginName, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	stub := &workflowHTTPStub{}
	router := NewRouter(service, false, AdminServices{Workflow: stub})
	id := uuid.NewString()
	path := "/api/v1/department-submissions/" + id + "/resubmit"
	request := workflow.ResubmitInput{Version: 2, Records: []workflow.VersionedRecord{{ID: uuid.NewString(), Version: 3}}}
	if response := performJSON(router, "POST", path, request, ""); response.Code != 401 || stub.calls != 0 {
		t.Fatalf("unauthenticated resubmit: %d calls=%d", response.Code, stub.calls)
	}
	response := performJSON(router, "POST", path, request, tokens.AccessToken)
	if response.Code != 200 || !strings.Contains(response.Body.String(), `"revisionNo":2`) || stub.actor.ID != account.ID {
		t.Fatalf("resubmit: %d %s", response.Code, response.Body.String())
	}
	for _, scenario := range []struct {
		err    error
		status int
		code   string
	}{
		{workflow.ErrInvalidSubmit, 400, "INVALID_REQUEST"},
		{workflow.ErrSubmissionMissing, 404, "SUBMISSION_NOT_FOUND"},
		{workflow.ErrSubmissionScope, 403, "FORBIDDEN_SCOPE"},
		{workflow.ErrVersionConflict, 409, "VERSION_CONFLICT"},
		{workflow.ErrInvalidTransition, 409, "INVALID_SUBMISSION_STATE"},
		{errors.New("private SQL failure"), 500, "SUBMISSION_FAILED"},
	} {
		stub.err = scenario.err
		response = performJSON(router, "POST", path, request, tokens.AccessToken)
		if response.Code != scenario.status || !strings.Contains(response.Body.String(), `"code":"`+scenario.code+`"`) || strings.Contains(response.Body.String(), "private SQL") {
			t.Fatalf("%v: %d %s", scenario.err, response.Code, response.Body.String())
		}
	}
}

func TestDepartmentReadHTTPContracts(t *testing.T) {
	repo := &accounts{values: map[string]auth.Account{}}
	service := auth.NewService(repo, &sessions{values: map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, err := service.Register(context.Background(), auth.RegisterInput{StudentNo: "workflow-reader", Name: "reader", Password: "Password123", RequestedDepartmentID: "department", RequestedOrgRoleID: "member"})
	if err != nil {
		t.Fatal(err)
	}
	account := repo.values["workflow-reader"]
	account.Status = auth.StatusActive
	repo.values[account.LoginName] = account
	tokens, err := service.Login(context.Background(), account.LoginName, "Password123")
	if err != nil {
		t.Fatal(err)
	}
	stub := &workflowHTTPStub{}
	router := NewRouter(service, false, AdminServices{Workflow: stub})
	for _, path := range []string{"/api/v1/department-submissions?page=1&pageSize=20", "/api/v1/department-submissions/" + uuid.NewString()} {
		previousCalls := stub.calls
		if response := performJSON(router, "GET", path, nil, ""); response.Code != 401 || stub.calls != previousCalls {
			t.Fatalf("unauthenticated %s: %d", path, response.Code)
		}
		response := performJSON(router, "GET", path, nil, tokens.AccessToken)
		if response.Code != 200 || stub.actor.ID != account.ID {
			t.Fatalf("read %s: %d %s", path, response.Code, response.Body.String())
		}
		for _, scenario := range []struct {
			err    error
			status int
			code   string
		}{
			{workflow.ErrInvalidSubmit, 400, "INVALID_REQUEST"},
			{workflow.ErrSubmissionMissing, 404, "SUBMISSION_NOT_FOUND"},
			{errors.New("private SQL failure"), 500, "SUBMISSION_FAILED"},
		} {
			stub.err = scenario.err
			response = performJSON(router, "GET", path, nil, tokens.AccessToken)
			if response.Code != scenario.status || !strings.Contains(response.Body.String(), `"code":"`+scenario.code+`"`) || strings.Contains(response.Body.String(), "private SQL") {
				t.Fatalf("%s %v: %d %s", path, scenario.err, response.Code, response.Body.String())
			}
		}
		stub.err = nil
	}
	response := performJSON(router, "GET", "/api/v1/department-submissions?page=bad", nil, tokens.AccessToken)
	if response.Code != 400 {
		t.Fatalf("invalid query: %d %s", response.Code, response.Body.String())
	}
}
