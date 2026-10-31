package reporting

import (
	"context"
	"errors"
	"testing"

	"four-check-management-system/server/internal/permission"
)

type reportArchiveRepositoryStub struct {
	*reportingRepositoryStub
	query     ReportArchiveQuery
	page      ReportArchivePage
	workflow  ReportWorkflow
	action    string
	listCalls int
	readCalls int
}

func (r *reportArchiveRepositoryStub) ListReportArchive(_ context.Context, query ReportArchiveQuery,
	permissions []permission.EffectivePermission) (ReportArchivePage, error) {
	r.listCalls++
	r.query = query
	r.permissions = permissions
	return r.page, nil
}

func (r *reportArchiveRepositoryStub) GetReportWorkflowForAction(_ context.Context, _, _ string,
	permissions []permission.EffectivePermission, action string) (ReportWorkflow, error) {
	r.readCalls++
	r.permissions = permissions
	r.action = action
	return r.workflow, nil
}

func TestArchiveRequiresArchiveGrantAndValidPagination(t *testing.T) {
	repository := &reportArchiveRepositoryStub{reportingRepositoryStub: &reportingRepositoryStub{}}
	service := NewService(repository, reportingPermissionsStub{})

	if _, err := service.Archive(context.Background(), "actor", ReportArchiveQuery{Page: 1, PageSize: 101}); !errors.Is(err, ErrInvalidReportArchiveQuery) {
		t.Fatalf("oversized page query error = %v", err)
	}
	if _, err := service.Archive(context.Background(), "actor", ReportArchiveQuery{Page: 1, PageSize: 20}); !errors.Is(err, ErrForbidden) {
		t.Fatalf("archive query without archive permission error = %v", err)
	}
	if repository.listCalls != 0 {
		t.Fatalf("archive repository called %d times without a valid grant", repository.listCalls)
	}
}

func TestArchiveDefaultsPaginationAndPassesOnlyArchivePermissions(t *testing.T) {
	archiveGrant := permission.EffectivePermission{
		Action: permission.ActionArchiveView, Effect: permission.EffectAllow,
		Scope: permission.Scope{AcademicYearID: "year-1"},
	}
	viewGrant := permission.EffectivePermission{Action: permission.ActionReportView, Effect: permission.EffectAllow}
	repository := &reportArchiveRepositoryStub{
		reportingRepositoryStub: &reportingRepositoryStub{},
		page:                    ReportArchivePage{Items: []ReportArchiveEntry{}, Total: 0, Page: 1, PageSize: 20},
	}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{
		Permissions: []permission.EffectivePermission{viewGrant, archiveGrant},
	}})

	page, err := service.Archive(context.Background(), "actor", ReportArchiveQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if repository.listCalls != 1 || repository.query.Page != 1 || repository.query.PageSize != 20 {
		t.Fatalf("archive query defaults/calls = %+v/%d", repository.query, repository.listCalls)
	}
	if len(repository.permissions) != 1 || repository.permissions[0].Action != permission.ActionArchiveView || page.PageSize != 20 {
		t.Fatalf("archive result or permissions = %+v / %+v", page, repository.permissions)
	}
}

func TestArchiveWorkflowIncludesGeneratedDraftsAndUsesArchiveScope(t *testing.T) {
	grant := permission.EffectivePermission{Action: permission.ActionArchiveView, Effect: permission.EffectAllow}
	repository := &reportArchiveRepositoryStub{
		reportingRepositoryStub: &reportingRepositoryStub{},
		workflow:                ReportWorkflow{Report: &ReportState{Status: ReportStatusDraft, VersionNo: 1}},
	}
	service := NewService(repository, reportingPermissionsStub{result: permission.EffectiveResult{Permissions: []permission.EffectivePermission{grant}}})
	semesterID := "a0000000-0000-0000-0000-000000000003"
	weekID := "a0000000-0000-0000-0000-000000000004"

	if workflow, err := service.ArchiveWorkflow(context.Background(), "actor", semesterID, weekID); err != nil ||
		workflow.Report == nil || workflow.Report.Status != ReportStatusDraft {
		t.Fatalf("generated draft archive read = %+v, %v", workflow, err)
	}
	if repository.readCalls != 1 || repository.action != permission.ActionArchiveView || len(repository.permissions) != 1 {
		t.Fatalf("archive workflow authorization = calls %d action %q permissions %+v", repository.readCalls, repository.action, repository.permissions)
	}

	repository.workflow.Report.Status = ReportStatusFinalized
	if workflow, err := service.ArchiveWorkflow(context.Background(), "actor", semesterID, weekID); err != nil || workflow.Report.Status != ReportStatusFinalized {
		t.Fatalf("finalized report archive read = %+v, %v", workflow, err)
	}
}
