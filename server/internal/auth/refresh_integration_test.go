package auth_test

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/store"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRefreshAtomicPostgres(t *testing.T) {
	url := os.Getenv("REFRESH_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("requires an empty disposable database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
	account, err := svc.BootstrapAdmin(ctx, "atomic-refresh", "Rotation fixture", "InitialPassword123")
	if err != nil {
		t.Fatal(err)
	}
	login := func() auth.Tokens {
		t.Helper()
		tokens, e := svc.Login(ctx, "atomic-refresh", "InitialPassword123")
		if e != nil {
			t.Fatal(e)
		}
		return tokens
	}
	first := login()
	if _, err = db.Pool.Exec(ctx, `CREATE FUNCTION reject_refresh_insert() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'isolated refresh save failure'; END $$; CREATE TRIGGER reject_refresh_insert BEFORE INSERT ON refresh_sessions FOR EACH ROW EXECUTE FUNCTION reject_refresh_insert()`); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Refresh(ctx, first.RefreshToken); err == nil || errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatal("save failure was not propagated")
	}
	var active int
	if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE account_id=$1 AND revoked_at IS NULL`, account.ID).Scan(&active); err != nil || active != 1 {
		t.Fatalf("failed rotation changed old session: active=%d err=%v", active, err)
	}
	if _, err = db.Pool.Exec(ctx, `DROP TRIGGER reject_refresh_insert ON refresh_sessions; DROP FUNCTION reject_refresh_insert()`); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Refresh(ctx, first.RefreshToken); err != nil {
		t.Fatal("retry failed", err)
	}
	if _, err = svc.Refresh(ctx, first.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
		t.Fatal("replay accepted")
	}

	t.Run("two simultaneous refreshes", func(t *testing.T) {
		original := login()
		start := make(chan struct{})
		results := make(chan error, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); <-start; _, e := svc.Refresh(ctx, original.RefreshToken); results <- e }()
		}
		close(start)
		wg.Wait()
		close(results)
		success, invalid := 0, 0
		for e := range results {
			if e == nil {
				success++
			} else if errors.Is(e, auth.ErrInvalidRefreshToken) {
				invalid++
			} else {
				t.Fatal(e)
			}
		}
		if success != 1 || invalid != 1 {
			t.Fatalf("success=%d invalid=%d", success, invalid)
		}
	})

	// Hold the account lock while rotation starts, exactly as password and
	// permission mutations do. A stale pre-read version cannot survive commit.
	t.Run("account invalidation races with rotation", func(t *testing.T) {
		login()
		var hash string
		if err = db.Pool.QueryRow(ctx, `SELECT token_hash FROM refresh_sessions WHERE account_id=$1 AND revoked_at IS NULL ORDER BY created_at DESC LIMIT 1`, account.ID).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		tx, e := db.Pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		defer tx.Rollback(ctx)
		if _, e = tx.Exec(ctx, `UPDATE accounts SET session_version=session_version+1 WHERE id=$1`, account.ID); e != nil {
			t.Fatal(e)
		}
		replacement := auth.RefreshSession{ID: uuid.NewString(), AccountID: account.ID, TokenHash: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ExpiresAt: time.Now().Add(time.Hour)}
		result := make(chan error, 1)
		go func() { result <- db.Rotate(ctx, hash, time.Now(), replacement, account.SessionVersion) }()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var waiting bool
			if e = db.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE 'SELECT id FROM accounts WHERE id=%')`).Scan(&waiting); e != nil {
				t.Fatal(e)
			}
			if waiting {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("rotation did not reach account lock")
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, e = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=now() WHERE account_id=$1 AND revoked_at IS NULL`, account.ID); e != nil {
			t.Fatal(e)
		}
		if e = tx.Commit(ctx); e != nil {
			t.Fatal(e)
		}
		if e = <-result; !errors.Is(e, auth.ErrInvalidRefreshToken) {
			t.Fatal("stale account rotation accepted", e)
		}
		if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE account_id=$1 AND revoked_at IS NULL`, account.ID).Scan(&active); e != nil || active != 0 {
			t.Fatal("invalidation left active sessions", active, e)
		}
	})

	t.Run("expired revoked disabled and query failures", func(t *testing.T) {
		for _, statement := range []string{
			`UPDATE refresh_sessions SET expires_at=now()-interval '1 second' WHERE account_id=$1`,
			`UPDATE refresh_sessions SET revoked_at=now() WHERE account_id=$1`,
			`UPDATE accounts SET status='DISABLED' WHERE id=$1`,
		} {
			tokens := login()
			if _, err = db.Pool.Exec(ctx, statement, account.ID); err != nil {
				t.Fatal(err)
			}
			var before, after int
			if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions`).Scan(&before); err != nil {
				t.Fatal(err)
			}
			if _, err = svc.Refresh(ctx, tokens.RefreshToken); !errors.Is(err, auth.ErrInvalidRefreshToken) {
				t.Fatal("invalid session accepted", err)
			}
			if err = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions`).Scan(&after); err != nil || before != after {
				t.Fatal("invalid session created replacement")
			}
			if _, err = db.Pool.Exec(ctx, `UPDATE accounts SET status='ACTIVE' WHERE id=$1`, account.ID); err != nil {
				t.Fatal(err)
			}
		}
		cancelled, stop := context.WithCancel(ctx)
		stop()
		if _, err = db.FindRefreshSession(cancelled, "unknown", time.Now()); err == nil || errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatal("query failure classified as invalid credentials", err)
		}
	})

	t.Run("real password change invalidates an in-flight refresh", func(t *testing.T) {
		tokens := login()
		repository := &pausedRotation{Postgres: db, reached: make(chan struct{}), resume: make(chan struct{})}
		rotatingService := auth.NewService(db, repository, []byte("01234567890123456789012345678901"), time.Minute, time.Hour)
		result := make(chan error, 1)
		go func() { _, e := rotatingService.Refresh(ctx, tokens.RefreshToken); result <- e }()
		select {
		case <-repository.reached:
		case <-ctx.Done():
			t.Fatal("rotation did not reach persistence")
		}
		currentActor, e := svc.AuthenticateAccessToken(ctx, tokens.AccessToken)
		if e != nil {
			close(repository.resume)
			t.Fatal(e)
		}
		e = svc.ChangePassword(ctx, currentActor, "InitialPassword123", "ChangedPassword456")
		close(repository.resume)
		if e != nil {
			t.Fatal(e)
		}
		if e = <-result; !errors.Is(e, auth.ErrInvalidRefreshToken) {
			t.Fatal("password change failed to reject stale refresh", e)
		}
		if e = db.Pool.QueryRow(ctx, `SELECT count(*) FROM refresh_sessions WHERE account_id=$1 AND revoked_at IS NULL`, account.ID).Scan(&active); e != nil || active != 0 {
			t.Fatal("password change left active sessions", active, e)
		}
		if _, e = svc.AuthenticateAccessToken(ctx, tokens.AccessToken); !errors.Is(e, auth.ErrInvalidAccessToken) {
			t.Fatal("old access token survived password change", e)
		}
	})
}

type pausedRotation struct {
	*store.Postgres
	reached, resume chan struct{}
}

func (p *pausedRotation) Rotate(ctx context.Context, hash string, now time.Time, replacement auth.RefreshSession, version int) error {
	close(p.reached)
	select {
	case <-p.resume:
	case <-ctx.Done():
		return ctx.Err()
	}
	return p.Postgres.Rotate(ctx, hash, now, replacement, version)
}
