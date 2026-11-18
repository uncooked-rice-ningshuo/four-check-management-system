package disciplineconfig

import (
	"context"
	"testing"
)

type repositoryStub struct {
	types       []InspectionType
	definitions []TierDefinition
	saved       Version
}

func (r *repositoryStub) ListInspectionTypes(context.Context) ([]InspectionType, error) {
	return r.types, nil
}
func (r *repositoryStub) ListDisciplineTierDefinitions(context.Context) ([]TierDefinition, error) {
	if r.definitions != nil {
		return r.definitions, nil
	}
	return defaultTierDefinitions(), nil
}
func (r *repositoryStub) Current(context.Context) (Version, bool, error) {
	return Version{}, false, nil
}
func (r *repositoryStub) ListVersions(context.Context) ([]Version, error)      { return nil, nil }
func (r *repositoryStub) GetVersion(context.Context, string) (Version, error)  { return r.saved, nil }
func (r *repositoryStub) CreateDraft(context.Context, string) (Version, error) { return Version{}, nil }
func (r *repositoryStub) SaveDraft(_ context.Context, v Version, _ int, _ string) (Version, error) {
	r.saved = v
	return v, nil
}
func (r *repositoryStub) Publish(context.Context, string, int, string) (Version, error) {
	return Version{}, nil
}

func TestSaveDraftNormalizesUnitsAndPreservesCustomTierName(t *testing.T) {
	repository := &repositoryStub{types: []InspectionType{{ID: "a", Code: "ATTENDANCE_RATE", Name: "目录权威名称", Enabled: true}, {ID: "b", Code: "LATE_RETURN", Name: "晚归", Enabled: true}}, definitions: []TierDefinition{{Code: "WARNING", Name: "管理员维护警告名称", SortOrder: 20}}}
	service := NewService(repository)
	version := Version{ID: "version", Status: StatusDraft, RowVersion: 1, Groups: []Group{{Key: "group", Name: "累计组", Terms: []Term{{InspectionTypeID: "a", InspectionTypeName: "客户端提供的名称", SourceUnit: "伪造", Operator: "+", Factor: 1}, {InspectionTypeID: "b", SourceUnit: "学时", Operator: "+", Factor: 1}}, Tiers: []Tier{{Code: "WARNING", Name: "伪造处分名", Lower: 0, Clause: "条款", TeacherConfirmationRequired: true}}}}}
	if _, err := service.SaveDraft(context.Background(), version, 1, "actor"); err != nil {
		t.Fatal(err)
	}
	if got := repository.saved.Groups[0].Terms[0].InspectionTypeName; got != "目录权威名称" {
		t.Fatalf("saved type name = %q", got)
	}
	if got := repository.saved.Groups[0].Terms[0].SourceUnit; got != "学时" {
		t.Fatalf("saved source unit = %q", got)
	}
	if got := repository.saved.Groups[0].Tiers[0].Name; got != "伪造处分名" {
		t.Fatalf("saved tier label = %q", got)
	}
	if got := repository.saved.Groups[0].Terms[1].SourceUnit; got != "次" {
		t.Fatalf("saved non-attendance source unit = %q", got)
	}
}
