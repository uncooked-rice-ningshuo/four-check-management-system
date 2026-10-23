package auth_test

import (
	"context"
	"errors"
	"fmt"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/store"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAccountMutationAtomicPostgres(t *testing.T) {
	url := os.Getenv("ACCOUNT_MUTATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	db, err := store.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	paths, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.sql"))
	if err != nil || len(paths) == 0 {
		t.Fatal("migration files unavailable")
	}
	for _, path := range paths {
		content, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Pool.Exec(ctx, string(content)); e != nil {
			t.Fatalf("migration %s: %v", filepath.Base(path), e)
		}
	}
	svc := auth.NewService(db, db, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	admin, err := svc.BootstrapAdmin(ctx, "atomic-admin", "Mutation fixture", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	for i, kind := range []string{"change-password", "reset-password", "disable", "update-account"} {
		t.Run(kind, func(t *testing.T) {
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("atomic-person-%d", i), Name: "Mutation user", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			tokens, e := svc.Login(ctx, user.LoginName, "TemporaryPassword123")
			if e != nil {
				t.Fatal(e)
			}
			before, e := db.FindByID(ctx, user.ID)
			if e != nil {
				t.Fatal(e)
			}
			var auditBefore int
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&auditBefore); e != nil {
				t.Fatal(e)
			}
			mutate := func() error {
				switch kind {
				case "change-password":
					return svc.ChangePassword(ctx, user, "TemporaryPassword123", "ChangedPassword456")
				case "reset-password":
					return svc.AdminResetPassword(ctx, admin, user.ID, "ChangedPassword456")
				case "disable":
					_, e := svc.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusDisabled)
					return e
				default:
					_, e := svc.AdminUpdateUser(ctx, admin, user.ID, auth.AdminUpdateUserInput{StudentNo: user.StudentNo, Name: "Updated user", SystemRole: auth.SystemRoleAdmin})
					return e
				}
			}
			if _, e = db.Pool.Exec(ctx, `CREATE FUNCTION reject_account_revocation() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated revocation failure'; END $$; CREATE TRIGGER reject_account_revocation BEFORE UPDATE ON refresh_sessions FOR EACH ROW EXECUTE FUNCTION reject_account_revocation()`); e != nil {
				t.Fatal(e)
			}
			t.Cleanup(func() {
				if _, cleanupErr := db.Pool.Exec(ctx, `DROP TRIGGER IF EXISTS reject_account_revocation ON refresh_sessions; DROP FUNCTION IF EXISTS reject_account_revocation()`); cleanupErr != nil {
					t.Errorf("remove injected revocation fault: %v", cleanupErr)
				}
			})
			if e = mutate(); e == nil {
				t.Fatal("revocation fault unexpectedly succeeded")
			}
			after, e := db.FindByID(ctx, user.ID)
			if e != nil {
				t.Fatal(e)
			}
			if after.PasswordHash != before.PasswordHash || after.SessionVersion != before.SessionVersion || after.Status != before.Status || after.Name != before.Name || after.SystemRole != before.SystemRole {
				t.Fatal("revocation failure left a partially committed account change")
			}
			var audits, active int
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&audits); e != nil || audits != auditBefore {
				t.Fatal("failed mutation committed audit", audits, e)
			}
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE account_id=$1 AND revoked_at IS NULL`, user.ID).Scan(&active); e != nil || active != 1 {
				t.Fatal("failed mutation changed sessions", active, e)
			}
			if _, e = db.Pool.Exec(ctx, `DROP TRIGGER reject_account_revocation ON refresh_sessions; DROP FUNCTION reject_account_revocation()`); e != nil {
				t.Fatal(e)
			}
			if e = mutate(); e != nil {
				t.Fatal("retry failed", e)
			}
			if _, e = svc.Refresh(ctx, tokens.RefreshToken); !errors.Is(e, auth.ErrInvalidRefreshToken) {
				t.Fatal("old refresh survived account mutation", e)
			}
			if _, e = svc.AuthenticateAccessToken(ctx, tokens.AccessToken); !errors.Is(e, auth.ErrInvalidAccessToken) {
				t.Fatal("old access token survived account mutation", e)
			}
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&audits); e != nil || audits != auditBefore+1 {
				t.Fatal("retry did not commit exactly one audit", audits, e)
			}
		})
	}
	for i, kind := range []string{"password", "status", "role"} {
		t.Run("login read before "+kind+" mutation", func(t *testing.T) {
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("login-race-%d", i), Name: "Login race", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			repository := &pausedLoginSave{Postgres: db, reached: make(chan struct{}), resume: make(chan struct{})}
			loginService := auth.NewService(db, repository, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
			result := make(chan error, 1)
			go func() {
				_, loginErr := loginService.Login(ctx, user.LoginName, "TemporaryPassword123")
				result <- loginErr
			}()
			select {
			case <-repository.reached:
			case <-ctx.Done():
				t.Fatal("login did not reach session persistence")
			}
			switch kind {
			case "password":
				e = svc.AdminResetPassword(ctx, admin, user.ID, "ChangedPassword456")
			case "status":
				_, e = svc.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusDisabled)
			case "role":
				_, e = svc.AdminUpdateUser(ctx, admin, user.ID, auth.AdminUpdateUserInput{StudentNo: user.StudentNo, Name: user.Name, SystemRole: auth.SystemRoleAdmin})
			}
			close(repository.resume)
			if e != nil {
				t.Fatal(e)
			}
			if e = <-result; !errors.Is(e, auth.ErrInvalidCredentials) {
				t.Fatal("stale login was not rejected", e)
			}
			var sessions int
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE account_id=$1`, user.ID).Scan(&sessions); e != nil || sessions != 0 {
				t.Fatal("stale login left a refresh session", sessions, e)
			}
		})
	}
	for i, kind := range []string{"self-change", "admin-reset", "disable"} {
		t.Run("stale password write after "+kind, func(t *testing.T) {
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("password-race-%d", i), Name: "Password race", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			repository := &pausedPasswordWrite{Postgres: db, reached: make(chan struct{}), resume: make(chan struct{})}
			staleService := auth.NewService(repository, db, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
			result := make(chan error, 1)
			go func() {
				if kind == "admin-reset" {
					result <- staleService.AdminResetPassword(ctx, admin, user.ID, "StalePassword789")
				} else {
					result <- staleService.ChangePassword(ctx, user, "TemporaryPassword123", "StalePassword789")
				}
			}()
			select {
			case <-repository.reached:
			case <-ctx.Done():
				t.Fatal("password write did not reach repository")
			}
			if kind == "disable" {
				_, e = svc.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusDisabled)
			} else {
				e = svc.ChangePassword(ctx, user, "TemporaryPassword123", "WinningPassword456")
			}
			if e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			winner, e := db.FindByID(ctx, user.ID)
			if e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			var audits int
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&audits); e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			close(repository.resume)
			if e = <-result; !errors.Is(e, auth.ErrVersionConflict) {
				t.Fatal("stale password write did not return a version conflict", e)
			}
			after, e := db.FindByID(ctx, user.ID)
			if e != nil {
				t.Fatal(e)
			}
			if after.PasswordHash != winner.PasswordHash || after.SessionVersion != winner.SessionVersion || after.Status != winner.Status || after.MustChangePassword != winner.MustChangePassword {
				t.Fatal("stale password write overwrote committed state")
			}
			var finalAudits int
			if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&finalAudits); e != nil || finalAudits != audits {
				t.Fatal("stale password write committed audit", e)
			}
		})
	}
	for i, kind := range []string{"disabled", "version-changed"} {
		t.Run("old authenticated actor before password read "+kind, func(t *testing.T) {
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("actor-race-%d", i), Name: "Actor race", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			if kind == "disabled" {
				_, e = svc.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusDisabled)
			} else {
				_, e = svc.AdminUpdateUser(ctx, admin, user.ID, auth.AdminUpdateUserInput{StudentNo: user.StudentNo, Name: user.Name, SystemRole: auth.SystemRoleAdmin})
			}
			if e != nil {
				t.Fatal(e)
			}
			before, e := db.FindByID(ctx, user.ID)
			if e != nil {
				t.Fatal(e)
			}
			if e = svc.ChangePassword(ctx, user, "TemporaryPassword123", "StalePassword789"); !errors.Is(e, auth.ErrInvalidAccessToken) {
				t.Fatal("old actor accepted after account changed", e)
			}
			after, e := db.FindByID(ctx, user.ID)
			if e != nil || after.PasswordHash != before.PasswordHash || after.SessionVersion != before.SessionVersion {
				t.Fatal("old actor changed password", e)
			}
			if kind == "disabled" {
				if e = svc.AdminResetPassword(ctx, admin, user.ID, "AdminResetPassword456"); e != nil {
					t.Fatal("administrator cannot reset disabled target", e)
				}
			}
		})
	}
	for i, scenario := range []struct{ operation, winner string }{{"update", "role"}, {"update", "password"}, {"status", "disable"}, {"status", "role"}} {
		t.Run("stale account "+scenario.operation+" after "+scenario.winner, func(t *testing.T) {
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("account-write-race-%d", i), Name: "Original name", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			repository := &pausedAccountWrite{Postgres: db, reached: make(chan struct{}), resume: make(chan struct{})}
			staleService := auth.NewService(repository, db, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
			result := make(chan error, 1)
			go func() {
				var writeErr error
				if scenario.operation == "update" {
					_, writeErr = staleService.AdminUpdateUser(ctx, admin, user.ID, auth.AdminUpdateUserInput{StudentNo: user.StudentNo, Name: "Stale name", SystemRole: auth.SystemRoleMember})
				} else {
					_, writeErr = staleService.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusActive)
				}
				result <- writeErr
			}()
			select {
			case <-repository.reached:
			case <-ctx.Done():
				t.Fatal("account write did not reach repository")
			}
			password := "TemporaryPassword123"
			switch scenario.winner {
			case "role":
				_, e = svc.AdminUpdateUser(ctx, admin, user.ID, auth.AdminUpdateUserInput{StudentNo: user.StudentNo, Name: "Winning name", SystemRole: auth.SystemRoleAdmin})
			case "password":
				password = "WinningPassword456"
				e = svc.AdminResetPassword(ctx, admin, user.ID, password)
			case "disable":
				_, e = svc.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusDisabled)
			}
			if e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			winner, e := db.FindByID(ctx, user.ID)
			if e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			accessToken := ""
			if winner.Status == auth.StatusActive {
				tokens, e := svc.Login(ctx, winner.LoginName, password)
				if e != nil {
					close(repository.resume)
					t.Fatal(e)
				}
				accessToken = tokens.AccessToken
			}
			var audits int
			if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&audits); e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			close(repository.resume)
			if e := <-result; !errors.Is(e, auth.ErrVersionConflict) {
				t.Fatal("stale account write accepted", e)
			}
			after, e := db.FindByID(ctx, user.ID)
			if e != nil {
				t.Fatal(e)
			}
			if after.SessionVersion != winner.SessionVersion || after.Status != winner.Status || after.SystemRole != winner.SystemRole || after.Name != winner.Name || after.PasswordHash != winner.PasswordHash {
				t.Fatal("stale write changed committed account")
			}
			var finalAudits int
			if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&finalAudits); e != nil || finalAudits != audits {
				t.Fatal("stale write committed audit", e)
			}
			if accessToken != "" {
				if _, e := svc.AuthenticateAccessToken(ctx, accessToken); e != nil {
					t.Fatal("stale write invalidated new session", e)
				}
			}
		})
	}

	for i, winnerKind := range []string{"assignment", "disable"} {
		t.Run("stale organization assignment after "+winnerKind, func(t *testing.T) {
			var firstDepartment, secondDepartment, role string
			if e := db.Pool.QueryRow(ctx, `SELECT id FROM departments WHERE enabled ORDER BY code LIMIT 1`).Scan(&firstDepartment); e != nil {
				t.Fatal(e)
			}
			if e := db.Pool.QueryRow(ctx, `SELECT id FROM departments WHERE enabled ORDER BY code OFFSET 1 LIMIT 1`).Scan(&secondDepartment); e != nil {
				t.Fatal(e)
			}
			if e := db.Pool.QueryRow(ctx, `SELECT id FROM organization_roles WHERE enabled ORDER BY code LIMIT 1`).Scan(&role); e != nil {
				t.Fatal(e)
			}
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("assignment-race-%d", i), Name: "Assignment original", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			repository := &pausedAssignmentWrite{Postgres: db, reached: make(chan struct{}), resume: make(chan struct{})}
			staleService := organization.NewService(repository)
			result := make(chan error, 1)
			go func() {
				result <- staleService.AssignUserRole(ctx, organization.UserAssignment{AccountID: user.ID, Name: "Forged name", Status: "DISABLED", SystemRole: "SUPER_ADMIN", IsSuperAdmin: true, DepartmentID: firstDepartment, OrganizationRoleID: role}, audit.Metadata{ActorID: admin.ID})
			}()
			select {
			case <-repository.reached:
			case <-ctx.Done():
				t.Fatal("assignment did not reach repository")
			}
			if winnerKind == "assignment" {
				e = organization.NewService(db).AssignUserRole(ctx, organization.UserAssignment{AccountID: user.ID, Name: "Forged winner", DepartmentID: secondDepartment, OrganizationRoleID: role}, audit.Metadata{ActorID: admin.ID})
			} else {
				_, e = svc.AdminSetUserStatus(ctx, admin, user.ID, auth.StatusDisabled)
			}
			if e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			winner, e := db.FindByID(ctx, user.ID)
			if e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			var audits int
			if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&audits); e != nil {
				close(repository.resume)
				t.Fatal(e)
			}
			close(repository.resume)
			if e := <-result; !errors.Is(e, auth.ErrVersionConflict) {
				t.Fatal("stale assignment accepted", e)
			}
			after, e := db.FindByID(ctx, user.ID)
			if e != nil || after.SessionVersion != winner.SessionVersion || after.PrimaryDepartmentID != winner.PrimaryDepartmentID || after.Status != winner.Status {
				t.Fatal("stale assignment changed account", e)
			}
			var finalAudits int
			if e := db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs`).Scan(&finalAudits); e != nil || finalAudits != audits {
				t.Fatal("stale assignment audited", e)
			}
			if winnerKind == "assignment" {
				var name, status, systemRole string
				if e := db.Pool.QueryRow(ctx, `SELECT after_json->>'name',after_json->>'status',after_json->>'systemRole' FROM audit_logs WHERE resource_id=$1 AND action='ORGANIZATION_UPDATED' ORDER BY created_at DESC LIMIT 1`, user.ID).Scan(&name, &status, &systemRole); e != nil || name != user.Name || status != user.Status || systemRole != user.SystemRole {
					t.Fatal("assignment audit trusted client account fields", e)
				}
			}
		})
	}

	for i, kind := range []string{"disabled department", "disabled role", "missing department", "missing role", "malformed department", "malformed role"} {
		t.Run("assignment rejects "+kind, func(t *testing.T) {
			var department, role string
			if e := db.Pool.QueryRow(ctx, `SELECT id FROM departments WHERE enabled ORDER BY code LIMIT 1`).Scan(&department); e != nil {
				t.Fatal(e)
			}
			if e := db.Pool.QueryRow(ctx, `SELECT id FROM organization_roles WHERE enabled ORDER BY code LIMIT 1`).Scan(&role); e != nil {
				t.Fatal(e)
			}
			user, e := svc.AdminCreateUser(ctx, admin, auth.AdminCreateUserInput{StudentNo: fmt.Sprintf("assignment-reference-%d", i), Name: "Reference fixture", TemporaryPassword: "TemporaryPassword123", SystemRole: auth.SystemRoleMember})
			if e != nil {
				t.Fatal(e)
			}
			tokens, e := svc.Login(ctx, user.LoginName, "TemporaryPassword123")
			if e != nil {
				t.Fatal(e)
			}
			before, e := db.FindByID(ctx, user.ID)
			if e != nil {
				t.Fatal(e)
			}
			if kind == "missing department" {
				department = "00000000-0000-4000-8000-000000000001"
			}
			if kind == "missing role" {
				role = "00000000-0000-4000-8000-000000000002"
			}
			if kind == "malformed department" {
				department = "invalid-id"
			}
			if kind == "malformed role" {
				role = "invalid-id"
			}
			repository := &pausedAssignmentWrite{Postgres: db, reached: make(chan struct{}), resume: make(chan struct{})}
			result := make(chan error, 1)
			go func() {
				result <- organization.NewService(repository).AssignUserRole(ctx, organization.UserAssignment{AccountID: user.ID, DepartmentID: department, OrganizationRoleID: role}, audit.Metadata{ActorID: admin.ID})
			}()
			select {
			case <-repository.reached:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			if kind == "disabled department" {
				_, e = db.Pool.Exec(ctx, `UPDATE departments SET enabled=false WHERE id=$1`, department)
				defer func() {
					if _, err := db.Pool.Exec(ctx, `UPDATE departments SET enabled=true WHERE id=$1`, department); err != nil {
						t.Error(err)
					}
				}()
			}
			if kind == "disabled role" {
				_, e = db.Pool.Exec(ctx, `UPDATE organization_roles SET enabled=false WHERE id=$1`, role)
				defer func() {
					if _, err := db.Pool.Exec(ctx, `UPDATE organization_roles SET enabled=true WHERE id=$1`, role); err != nil {
						t.Error(err)
					}
				}()
			}
			close(repository.resume)
			if e != nil {
				t.Fatal(e)
			}
			if e := <-result; !errors.Is(e, organization.ErrInvalidInput) {
				t.Fatalf("invalid reference accepted or misclassified: %v", e)
			}
			after, e := db.FindByID(ctx, user.ID)
			if e != nil || after.SessionVersion != before.SessionVersion || after.PrimaryDepartmentID != before.PrimaryDepartmentID {
				t.Fatal("invalid reference changed account", e)
			}
			var writes int
			if e := db.Pool.QueryRow(ctx, `SELECT (SELECT count(*) FROM user_org_roles WHERE account_id=$1::uuid)+(SELECT count(*) FROM audit_logs WHERE resource_id=$1::text AND action='ORGANIZATION_UPDATED')`, user.ID).Scan(&writes); e != nil || writes != 0 {
				t.Fatal("invalid reference committed assignment/audit", e)
			}
			if _, e := svc.AuthenticateAccessToken(ctx, tokens.AccessToken); e != nil {
				t.Fatal("invalid reference revoked session", e)
			}
		})
	}

}

type pausedPasswordWrite struct {
	*store.Postgres
	reached, resume chan struct{}
}

func (p *pausedPasswordWrite) UpdatePassword(ctx context.Context, id, hash string, mustChange bool, version int, mutation audit.Mutation) error {
	close(p.reached)
	select {
	case <-p.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return p.Postgres.UpdatePassword(ctx, id, hash, mustChange, version, mutation)
}

type pausedLoginSave struct {
	*store.Postgres
	reached, resume chan struct{}
}

func (p *pausedLoginSave) Save(ctx context.Context, session auth.RefreshSession, version int) error {
	close(p.reached)
	select {
	case <-p.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return p.Postgres.Save(ctx, session, version)
}

type pausedAccountWrite struct {
	*store.Postgres
	reached, resume chan struct{}
}

func (p *pausedAccountWrite) wait(ctx context.Context) error {
	close(p.reached)
	select {
	case <-p.resume:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
func (p *pausedAccountWrite) UpdateAccount(ctx context.Context, account auth.Account, mutation audit.Mutation) (auth.Account, error) {
	if e := p.wait(ctx); e != nil {
		return auth.Account{}, e
	}
	return p.Postgres.UpdateAccount(ctx, account, mutation)
}
func (p *pausedAccountWrite) SetStatus(ctx context.Context, id, status string, expectedVersion int, mutation audit.Mutation) (auth.Account, error) {
	if e := p.wait(ctx); e != nil {
		return auth.Account{}, e
	}
	return p.Postgres.SetStatus(ctx, id, status, expectedVersion, mutation)
}

type pausedAssignmentWrite struct {
	*store.Postgres
	reached, resume chan struct{}
}

func (p *pausedAssignmentWrite) AssignUserRole(ctx context.Context, value organization.UserAssignment, mutation audit.Mutation) error {
	close(p.reached)
	select {
	case <-p.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return p.Postgres.AssignUserRole(ctx, value, mutation)
}
