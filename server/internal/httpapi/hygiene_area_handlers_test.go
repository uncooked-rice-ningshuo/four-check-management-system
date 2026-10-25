package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
)

func TestHygieneAreaOptionsAndReferencesRespectLiveGrants(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, err := svc.Register(context.Background(), auth.RegisterInput{StudentNo: "area-options", Name: "区域测试", Password: "Password123", RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1"})
	if err != nil {
		t.Fatal(err)
	}
	account := repository.values["area-options"]
	account.Status = auth.StatusActive
	repository.values["area-options"] = account
	tokens, err := svc.Login(context.Background(), "area-options", "Password123")
	if err != nil {
		t.Fatal(err)
	}
	const departmentID = "10000000-0000-0000-0000-000000000002"
	const typeID = "30000000-0000-0000-0000-000000000003"
	permissions := &permissionRepository{profile: permission.AccessProfile{SystemRole: permission.SystemRoleMember, Rules: []permission.Rule{{Action: permission.ActionMasterDataManage, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentID, InspectionTypeID: typeID}, Enabled: true}}}}
	otherDepartmentID := "10000000-0000-0000-0000-000000000004"
	organizationRepo := &attendanceOrganizationRepository{catalogs: organization.Catalogs{Departments: []organization.Department{{ID: departmentID, Name: "区域部门", Enabled: true}, {ID: otherDepartmentID, Name: "其他区域部门", Enabled: true}}, InspectionTypes: []organization.InspectionType{{ID: typeID, Code: "CLASSROOM_HYGIENE", Name: "教室卫生", Enabled: true}}}, scopes: []organization.DepartmentInspectionScope{{DepartmentID: departmentID, InspectionTypeID: typeID, Enabled: true}, {DepartmentID: otherDepartmentID, InspectionTypeID: typeID, Enabled: true}}}
	masterRepo := &masterRepository{records: map[string]masterdata.Record{"classrooms:room": {ID: "room", Kind: masterdata.KindClassroom, Name: "教学楼 101", Enabled: true}}}
	router := NewRouter(svc, false, AdminServices{Permission: permission.NewService(permissions), Organization: organization.NewService(organizationRepo), MasterData: masterdata.NewService(masterRepo)})
	const optionsPath = "/api/v1/admin/hygiene-areas/options"
	const referencePath = "/api/v1/admin/hygiene-areas/classroom-hygiene-areas/references?departmentId=" + departmentID
	for _, path := range []string{optionsPath, referencePath} {
		response := performJSON(router, http.MethodGet, path, nil, "")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("anonymous status=%d", response.Code)
		}
	}
	response := performJSON(router, http.MethodGet, optionsPath, nil, tokens.AccessToken)
	var targets []hygieneAreaTarget
	if response.Code != http.StatusOK || json.Unmarshal(response.Body.Bytes(), &targets) != nil || len(targets) != 2 || targets[0].Kind != masterdata.KindClassroomHygieneArea || targets[1].Kind != masterdata.KindClassroomHygieneArea {
		t.Fatalf("targets: %d %s", response.Code, response.Body.String())
	}
	response = performJSON(router, http.MethodGet, referencePath+"&page=2", nil, tokens.AccessToken)
	if response.Code != http.StatusOK || masterRepo.lastQuery.Kind != masterdata.KindClassroom || masterRepo.lastQuery.Page != 2 || masterRepo.lastQuery.PageSize != 100 || masterRepo.lastQuery.Enabled == nil || !*masterRepo.lastQuery.Enabled {
		t.Fatalf("references: %d query=%+v", response.Code, masterRepo.lastQuery)
	}
	response = performJSON(router, http.MethodGet, referencePath+"&page=bad", nil, tokens.AccessToken)
	var apiError map[string]any
	if response.Code != http.StatusBadRequest || json.Unmarshal(response.Body.Bytes(), &apiError) != nil || apiError["code"] != "INVALID_REQUEST" {
		t.Fatalf("invalid pagination: %d %s", response.Code, response.Body.String())
	}
	permissions.profile.Rules = append(permissions.profile.Rules, permission.Rule{Action: permission.ActionMasterDataManage, Effect: permission.EffectDeny, Scope: permission.Scope{InspectionTypeID: typeID}, Enabled: true})
	response = performJSON(router, http.MethodGet, optionsPath, nil, tokens.AccessToken)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("DENY targets: %d %s", response.Code, response.Body.String())
	}
	response = performJSON(router, http.MethodGet, referencePath, nil, tokens.AccessToken)
	if response.Code != http.StatusForbidden || json.Unmarshal(response.Body.Bytes(), &apiError) != nil || apiError["code"] != "FORBIDDEN" {
		t.Fatalf("DENY references: %d %s", response.Code, response.Body.String())
	}
	permissions.profile.Rules = nil
	response = performJSON(router, http.MethodGet, optionsPath, nil, tokens.AccessToken)
	if response.Code != http.StatusOK || response.Body.String() != "[]" {
		t.Fatalf("ungranted targets: %d %s", response.Code, response.Body.String())
	}
}
