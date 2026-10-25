package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/permission"
)

type accounts struct {
	values    map[string]auth.Account
	reviewErr error
}

func TestPublicAccountIncludesDisplayAssignmentWithoutPrivateData(t *testing.T) {
	value := publicAccount(auth.Account{
		ID: "member-1", Name: "示例用户", SystemRole: auth.SystemRoleMember,
		PrimaryDepartmentName: "学习部", OrganizationRoleName: "委员", PasswordHash: "private-hash",
	})
	if value["primaryDepartmentName"] != "学习部" || value["organizationRoleName"] != "委员" {
		t.Fatal("missing display assignment", value)
	}
	if _, exposed := value["passwordHash"]; exposed {
		t.Fatal("password hash leaked")
	}
}

type readinessStub struct{ err error }

func (s readinessStub) Ready(context.Context) error { return s.err }

func (r *accounts) Create(_ context.Context, a auth.Account, _ audit.Mutation) error {
	if _, ok := r.values[a.LoginName]; ok {
		return auth.ErrLoginNameExists
	}
	r.values[a.LoginName] = a
	return nil
}
func (r *accounts) FindByLoginName(_ context.Context, no string) (auth.Account, error) {
	a, ok := r.values[no]
	if !ok {
		return auth.Account{}, auth.ErrAccountNotFound
	}
	return a, nil
}
func (r *accounts) UpdatePassword(_ context.Context, id, hash string, mustChange bool, version int, _ audit.Mutation) error {
	for key, value := range r.values {
		if value.ID == id {
			if value.SessionVersion != version {
				return auth.ErrVersionConflict
			}
			value.PasswordHash, value.MustChangePassword = hash, mustChange
			value.SessionVersion++
			r.values[key] = value
			return nil
		}
	}
	return auth.ErrAccountNotFound
}
func (r *accounts) ListPending(_ context.Context) ([]auth.Account, error) {
	var result []auth.Account
	for _, value := range r.values {
		if value.Status == auth.StatusPending {
			result = append(result, value)
		}
	}
	return result, nil
}
func (r *accounts) ReviewRegistration(_ context.Context, id, _ string, decision, _ string, _ time.Time, _ audit.Mutation) (auth.Account, error) {
	if r.reviewErr != nil {
		return auth.Account{}, r.reviewErr
	}
	for key, value := range r.values {
		if value.ID == id {
			if value.Status != auth.StatusPending {
				return auth.Account{}, auth.ErrInvalidTransition
			}
			if decision == auth.ReviewApprove {
				value.Status = auth.StatusActive
			} else {
				value.Status = auth.StatusRejected
			}
			r.values[key] = value
			return value, nil
		}
	}
	return auth.Account{}, auth.ErrAccountNotFound
}
func (r *accounts) FindByID(_ context.Context, id string) (auth.Account, error) {
	for _, a := range r.values {
		if a.ID == id {
			return a, nil
		}
	}
	return auth.Account{}, auth.ErrAccountNotFound
}
func (r *accounts) UpdateAccount(_ context.Context, account auth.Account, _ audit.Mutation) (auth.Account, error) {
	for key, value := range r.values {
		if value.ID == account.ID {
			if value.SessionVersion != account.SessionVersion {
				return auth.Account{}, auth.ErrVersionConflict
			}
			delete(r.values, key)
			account.PasswordHash = value.PasswordHash
			account.SessionVersion = value.SessionVersion + 1
			r.values[account.LoginName] = account
			return account, nil
		}
	}
	return auth.Account{}, auth.ErrAccountNotFound
}
func (r *accounts) SetStatus(_ context.Context, id, status string, expectedVersion int, _ audit.Mutation) (auth.Account, error) {
	for key, value := range r.values {
		if value.ID == id {
			if value.SessionVersion != expectedVersion {
				return auth.Account{}, auth.ErrVersionConflict
			}
			value.Status = status
			value.SessionVersion++
			r.values[key] = value
			return value, nil
		}
	}
	return auth.Account{}, auth.ErrAccountNotFound
}

type sessions struct {
	values map[string]auth.RefreshSession
}

type permissionRepository struct{ profile permission.AccessProfile }

func (r *permissionRepository) LoadAccessProfile(context.Context, string) (permission.AccessProfile, error) {
	return r.profile, nil
}
func (r *permissionRepository) ListRules(context.Context, string) ([]permission.Rule, error) {
	return r.profile.Rules, nil
}
func (r *permissionRepository) CreateRule(_ context.Context, value permission.Rule, _ audit.Mutation) (permission.Rule, error) {
	r.profile.Rules = append(r.profile.Rules, value)
	return value, nil
}
func (r *permissionRepository) UpdateRule(_ context.Context, value permission.Rule, _ audit.Mutation) (permission.Rule, error) {
	return value, nil
}
func (r *permissionRepository) DeleteRule(context.Context, string, string, audit.Mutation) error {
	return nil
}

type masterRepository struct {
	records   map[string]masterdata.Record
	createErr error
	lastQuery masterdata.Query
}

func (r *masterRepository) ListMasterData(_ context.Context, query masterdata.Query) (masterdata.Page, error) {
	r.lastQuery = query
	var items []masterdata.Record
	for _, record := range r.records {
		if record.Kind == query.Kind {
			items = append(items, record)
		}
	}
	return masterdata.Page{Items: items, Total: len(items), Page: query.Page, PageSize: query.PageSize}, nil
}
func (r *masterRepository) FindMasterData(_ context.Context, kind, id string) (masterdata.Record, error) {
	record, ok := r.records[kind+":"+id]
	if !ok {
		return masterdata.Record{}, masterdata.ErrNotFound
	}
	return record, nil
}
func (r *masterRepository) MasterDataExists(_ context.Context, kind, id string) (bool, error) {
	_, ok := r.records[kind+":"+id]
	return ok, nil
}
func (r *masterRepository) AcademicSemesterExists(_ context.Context, id string) (bool, error) {
	return id == "semester-1", nil
}
func (r *masterRepository) CreateMasterData(_ context.Context, _ string, record masterdata.Record, _ audit.Mutation) (masterdata.Record, error) {
	if r.createErr != nil {
		return masterdata.Record{}, r.createErr
	}
	r.records[record.Kind+":"+record.ID] = record
	return record, nil
}
func (r *masterRepository) UpdateMasterData(_ context.Context, _ string, record masterdata.Record, _ audit.Mutation) (masterdata.Record, error) {
	r.records[record.Kind+":"+record.ID] = record
	return record, nil
}
func (r *masterRepository) ListMasterDataChanges(context.Context, string, string) ([]masterdata.Change, error) {
	return nil, nil
}
func (r *masterRepository) HygieneAreaInspectionScope(context.Context, string, string) (string, error) {
	return "30000000-0000-0000-0000-000000000003", nil
}

type dictionaryRepository struct {
	types []dictionary.Type
	items []dictionary.Item
}

func (r *dictionaryRepository) ListDictionaryTypes(context.Context) ([]dictionary.Type, error) {
	return r.types, nil
}
func (r *dictionaryRepository) ListDictionaryItems(context.Context, string) ([]dictionary.Item, error) {
	return r.items, nil
}
func (r *dictionaryRepository) FindDictionaryType(_ context.Context, id string) (dictionary.Type, error) {
	for _, value := range r.types {
		if value.ID == id {
			return value, nil
		}
	}
	return dictionary.Type{}, dictionary.ErrNotFound
}
func (r *dictionaryRepository) CreateDictionaryType(_ context.Context, value dictionary.Type, _ audit.Mutation) (dictionary.Type, error) {
	r.types = append(r.types, value)
	return value, nil
}
func (r *dictionaryRepository) UpdateDictionaryType(_ context.Context, value dictionary.Type, _ audit.Mutation) (dictionary.Type, error) {
	return value, nil
}
func (r *dictionaryRepository) CreateDictionaryItem(_ context.Context, value dictionary.Item, _ audit.Mutation) (dictionary.Item, error) {
	r.items = append(r.items, value)
	return value, nil
}
func (r *dictionaryRepository) UpdateDictionaryItem(_ context.Context, value dictionary.Item, _ audit.Mutation) (dictionary.Item, error) {
	return value, nil
}

func (r *sessions) Save(_ context.Context, s auth.RefreshSession, _ int) error {
	r.values[s.TokenHash] = s
	return nil
}
func (r *sessions) FindRefreshSession(_ context.Context, h string, now time.Time) (auth.RefreshSession, error) {
	s, ok := r.values[h]
	if !ok || s.RevokedAt != nil || !s.ExpiresAt.After(now) {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	return s, nil
}
func (r *sessions) Rotate(ctx context.Context, hash string, now time.Time, next auth.RefreshSession, _ int) error {
	s, err := r.FindRefreshSession(ctx, hash, now)
	if err != nil {
		return err
	}
	if s.AccountID != next.AccountID {
		return auth.ErrInvalidRefreshToken
	}
	s.RevokedAt = &now
	r.values[hash] = s
	r.values[next.TokenHash] = next
	return nil
}
func (r *sessions) Revoke(_ context.Context, h string, now time.Time) error {
	s := r.values[h]
	s.RevokedAt = &now
	r.values[h] = s
	return nil
}
func (r *sessions) RevokeAll(_ context.Context, accountID string, now time.Time) error {
	for key, value := range r.values {
		if value.AccountID == accountID {
			value.RevokedAt = &now
			r.values[key] = value
		}
	}
	return nil
}

func (r *sessions) CheckAccessSession(_ context.Context, accountID, sessionID string, now time.Time) error {
	for _, session := range r.values {
		if session.AccountID == accountID && session.LoginSessionID == sessionID && session.RevokedAt == nil && session.ExpiresAt.After(now) {
			return nil
		}
	}
	return auth.ErrInvalidAccessToken
}

func TestRegisterLoginAndMe(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	r := NewRouter(svc, false)

	register := performJSON(r, http.MethodPost, "/api/v1/auth/register", map[string]string{"studentNo": "22010101", "name": "张三", "password": "Password123", "requestedDepartmentId": "department-1", "requestedOrgRoleId": "role-1"}, "")
	if register.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", register.Code, register.Body.String())
	}
	registered, _ := repository.FindByLoginName(context.Background(), "22010101")
	registered.Status = auth.StatusActive
	repository.values[registered.LoginName] = registered
	login := performJSON(r, http.MethodPost, "/api/v1/auth/login", map[string]string{"loginName": "22010101", "password": "Password123"}, "")
	if login.Code != http.StatusOK {
		t.Fatalf("login status=%d body=%s", login.Code, login.Body.String())
	}
	var body struct {
		AccessToken string `json:"accessToken"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &body)
	me := performJSON(r, http.MethodGet, "/api/v1/auth/me", nil, body.AccessToken)
	if me.Code != http.StatusOK {
		t.Fatalf("me status=%d body=%s", me.Code, me.Body.String())
	}
}

func TestRegistrationReviewListNeverExposesPasswordHash(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	_, _ = svc.Register(context.Background(), auth.RegisterInput{StudentNo: "22010104", Name: "赵六", Password: "Password123", RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1"})
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")

	response := performJSON(NewRouter(svc, false), http.MethodGet, "/api/v1/admin/registration-requests", nil, tokens.AccessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("passwordHash")) || bytes.Contains(response.Body.Bytes(), []byte("$2")) {
		t.Fatalf("response leaked password hash: %s", response.Body.String())
	}
}

func TestRegistrationReviewReturnsActionableErrorForRepositoryFailure(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
	repository.reviewErr = errors.New("database unavailable")

	response := performJSON(NewRouter(svc, false), http.MethodPost, "/api/v1/admin/registration-requests/request-1/review", map[string]string{"decision": auth.ReviewApprove, "reason": ""}, tokens.AccessToken)
	if response.Code != http.StatusInternalServerError || !bytes.Contains(response.Body.Bytes(), []byte("REGISTRATION_REVIEW_FAILED")) || !bytes.Contains(response.Body.Bytes(), []byte("审核失败，请稍后重试")) {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestSuperAdminCanCreateUpdateDisableAndResetUser(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	sessionsRepository := &sessions{map[string]auth.RefreshSession{}}
	svc := auth.NewService(repository, sessionsRepository, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
	router := NewRouter(svc, false)

	createdResponse := performJSON(router, http.MethodPost, "/api/v1/admin/users", map[string]any{"studentNo": "24010001", "name": "管理员创建", "systemRole": "MEMBER"}, tokens.AccessToken)
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", createdResponse.Code, createdResponse.Body.String())
	}
	var created struct {
		ID                string `json:"id"`
		TemporaryPassword string `json:"temporaryPassword"`
	}
	_ = json.Unmarshal(createdResponse.Body.Bytes(), &created)
	if len(created.TemporaryPassword) < 12 || createdResponse.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("created credential response invalid")
	}
	if _, err := svc.Login(context.Background(), "24010001", created.TemporaryPassword); err != nil {
		t.Fatalf("generated credential cannot log in: %v", err)
	}
	updated := performJSON(router, http.MethodPut, "/api/v1/admin/users/"+created.ID, map[string]any{"studentNo": "24010001", "name": "已修改", "systemRole": "ADMIN", "version": 1}, tokens.AccessToken)
	if updated.Code != http.StatusOK {
		t.Fatalf("update status=%d body=%s", updated.Code, updated.Body.String())
	}
	var updatedResult struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(updated.Body.Bytes(), &updatedResult); err != nil || updatedResult.Version != 2 {
		t.Fatalf("update version=%d, error=%v", updatedResult.Version, err)
	}
	staleStatus := performJSON(router, http.MethodPut, "/api/v1/admin/users/"+created.ID+"/status", map[string]any{"status": "DISABLED", "version": 1}, tokens.AccessToken)
	if staleStatus.Code != http.StatusConflict || !strings.Contains(staleStatus.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("stale status change status=%d body=%s", staleStatus.Code, staleStatus.Body.String())
	}
	staleReset := performJSON(router, http.MethodPost, "/api/v1/admin/users/"+created.ID+"/reset-password", map[string]int{"version": 1}, tokens.AccessToken)
	if staleReset.Code != http.StatusConflict || !strings.Contains(staleReset.Body.String(), "VERSION_CONFLICT") {
		t.Fatalf("stale password reset status=%d body=%s", staleReset.Code, staleReset.Body.String())
	}
	disabled := performJSON(router, http.MethodPut, "/api/v1/admin/users/"+created.ID+"/status", map[string]any{"status": "DISABLED", "version": 2}, tokens.AccessToken)
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%s", disabled.Code, disabled.Body.String())
	}
	var disabledResult struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(disabled.Body.Bytes(), &disabledResult); err != nil || disabledResult.Version != 3 {
		t.Fatalf("disabled version=%d, error=%v", disabledResult.Version, err)
	}
	reset := performJSON(router, http.MethodPost, "/api/v1/admin/users/"+created.ID+"/reset-password", map[string]int{"version": 3}, tokens.AccessToken)
	if reset.Code != http.StatusOK {
		t.Fatalf("reset status=%d body=%s", reset.Code, reset.Body.String())
	}
	var resetResult struct {
		TemporaryPassword string `json:"temporaryPassword"`
	}
	_ = json.Unmarshal(reset.Body.Bytes(), &resetResult)
	if len(resetResult.TemporaryPassword) < 12 || resetResult.TemporaryPassword == created.TemporaryPassword || reset.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("reset credential response invalid")
	}
}

func TestSuperAdminCanReadEffectivePermissionSourcesAndCreateRule(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
	permissionRepo := &permissionRepository{profile: permission.AccessProfile{AccountID: "user-1", SystemRole: permission.SystemRoleMember}}
	router := NewRouter(svc, false, AdminServices{Permission: permission.NewService(permissionRepo)})

	created := performJSON(router, http.MethodPost, "/api/v1/admin/users/user-1/permission-rules", map[string]any{"action": permission.ActionInspectionView, "effect": permission.EffectAllow, "scope": map[string]string{"semesterId": "semester-1"}}, tokens.AccessToken)
	if created.Code != http.StatusCreated {
		t.Fatalf("create rule status=%d body=%s", created.Code, created.Body.String())
	}
	details := performJSON(router, http.MethodGet, "/api/v1/admin/users/user-1/permissions", nil, tokens.AccessToken)
	if details.Code != http.StatusOK || !bytes.Contains(details.Body.Bytes(), []byte("用户补充授权")) {
		t.Fatalf("permissions status=%d body=%s", details.Code, details.Body.String())
	}
}

func TestSuperAdminCanCreateAndListMasterData(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
	router := NewRouter(svc, false, AdminServices{MasterData: masterdata.NewService(&masterRepository{records: map[string]masterdata.Record{}})})
	created := performJSON(router, http.MethodPost, "/api/v1/admin/master-data/faculties", map[string]any{"code": "CS", "name": "计算机学院"}, tokens.AccessToken)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	listed := performJSON(router, http.MethodGet, "/api/v1/admin/master-data/faculties?page=1&pageSize=20", nil, tokens.AccessToken)
	if listed.Code != http.StatusOK || !bytes.Contains(listed.Body.Bytes(), []byte("计算机学院")) {
		t.Fatalf("list status=%d body=%s", listed.Code, listed.Body.String())
	}
}

func TestSuperAdminCanCreateGradeWithoutCode(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
	router := NewRouter(svc, false, AdminServices{MasterData: masterdata.NewService(&masterRepository{records: map[string]masterdata.Record{}})})

	created := performJSON(router, http.MethodPost, "/api/v1/admin/master-data/grades", map[string]any{"name": "2027"}, tokens.AccessToken)
	if created.Code != http.StatusCreated || bytes.Contains(created.Body.Bytes(), []byte(`"code"`)) || !bytes.Contains(created.Body.Bytes(), []byte(`"name":"2027级"`)) {
		t.Fatalf("create grade status=%d body=%s", created.Code, created.Body.String())
	}
}

func TestListMasterDataForwardsKindSpecificFilters(t *testing.T) {
	accountsRepository := &accounts{values: map[string]auth.Account{}}
	authService := auth.NewService(accountsRepository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := authService.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	accountsRepository.values[admin.LoginName] = admin
	tokens, _ := authService.Login(context.Background(), "admin", "InitialPassword123")
	repository := &masterRepository{records: map[string]masterdata.Record{}}
	router := NewRouter(authService, false, AdminServices{MasterData: masterdata.NewService(repository)})

	response := performJSON(router, http.MethodGet, "/api/v1/admin/master-data/students?code=S01&name=%E5%BC%A0&enabled=true&facultyId=faculty-1&majorId=major-1&gradeId=grade-1&enrollmentStatus=ENROLLED", nil, tokens.AccessToken)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	query := repository.lastQuery
	if query.Code != "S01" || query.Name != "张" || query.Enabled == nil || !*query.Enabled || query.FacultyID != "faculty-1" || query.MajorID != "major-1" || query.GradeID != "grade-1" || query.EnrollmentStatus != "ENROLLED" {
		t.Fatalf("query = %+v", query)
	}
}

func TestCreateMasterDataReportsDuplicateFacultyName(t *testing.T) {
	tests := []struct {
		name          string
		repositoryErr error
		code          string
		message       string
	}{
		{name: "name", repositoryErr: masterdata.ErrDuplicateName, code: "MASTER_DATA_NAME_EXISTS", message: "该名称已存在"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repository := &accounts{values: map[string]auth.Account{}}
			svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
			admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
			admin.MustChangePassword = false
			repository.values[admin.LoginName] = admin
			tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
			router := NewRouter(svc, false, AdminServices{MasterData: masterdata.NewService(&masterRepository{
				records: map[string]masterdata.Record{}, createErr: test.repositoryErr,
			})})

			response := performJSON(router, http.MethodPost, "/api/v1/admin/master-data/faculties", map[string]any{"name": "计算机科学与工程学院"}, tokens.AccessToken)
			if response.Code != http.StatusConflict || !bytes.Contains(response.Body.Bytes(), []byte(test.code)) || !bytes.Contains(response.Body.Bytes(), []byte(test.message)) {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
		})
	}
}

func TestSuperAdminCanManageDictionaryTypesAndItems(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, _ := svc.BootstrapAdmin(context.Background(), "admin", "系统管理员", "InitialPassword123")
	admin.MustChangePassword = false
	repository.values[admin.LoginName] = admin
	tokens, _ := svc.Login(context.Background(), "admin", "InitialPassword123")
	dictionaryRepo := &dictionaryRepository{}
	router := NewRouter(svc, false, AdminServices{Dictionary: dictionary.NewService(dictionaryRepo)})
	createdType := performJSON(router, http.MethodPost, "/api/v1/admin/dictionaries/types", map[string]any{"code": "GENDER", "name": "性别"}, tokens.AccessToken)
	if createdType.Code != http.StatusCreated {
		t.Fatalf("create type status=%d body=%s", createdType.Code, createdType.Body.String())
	}
	var typeResult dictionary.Type
	_ = json.Unmarshal(createdType.Body.Bytes(), &typeResult)
	createdItem := performJSON(router, http.MethodPost, "/api/v1/admin/dictionaries/types/"+typeResult.ID+"/items", map[string]any{"value": "MALE", "label": "男", "sortOrder": 10}, tokens.AccessToken)
	if createdItem.Code != http.StatusCreated {
		t.Fatalf("create item status=%d body=%s", createdItem.Code, createdItem.Body.String())
	}
}

func TestSystemAdminCanManageMasterDataButMemberCannot(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	createActiveAccount := func(loginName, role string) string {
		_, _ = svc.Register(context.Background(), auth.RegisterInput{StudentNo: loginName, Name: loginName, Password: "Password123", RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1"})
		account := repository.values[loginName]
		account.Status = auth.StatusActive
		account.SystemRole = role
		repository.values[loginName] = account
		tokens, _ := svc.Login(context.Background(), loginName, "Password123")
		return tokens.AccessToken
	}
	adminToken := createActiveAccount("24010002", auth.SystemRoleAdmin)
	memberToken := createActiveAccount("24010003", auth.SystemRoleMember)
	permissionRepo := &permissionRepository{}
	router := NewRouter(svc, false, AdminServices{
		Permission: permission.NewService(permissionRepo),
		MasterData: masterdata.NewService(&masterRepository{records: map[string]masterdata.Record{}}),
	})

	permissionRepo.profile = permission.AccessProfile{SystemRole: permission.SystemRoleAdmin}
	allowed := performJSON(router, http.MethodGet, "/api/v1/admin/master-data/faculties?page=1&pageSize=20", nil, adminToken)
	if allowed.Code != http.StatusOK {
		t.Fatalf("admin status=%d body=%s", allowed.Code, allowed.Body.String())
	}
	permissionRepo.profile = permission.AccessProfile{SystemRole: permission.SystemRoleMember}
	denied := performJSON(router, http.MethodGet, "/api/v1/admin/master-data/faculties?page=1&pageSize=20", nil, memberToken)
	if denied.Code != http.StatusForbidden {
		t.Fatalf("member status=%d body=%s", denied.Code, denied.Body.String())
	}
}

func TestHygieneAreaScopedMasterDataGrantDoesNotOpenGeneralCatalog(t *testing.T) {
	repository := &accounts{values: map[string]auth.Account{}}
	svc := auth.NewService(repository, &sessions{map[string]auth.RefreshSession{}}, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	_, _ = svc.Register(context.Background(), auth.RegisterInput{StudentNo: "area-member", Name: "area-member", Password: "Password123", RequestedDepartmentID: "department-1", RequestedOrgRoleID: "role-1"})
	account := repository.values["area-member"]
	account.Status = auth.StatusActive
	repository.values["area-member"] = account
	tokens, _ := svc.Login(context.Background(), "area-member", "Password123")
	departmentID := "10000000-0000-0000-0000-000000000002"
	typeID := "30000000-0000-0000-0000-000000000003"
	permissionRepo := &permissionRepository{profile: permission.AccessProfile{SystemRole: permission.SystemRoleMember, Rules: []permission.Rule{{Action: permission.ActionMasterDataManage, Effect: permission.EffectAllow, Scope: permission.Scope{DepartmentID: departmentID, InspectionTypeID: typeID}, Enabled: true}}}}
	router := NewRouter(svc, false, AdminServices{Permission: permission.NewService(permissionRepo), MasterData: masterdata.NewService(&masterRepository{records: map[string]masterdata.Record{}})})
	areaURL := "/api/v1/admin/hygiene-areas/classroom-hygiene-areas?departmentId=" + departmentID
	if response := performJSON(router, http.MethodGet, areaURL, nil, tokens.AccessToken); response.Code != http.StatusOK {
		t.Fatalf("scoped area listing status=%d body=%s", response.Code, response.Body.String())
	}
	for _, path := range []string{"/api/v1/admin/master-data/faculties", "/api/v1/admin/hygiene-areas/classroom-hygiene-areas?departmentId=10000000-0000-0000-0000-000000000004", "/api/v1/admin/hygiene-areas/faculties?departmentId=" + departmentID} {
		response := performJSON(router, http.MethodGet, path, nil, tokens.AccessToken)
		if response.Code == http.StatusOK {
			t.Fatalf("scope leaked to %s", path)
		}
	}
	permissionRepo.profile.Rules = append(permissionRepo.profile.Rules, permission.Rule{Action: permission.ActionMasterDataManage, Effect: permission.EffectDeny, Scope: permission.Scope{DepartmentID: departmentID}, Enabled: true})
	if response := performJSON(router, http.MethodGet, areaURL, nil, tokens.AccessToken); response.Code != http.StatusForbidden {
		t.Fatalf("DENY did not override scoped grant: %d %s", response.Code, response.Body.String())
	}
}

func TestReadinessFailsWhenPostgresHealthyAndObjectStorageUnavailable(t *testing.T) {
	router := NewRouter(nil, false, AdminServices{
		PostgresReady:      readinessStub{},
		ObjectStorageReady: readinessStub{err: errors.New("secret endpoint detail")},
	})

	response := performJSON(router, http.MethodGet, "/health/ready", nil, "")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret endpoint") {
		t.Fatalf("readiness leaked dependency detail: %s", response.Body.String())
	}
}

func TestReadinessSucceedsOnlyWhenPostgresAndObjectStorageAreHealthy(t *testing.T) {
	router := NewRouter(nil, false, AdminServices{
		PostgresReady:      readinessStub{},
		ObjectStorageReady: readinessStub{},
	})

	response := performJSON(router, http.MethodGet, "/health/ready", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestLivenessDoesNotDependOnObjectStorage(t *testing.T) {
	router := NewRouter(nil, false, AdminServices{
		PostgresReady:      readinessStub{err: errors.New("database unavailable")},
		ObjectStorageReady: readinessStub{err: errors.New("storage unavailable")},
	})

	response := performJSON(router, http.MethodGet, "/health/live", nil, "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

type httpHandler interface {
	ServeHTTP(http.ResponseWriter, *http.Request)
}

func performJSON(h httpHandler, method, path string, payload any, token string) *httptest.ResponseRecorder {
	var data []byte
	if payload != nil {
		data, _ = json.Marshal(payload)
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}
