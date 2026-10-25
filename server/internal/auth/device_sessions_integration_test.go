package auth_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/store"
)

func TestDeviceSessionLimitPostgres(t *testing.T) {
	url := os.Getenv("DEVICE_SESSION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
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
		body, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Pool.Exec(ctx, string(body)); e != nil {
			t.Fatalf("migration %s: %v", filepath.Base(path), e)
		}
	}
	svc := auth.NewService(db, db, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	account, err := svc.BootstrapAdmin(ctx, "device-limit", "Device fixture", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	login := func() auth.Tokens {
		t.Helper()
		value, e := svc.Login(ctx, account.LoginName, "InitialPassword123")
		if e != nil {
			t.Fatal(e)
		}
		return value
	}
	first, second, third := login(), login(), login()
	if _, err = svc.Refresh(ctx, first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if err = svc.Logout(ctx, first.RefreshToken); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AuthenticateAccessToken(ctx, first.AccessToken); err != nil {
		t.Fatalf("consumed refresh token revoked current device: %v", err)
	}
	if _, err = db.Pool.Exec(ctx, `CREATE FUNCTION reject_device_login_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated device login failure'; END $$;
		CREATE TRIGGER reject_device_login_insert BEFORE INSERT ON refresh_sessions FOR EACH ROW EXECUTE FUNCTION reject_device_login_insert()`); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Login(ctx, account.LoginName, "InitialPassword123"); err == nil {
		t.Fatal("injected login failure unexpectedly succeeded")
	}
	if _, err = db.Pool.Exec(ctx, `DROP TRIGGER reject_device_login_insert ON refresh_sessions; DROP FUNCTION reject_device_login_insert()`); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.AuthenticateAccessToken(ctx, second.AccessToken); err != nil {
		t.Fatalf("failed fourth login evicted an old device: %v", err)
	}
	fourth := login()
	if _, err = svc.AuthenticateAccessToken(ctx, second.AccessToken); !errors.Is(err, auth.ErrSessionEvicted) {
		t.Fatalf("old access token: %v", err)
	}
	if _, err = svc.Refresh(ctx, second.RefreshToken); !errors.Is(err, auth.ErrSessionEvicted) {
		t.Fatalf("old refresh token: %v", err)
	}
	for _, token := range []auth.Tokens{first, third, fourth} {
		if _, err = svc.AuthenticateAccessToken(ctx, token.AccessToken); err != nil {
			t.Fatalf("retained session: %v", err)
		}
	}
	var active int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM login_sessions WHERE account_id=$1 AND revoked_at IS NULL AND expires_at>now()`, account.ID).Scan(&active); err != nil || active != 3 {
		t.Fatalf("active sessions=%d err=%v", active, err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, e := svc.Login(ctx, account.LoginName, "InitialPassword123")
			results <- e
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	for e := range results {
		if e != nil {
			t.Fatalf("concurrent login: %v", e)
		}
	}
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM login_sessions WHERE account_id=$1 AND revoked_at IS NULL AND expires_at>now()`, account.ID).Scan(&active); err != nil || active != 3 {
		t.Fatalf("concurrent active sessions=%d err=%v", active, err)
	}
}

func TestDeviceSessionMigrationPreservesNewestThreePostgres(t *testing.T) {
	url := os.Getenv("DEVICE_SESSION_MIGRATION_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable PostgreSQL database")
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
	var latest string
	for _, path := range paths {
		if filepath.Base(path) == "028_login_device_sessions.sql" {
			latest = path
			continue
		}
		body, e := os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
		if _, e = db.Pool.Exec(ctx, string(body)); e != nil {
			t.Fatalf("migration %s: %v", filepath.Base(path), e)
		}
	}
	if latest == "" {
		t.Fatal("device session migration missing")
	}
	svc := auth.NewService(db, db, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
	account, err := svc.BootstrapAdmin(ctx, "legacy-device-limit", "Legacy fixture", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 4; i++ {
		_, err = db.Pool.Exec(ctx, `INSERT INTO refresh_sessions(id,account_id,token_hash,created_at,expires_at)
			VALUES(gen_random_uuid(),$1,$2,now()-($3::integer*interval '1 minute'),now()+interval '1 hour')`, account.ID, fmt.Sprintf("%064x", i+1), 4-i)
		if err != nil {
			t.Fatal(err)
		}
	}
	body, err := os.ReadFile(latest)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Pool.Exec(ctx, string(body)); err != nil {
		t.Fatal(err)
	}
	var active, evicted, unmapped int
	err = db.Pool.QueryRow(ctx, `SELECT count(*) FILTER (WHERE revoked_at IS NULL), count(*) FILTER (WHERE revoked_reason='EVICTED')
		FROM login_sessions WHERE account_id=$1`, account.ID).Scan(&active, &evicted)
	if err == nil {
		err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE account_id=$1 AND login_session_id IS NULL`, account.ID).Scan(&unmapped)
	}
	if err != nil || active != 3 || evicted != 1 || unmapped != 0 {
		t.Fatalf("active=%d evicted=%d unmapped=%d err=%v", active, evicted, unmapped, err)
	}
	var auditCount int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE action='SESSION_EVICTED' AND actor_id=$1`, account.ID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("audit=%d err=%v", auditCount, err)
	}
}
