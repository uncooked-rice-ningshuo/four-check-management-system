package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
	"four-check-management-system/server/internal/importjob"
	"four-check-management-system/server/internal/inspection"
	"four-check-management-system/server/internal/masterdata"
	"four-check-management-system/server/internal/organization"
	"four-check-management-system/server/internal/permission"
	"four-check-management-system/server/internal/storage"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres 使用同一连接池实现认证领域所需的账号与会话仓储接口。
type Postgres struct{ Pool *pgxpool.Pool }

// Open 创建并验证 PostgreSQL 连接池，连接不可用时阻止 API 带病启动。
func Open(ctx context.Context, url string) (*Postgres, error) {
	p, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := p.Ping(ctx); err != nil {
		p.Close()
		return nil, err
	}
	return &Postgres{p}, nil
}

// Close 释放连接池中的全部 PostgreSQL 连接。
func (p *Postgres) Close() { p.Pool.Close() }

// Ready checks the schema used by the running application as well as connectivity.
// A reachable database with pending migrations must not be advertised as ready.
func (p *Postgres) Ready(ctx context.Context) error {
	var ready bool
	err := p.Pool.QueryRow(ctx, `SELECT
		to_regclass('public.teaching_buildings') IS NOT NULL
		AND to_regclass('public.morning_exercise_details') IS NOT NULL
		AND to_regclass('public.late_return_details') IS NOT NULL
		AND to_regclass('public.dorm_hygiene_details') IS NOT NULL
		AND to_regclass('public.classroom_hygiene_details') IS NOT NULL
		AND to_regclass('public.public_dormitory_hygiene_details') IS NOT NULL
		AND to_regclass('public.department_submissions') IS NOT NULL
		AND to_regclass('public.department_submission_revisions') IS NOT NULL
		AND to_regclass('public.master_data_import_batches') IS NOT NULL
		AND to_regclass('public.academic_week_import_batches') IS NOT NULL
		AND EXISTS (SELECT 1 FROM pg_trigger WHERE tgrelid=to_regclass('public.department_submission_revisions')
			AND tgname='department_submission_revisions_append_only' AND NOT tgisinternal)
		AND EXISTS (SELECT 1 FROM system_settings WHERE key='work_week_override')
 AND EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='files' AND column_name='object_removed_at')`).Scan(&ready)
	if err != nil {
		return fmt.Errorf("database schema readiness: %w", err)
	}
	if !ready {
		return errors.New("database schema incomplete: apply pending migrations")
	}
	return nil
}

// withAudit 在同一 PostgreSQL 事务内执行一次业务变更并追加审计记录。
// action 和 before/after 值由应用服务决定；持久化层只负责脱敏、原子性与追加写。
func (p *Postgres) withAudit(ctx context.Context, metadata audit.Metadata, entry audit.Entry, mutation func(pgx.Tx) error) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := mutation(tx); err != nil {
		return err
	}
	if err := insertAuditTx(ctx, tx, metadata, entry); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func insertAuditTx(ctx context.Context, tx pgx.Tx, metadata audit.Metadata, entry audit.Entry) error {
	metadata, entry, err := audit.Sanitize(metadata, entry)
	if err != nil {
		return err
	}
	if entry.ID == "" {
		entry.ID = uuid.NewString()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	beforeJSON, err := auditJSON(entry.Before)
	if err != nil {
		return fmt.Errorf("encode audit before value: %w", err)
	}
	afterJSON, err := auditJSON(entry.After)
	if err != nil {
		return fmt.Errorf("encode audit after value: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(
		id,actor_id,action,resource_type,resource_id,before_json,after_json,reason,request_id,ip,user_agent,created_at
	) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::inet,$11,$12)`,
		entry.ID, metadata.ActorID, string(entry.Action), entry.ResourceType, entry.ResourceID,
		beforeJSON, afterJSON, metadata.Reason, metadata.RequestID, metadata.IP, metadata.UserAgent, entry.CreatedAt)
	return err
}

func auditJSON(value any) (any, error) {
	if value == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return encoded, nil
}

// CreateFile inserts the initial PENDING metadata before any object upload starts.
func (p *Postgres) CreateFile(ctx context.Context, file storage.File) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO files(
		id,storage_provider,bucket,object_key,original_name,declared_content_type,detected_content_type,
		size_bytes,sha256,status,retention_type,created_by,failure_code,failure_message,created_at,updated_at
	) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NULLIF($12,'')::uuid,$13,$14,$15,$16)`,
		file.ID, file.StorageProvider, file.Bucket, file.ObjectKey, file.OriginalName, file.DeclaredContentType,
		file.DetectedContentType, file.SizeBytes, file.SHA256, file.Status, file.RetentionType, file.CreatedBy,
		file.FailureCode, file.FailureMessage, file.CreatedAt, file.UpdatedAt)
	return err
}

// FindFile returns file metadata without exposing object contents or credentials.
func (p *Postgres) FindFile(ctx context.Context, id string) (storage.File, error) {
	return scanFile(p.Pool.QueryRow(ctx, fileSelect+` WHERE id=$1`, id))
}

// MarkFileAvailable atomically enforces PENDING -> AVAILABLE and appends its success audit.
func (p *Postgres) MarkFileAvailable(ctx context.Context, file storage.File, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE files SET detected_content_type=$2,size_bytes=$3,sha256=$4,status='AVAILABLE',
			failure_code='',failure_message='',updated_at=now() WHERE id=$1 AND status='PENDING'`,
			file.ID, file.DetectedContentType, file.SizeBytes, file.SHA256)
		if err == nil && tag.RowsAffected() == 0 {
			return storage.ErrFileUnavailable
		}
		return err
	})
}

// MarkFileFailed atomically records a stable failure for a PENDING object and appends its audit.
func (p *Postgres) MarkFileFailed(ctx context.Context, file storage.File, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE files SET detected_content_type=$2,sha256=$3,status='FAILED',
			failure_code=$4,failure_message=$5,updated_at=now() WHERE id=$1 AND status='PENDING'`,
			file.ID, file.DetectedContentType, file.SHA256, file.FailureCode, file.FailureMessage)
		if err == nil && tag.RowsAffected() == 0 {
			return storage.ErrFileUnavailable
		}
		return err
	})
}

// MarkImportSourceDiscarded serializes compensation with batch binding on the file row.
func (p *Postgres) MarkImportSourceDiscarded(ctx context.Context, file storage.File, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		current, err := scanFile(tx.QueryRow(ctx, fileSelect+` WHERE id=$1 FOR UPDATE`, file.ID))
		if err != nil {
			return err
		}
		if current.Status != storage.StatusAvailable || current.RetentionType != storage.RetentionImportSource || mutation.Metadata.ActorID == "" || current.CreatedBy != mutation.Metadata.ActorID {
			return storage.ErrFileUnavailable
		}
		var bound bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM import_batches WHERE original_file_id=$1)
			OR EXISTS(SELECT 1 FROM master_data_import_batches WHERE source_file_id=$1)
			OR EXISTS(SELECT 1 FROM academic_week_import_batches WHERE source_file_id=$1)`, file.ID).Scan(&bound); err != nil {
			return err
		}
		if bound {
			return storage.ErrFileUnavailable
		}
		_, err = tx.Exec(ctx, `UPDATE files SET status='FAILED',failure_code=$2,failure_message=$3,updated_at=now() WHERE id=$1`, file.ID, file.FailureCode, file.FailureMessage)
		return err
	})
}

// MarkFileMissing atomically enforces AVAILABLE -> MISSING after an object lookup proves absence.
func (p *Postgres) MarkFileMissing(ctx context.Context, file storage.File, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE files SET status='MISSING',failure_code=$2,failure_message=$3,updated_at=now()
			WHERE id=$1 AND status='AVAILABLE'`, file.ID, file.FailureCode, file.FailureMessage)
		if err == nil && tag.RowsAffected() == 0 {
			return storage.ErrFileUnavailable
		}
		return err
	})
}

// RecordFileCleanup commits a removal marker or retry timestamp together with its audit.
func (p *Postgres) RecordFileCleanup(ctx context.Context, id string, removed bool, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE files SET object_removed_at=CASE WHEN $2 THEN now() ELSE NULL END,updated_at=now()
   WHERE id=$1 AND status='FAILED' AND object_removed_at IS NULL`, id, removed)
		if err == nil && tag.RowsAffected() == 0 {
			return storage.ErrFileUnavailable
		}
		return err
	})
}

// RecordFileRecoveryDeferred preserves PENDING metadata while yielding a transiently failing row.
func (p *Postgres) RecordFileRecoveryDeferred(ctx context.Context, id string, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE files SET updated_at=now() WHERE id=$1 AND status='PENDING'`, id)
		if err == nil && tag.RowsAffected() == 0 {
			return storage.ErrFileUnavailable
		}
		return err
	})
}

// ListFileReconciliationCandidates selects only stale recoverable rows; it never enumerates MinIO.
func (p *Postgres) ListFileReconciliationCandidates(ctx context.Context, bucket string, staleBefore time.Time, limit int) ([]storage.File, error) {
	rows, err := p.Pool.Query(ctx, fileSelect+` WHERE bucket=$1 AND status IN ('PENDING','FAILED') AND object_removed_at IS NULL AND updated_at < $2 ORDER BY updated_at,id LIMIT $3`, bucket, staleBefore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var files []storage.File
	for rows.Next() {
		file, err := scanFile(rows)
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, rows.Err()
}

// RecordFileAudit appends a read-only file event without changing metadata.
func (p *Postgres) RecordFileAudit(ctx context.Context, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(pgx.Tx) error { return nil })
}

const fileSelect = `SELECT id,storage_provider,bucket,object_key,original_name,declared_content_type,
	detected_content_type,size_bytes,sha256,status,retention_type,COALESCE(created_by::text,''),
	failure_code,failure_message,created_at,updated_at FROM files`

func scanFile(row interface{ Scan(...any) error }) (storage.File, error) {
	var file storage.File
	err := row.Scan(&file.ID, &file.StorageProvider, &file.Bucket, &file.ObjectKey, &file.OriginalName,
		&file.DeclaredContentType, &file.DetectedContentType, &file.SizeBytes, &file.SHA256, &file.Status,
		&file.RetentionType, &file.CreatedBy, &file.FailureCode, &file.FailureMessage, &file.CreatedAt, &file.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return storage.File{}, storage.ErrFileNotFound
	}
	return file, err
}

// Create 持久化账号，并将数据库唯一约束冲突转换为稳定的领域错误。
func (p *Postgres) Create(ctx context.Context, a auth.Account, mutation audit.Mutation) error {
	var studentNo any
	if a.StudentNo != "" {
		studentNo = a.StudentNo
	}
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO accounts(
			id,login_name,student_no,name,password_hash,status,account_kind,is_super_admin,system_role,must_change_password,
			requested_department_id,requested_org_role_id,created_at
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,NULLIF($11,'')::uuid,NULLIF($12,'')::uuid,$13)`,
			a.ID, a.LoginName, studentNo, a.Name, a.PasswordHash, a.Status, a.AccountKind, a.IsSuperAdmin,
			a.SystemRole, a.MustChangePassword, a.RequestedDepartmentID, a.RequestedOrgRoleID, a.CreatedAt)
		return execErr
	})
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return auth.ErrStudentNoExists
	}
	return err
}

// FindByLoginName 统一支持系统账号名和普通用户学号登录。
func (p *Postgres) FindByLoginName(ctx context.Context, loginName string) (auth.Account, error) {
	return scanAccount(p.Pool.QueryRow(ctx, accountSelect+` WHERE login_name=$1`, loginName))
}

// FindByID 读取账号实时状态，用于令牌校验和账号禁用后的会话失效。
func (p *Postgres) FindByID(ctx context.Context, id string) (auth.Account, error) {
	return scanAccount(p.Pool.QueryRow(ctx, accountSelect+` WHERE id=$1`, id))
}

const accountSelect = `SELECT id,login_name,COALESCE(student_no,''),name,password_hash,status,account_kind,
	is_super_admin,system_role,must_change_password,session_version,COALESCE(primary_department_id::text,''),
	COALESCE(requested_department_id::text,''),COALESCE(requested_org_role_id::text,''),created_at FROM accounts`

type row interface{ Scan(...any) error }

func scanAccount(r row) (auth.Account, error) {
	var a auth.Account
	err := r.Scan(&a.ID, &a.LoginName, &a.StudentNo, &a.Name, &a.PasswordHash, &a.Status, &a.AccountKind,
		&a.IsSuperAdmin, &a.SystemRole, &a.MustChangePassword, &a.SessionVersion, &a.PrimaryDepartmentID, &a.RequestedDepartmentID,
		&a.RequestedOrgRoleID, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, auth.ErrAccountNotFound
	}
	return a, err
}

func (p *Postgres) UpdateAccount(ctx context.Context, account auth.Account, mutation audit.Mutation) (auth.Account, error) {
	var updated auth.Account
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE accounts SET login_name=$2,student_no=$3,name=$4,system_role=$5,
			session_version=session_version+1,updated_at=now() WHERE id=$1 AND NOT is_super_admin AND session_version=$6`, account.ID, account.LoginName, account.StudentNo, account.Name, account.SystemRole, account.SessionVersion)
		if execErr == nil && tag.RowsAffected() == 0 {
			return auth.ErrVersionConflict
		}
		if execErr != nil {
			return execErr
		}
		if execErr = revokeAccountSessionsTx(ctx, tx, account.ID); execErr != nil {
			return execErr
		}
		updated, execErr = scanAccount(tx.QueryRow(ctx, accountSelect+` WHERE id=$1`, account.ID))
		return execErr
	})
	if err != nil {
		return auth.Account{}, err
	}
	return updated, nil
}

func (p *Postgres) SetStatus(ctx context.Context, accountID, status string, expectedVersion int, mutation audit.Mutation) (auth.Account, error) {
	var updated auth.Account
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE accounts SET status=$2,session_version=session_version+1,updated_at=now()
			WHERE id=$1 AND NOT is_super_admin AND session_version=$3`, accountID, status, expectedVersion)
		if execErr == nil && tag.RowsAffected() == 0 {
			return auth.ErrVersionConflict
		}
		if execErr != nil {
			return execErr
		}
		if execErr = revokeAccountSessionsTx(ctx, tx, accountID); execErr != nil {
			return execErr
		}
		updated, execErr = scanAccount(tx.QueryRow(ctx, accountSelect+` WHERE id=$1`, accountID))
		return execErr
	})
	if err != nil {
		return auth.Account{}, err
	}
	return updated, nil
}

func (p *Postgres) UpdatePassword(ctx context.Context, accountID, passwordHash string, mustChange bool, expectedVersion int, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		// Password verification used this version. Reject a delayed write after
		// another password, status, role or permission change has committed.
		tag, execErr := tx.Exec(ctx, `UPDATE accounts SET password_hash=$2,must_change_password=$3,session_version=session_version+1,updated_at=now() WHERE id=$1 AND session_version=$4`, accountID, passwordHash, mustChange, expectedVersion)
		if execErr == nil && tag.RowsAffected() == 0 {
			return auth.ErrVersionConflict
		}
		if execErr != nil {
			return execErr
		}
		return revokeAccountSessionsTx(ctx, tx, accountID)
	})
}

// The account row is already locked by its mutation; keep the same account →
// session lock order as refresh rotation and permission changes.
func revokeAccountSessionsTx(ctx context.Context, tx pgx.Tx, accountID string) error {
	_, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=now() WHERE account_id=$1 AND revoked_at IS NULL`, accountID)
	return err
}

func (p *Postgres) ListPending(ctx context.Context) ([]auth.Account, error) {
	rows, err := p.Pool.Query(ctx, accountSelect+` WHERE status='PENDING' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []auth.Account
	for rows.Next() {
		a, err := scanAccount(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, a)
	}
	return result, rows.Err()
}

func (p *Postgres) ReviewRegistration(ctx context.Context, accountID, reviewerID, decision, reason string, reviewedAt time.Time, mutation audit.Mutation) (auth.Account, error) {
	var status, departmentID, roleID string
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		queryErr := tx.QueryRow(ctx, `SELECT status,COALESCE(requested_department_id::text,''),COALESCE(requested_org_role_id::text,'') FROM accounts WHERE id=$1 FOR UPDATE`, accountID).Scan(&status, &departmentID, &roleID)
		if errors.Is(queryErr, pgx.ErrNoRows) {
			return auth.ErrAccountNotFound
		}
		if queryErr != nil {
			return queryErr
		}
		if status != auth.StatusPending {
			return auth.ErrInvalidTransition
		}
		newStatus := auth.StatusRejected
		if decision == auth.ReviewApprove {
			newStatus = auth.StatusActive
		}
		if _, execErr := tx.Exec(ctx, `UPDATE accounts SET status=$2::varchar(24),primary_department_id=CASE WHEN $2::varchar(24)='ACTIVE' THEN requested_department_id ELSE primary_department_id END,
			reviewed_by=$3,reviewed_at=$4,review_reason=$5,updated_at=$4 WHERE id=$1`, accountID, newStatus, reviewerID, reviewedAt, reason); execErr != nil {
			return execErr
		}
		if decision == auth.ReviewApprove {
			_, execErr := tx.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id) VALUES($1,$2,$3,$4)
				ON CONFLICT(account_id,department_id,organization_role_id) DO NOTHING`, uuid.NewString(), accountID, departmentID, roleID)
			return execErr
		}
		return nil
	})
	if err != nil {
		return auth.Account{}, err
	}
	return p.FindByID(ctx, accountID)
}

// Save persists only the hashed credential, after checking the account snapshot
// used by Login. Its account lock serializes with password/status/role changes.
func (p *Postgres) Save(ctx context.Context, s auth.RefreshSession, expectedVersion int) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var accountID string
	err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE id=$1 AND status='ACTIVE' AND session_version=$2 FOR UPDATE`, s.AccountID, expectedVersion).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrInvalidCredentials
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO refresh_sessions(id,account_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, s.ID, accountID, s.TokenHash, s.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FindRefreshSession reads a candidate without consuming it. Rotate must check it
// again under a lock; this read alone never authorizes credential issuance.
func (p *Postgres) FindRefreshSession(ctx context.Context, hash string, now time.Time) (auth.RefreshSession, error) {
	var s auth.RefreshSession
	err := p.Pool.QueryRow(ctx, `SELECT id,account_id,token_hash,expires_at,revoked_at FROM refresh_sessions WHERE token_hash=$1`, hash).Scan(&s.ID, &s.AccountID, &s.TokenHash, &s.ExpiresAt, &s.RevokedAt)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (s.RevokedAt != nil || !s.ExpiresAt.After(now))) {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	return s, err
}

// Rotate commits consumption and replacement together. Lock the account before
// sessions, matching password/status/permission mutations and preventing a stale
// account read from creating a session after those mutations have invalidated it.
func (p *Postgres) Rotate(ctx context.Context, hash string, now time.Time, replacement auth.RefreshSession, expectedVersion int) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var accountID string
	err = tx.QueryRow(ctx, `SELECT id FROM accounts WHERE id=$1 AND status='ACTIVE' AND session_version=$2 FOR UPDATE`, replacement.AccountID, expectedVersion).Scan(&accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return auth.ErrInvalidRefreshToken
	}
	if err != nil {
		return err
	}
	consumed, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=$2 WHERE token_hash=$1 AND account_id=$3 AND revoked_at IS NULL AND expires_at>$2`, hash, now, accountID)
	if err != nil {
		return err
	}
	if consumed.RowsAffected() != 1 {
		return auth.ErrInvalidRefreshToken
	}
	if _, err = tx.Exec(ctx, `INSERT INTO refresh_sessions(id,account_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, replacement.ID, accountID, replacement.TokenHash, replacement.ExpiresAt); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Revoke 在退出登录时幂等地作废刷新会话。
func (p *Postgres) Revoke(ctx context.Context, hash string, now time.Time) error {
	_, err := p.Pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE token_hash=$1`, hash, now)
	return err
}

// RevokeAll 在改密、禁用和权限敏感变更后作废账号全部有效刷新会话。
func (p *Postgres) RevokeAll(ctx context.Context, accountID string, now time.Time) error {
	_, err := p.Pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,$2) WHERE account_id=$1 AND revoked_at IS NULL`, accountID, now)
	return err
}

func (p *Postgres) ListCatalogs(ctx context.Context, enabledOnly bool) (organization.Catalogs, error) {
	filter := ""
	if enabledOnly {
		filter = " WHERE enabled"
	}
	var result organization.Catalogs
	departmentRows, err := p.Pool.Query(ctx, `SELECT id,code,name,enabled FROM departments`+filter+` ORDER BY code`)
	if err != nil {
		return result, err
	}
	for departmentRows.Next() {
		var value organization.Department
		if err := departmentRows.Scan(&value.ID, &value.Code, &value.Name, &value.Enabled); err != nil {
			departmentRows.Close()
			return result, err
		}
		result.Departments = append(result.Departments, value)
	}
	departmentRows.Close()
	roleRows, err := p.Pool.Query(ctx, `SELECT id,code,name,enabled,sort_order FROM organization_roles`+filter+` ORDER BY sort_order,code`)
	if err != nil {
		return result, err
	}
	for roleRows.Next() {
		var value organization.OrganizationRole
		if err := roleRows.Scan(&value.ID, &value.Code, &value.Name, &value.Enabled, &value.SortOrder); err != nil {
			roleRows.Close()
			return result, err
		}
		result.OrganizationRoles = append(result.OrganizationRoles, value)
	}
	roleRows.Close()
	typeRows, err := p.Pool.Query(ctx, `SELECT id,code,name,extension_kind,enabled,participates_in_report,sort_order FROM inspection_types`+filter+` ORDER BY sort_order,code`)
	if err != nil {
		return result, err
	}
	defer typeRows.Close()
	for typeRows.Next() {
		var value organization.InspectionType
		if err := typeRows.Scan(&value.ID, &value.Code, &value.Name, &value.ExtensionKind, &value.Enabled, &value.ParticipatesInReport, &value.SortOrder); err != nil {
			return result, err
		}
		result.InspectionTypes = append(result.InspectionTypes, value)
	}
	return result, typeRows.Err()
}

func (p *Postgres) CreateDepartment(ctx context.Context, department organization.Department, mutation audit.Mutation) (organization.Department, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO departments(id,code,name,enabled) VALUES($1,$2,$3,$4)`, department.ID, department.Code, department.Name, department.Enabled)
		return execErr
	})
	return department, err
}

func (p *Postgres) UpdateDepartment(ctx context.Context, department organization.Department, mutation audit.Mutation) (organization.Department, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE departments SET name=$2,enabled=$3,updated_at=now() WHERE id=$1`, department.ID, department.Name, department.Enabled)
		return execErr
	})
	return department, err
}

func (p *Postgres) UpdateInspectionType(ctx context.Context, value organization.InspectionType, mutation audit.Mutation) (organization.InspectionType, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE inspection_types SET name=$2,enabled=$3,participates_in_report=$4,sort_order=$5 WHERE id=$1`, value.ID, value.Name, value.Enabled, value.ParticipatesInReport, value.SortOrder)
		return execErr
	})
	return value, err
}

func (p *Postgres) ListScopes(ctx context.Context) ([]organization.DepartmentInspectionScope, error) {
	rows, err := p.Pool.Query(ctx, `SELECT department_id,inspection_type_id,can_enter,can_view,can_review,can_export,can_summarize,enabled FROM department_inspection_scopes ORDER BY department_id,inspection_type_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []organization.DepartmentInspectionScope
	for rows.Next() {
		var value organization.DepartmentInspectionScope
		if err := rows.Scan(&value.DepartmentID, &value.InspectionTypeID, &value.CanEnter, &value.CanView, &value.CanReview, &value.CanExport, &value.CanSummarize, &value.Enabled); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) UpsertScope(ctx context.Context, value organization.DepartmentInspectionScope, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO department_inspection_scopes(department_id,inspection_type_id,can_enter,can_view,can_review,can_export,can_summarize,enabled)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(department_id,inspection_type_id) DO UPDATE SET
			can_enter=EXCLUDED.can_enter,can_view=EXCLUDED.can_view,can_review=EXCLUDED.can_review,can_export=EXCLUDED.can_export,
			can_summarize=EXCLUDED.can_summarize,enabled=EXCLUDED.enabled,updated_at=now()`, value.DepartmentID, value.InspectionTypeID,
			value.CanEnter, value.CanView, value.CanReview, value.CanExport, value.CanSummarize, value.Enabled)
		if err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `UPDATE accounts SET session_version=session_version+1,updated_at=now() WHERE primary_department_id=$1`, value.DepartmentID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id IN (SELECT id FROM accounts WHERE primary_department_id=$1) AND revoked_at IS NULL`, value.DepartmentID)
		return err
	})
}

func (p *Postgres) ListUsers(ctx context.Context) ([]organization.UserAssignment, error) {
	rows, err := p.Pool.Query(ctx, `SELECT a.id,a.login_name,COALESCE(a.student_no,''),a.name,a.status,a.system_role,a.is_super_admin,a.must_change_password,
		COALESCE(a.primary_department_id::text,''),COALESCE(u.organization_role_id::text,''),a.session_version
		FROM accounts a LEFT JOIN user_org_roles u ON u.account_id=a.id ORDER BY a.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []organization.UserAssignment
	for rows.Next() {
		var value organization.UserAssignment
		if err := rows.Scan(&value.AccountID, &value.LoginName, &value.StudentNo, &value.Name, &value.Status, &value.SystemRole,
			&value.IsSuperAdmin, &value.MustChangePassword, &value.DepartmentID, &value.OrganizationRoleID, &value.SessionVersion); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AssignUserRole(ctx context.Context, value organization.UserAssignment, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		// Hold reference locks through assignment commit so concurrent disabling
		// cannot invalidate a reference between validation and the account write.
		var referenceID string
		for _, reference := range []struct{ query, id string }{
			{`SELECT id FROM departments WHERE id=$1 AND enabled FOR SHARE`, value.DepartmentID},
			{`SELECT id FROM organization_roles WHERE id=$1 AND enabled FOR SHARE`, value.OrganizationRoleID},
		} {
			if _, err := uuid.Parse(reference.id); err != nil {
				return organization.ErrInvalidInput
			}
			err := tx.QueryRow(ctx, reference.query, reference.id).Scan(&referenceID)
			if errors.Is(err, pgx.ErrNoRows) {
				return organization.ErrInvalidInput
			}
			if err != nil {
				return err
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE accounts SET primary_department_id=$2,session_version=session_version+1,updated_at=now() WHERE id=$1 AND session_version=$3`, value.AccountID, value.DepartmentID, value.SessionVersion)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return auth.ErrVersionConflict
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_org_roles WHERE account_id=$1`, value.AccountID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id) VALUES($1,$2,$3,$4)`, uuid.NewString(), value.AccountID, value.DepartmentID, value.OrganizationRoleID); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1 AND revoked_at IS NULL`, value.AccountID)
		return err
	})
}

func (p *Postgres) LoadAccessProfile(ctx context.Context, accountID string) (permission.AccessProfile, error) {
	var profile permission.AccessProfile
	err := p.Pool.QueryRow(ctx, `SELECT a.id,a.system_role,COALESCE(a.primary_department_id::text,''),
		COALESCE(d.code,''),COALESCE(r.code,'') FROM accounts a
		LEFT JOIN departments d ON d.id=a.primary_department_id
		LEFT JOIN user_org_roles ur ON ur.account_id=a.id AND (ur.start_at IS NULL OR ur.start_at<=CURRENT_DATE) AND (ur.end_at IS NULL OR ur.end_at>=CURRENT_DATE)
		LEFT JOIN organization_roles r ON r.id=ur.organization_role_id
		WHERE a.id=$1`, accountID).Scan(&profile.AccountID, &profile.SystemRole, &profile.DepartmentID, &profile.DepartmentCode, &profile.OrganizationRoleCode)
	if errors.Is(err, pgx.ErrNoRows) {
		return profile, auth.ErrAccountNotFound
	}
	if err != nil {
		return profile, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT department_id,inspection_type_id,can_enter,can_view,can_review,can_export,can_summarize,enabled
		FROM department_inspection_scopes WHERE department_id=$1`, profile.DepartmentID)
	if err != nil {
		return profile, err
	}
	for rows.Next() {
		var value permission.DepartmentScope
		if err := rows.Scan(&value.DepartmentID, &value.InspectionTypeID, &value.CanEnter, &value.CanView, &value.CanReview, &value.CanExport, &value.CanSummarize, &value.Enabled); err != nil {
			rows.Close()
			return profile, err
		}
		profile.DepartmentScopes = append(profile.DepartmentScopes, value)
	}
	rows.Close()
	profile.Rules, err = p.ListRules(ctx, accountID)
	return profile, err
}

func (p *Postgres) ListRules(ctx context.Context, accountID string) ([]permission.Rule, error) {
	rows, err := p.Pool.Query(ctx, permissionRuleSelect+` WHERE account_id=$1 ORDER BY created_at,id`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []permission.Rule
	for rows.Next() {
		value, err := scanPermissionRule(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

const permissionRuleSelect = `SELECT id,account_id,action,effect,source_type,
	COALESCE(academic_year_id::text,''),COALESCE(semester_id::text,''),COALESCE(week_id::text,''),
	COALESCE(inspection_type_id::text,''),COALESCE(department_id::text,''),COALESCE(class_id::text,''),
	COALESCE(grade,''),note,enabled,COALESCE(created_by::text,'') FROM user_permission_rules`

func scanPermissionRule(r row) (permission.Rule, error) {
	var value permission.Rule
	err := r.Scan(&value.ID, &value.AccountID, &value.Action, &value.Effect, &value.SourceType,
		&value.Scope.AcademicYearID, &value.Scope.SemesterID, &value.Scope.WeekID, &value.Scope.InspectionTypeID,
		&value.Scope.DepartmentID, &value.Scope.ClassID, &value.Scope.Grade, &value.Note, &value.Enabled, &value.CreatedBy)
	return value, err
}

func (p *Postgres) CreateRule(ctx context.Context, value permission.Rule, mutation audit.Mutation) (permission.Rule, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO user_permission_rules(id,account_id,action,effect,source_type,academic_year_id,semester_id,week_id,
			inspection_type_id,department_id,class_id,grade,note,enabled,created_by) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,
			NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,NULLIF($10,'')::uuid,NULLIF($11,'')::uuid,NULLIF($12,''),$13,$14,NULLIF($15,'')::uuid)`,
			value.ID, value.AccountID, value.Action, value.Effect, value.SourceType, value.Scope.AcademicYearID, value.Scope.SemesterID,
			value.Scope.WeekID, value.Scope.InspectionTypeID, value.Scope.DepartmentID, value.Scope.ClassID, value.Scope.Grade,
			value.Note, value.Enabled, value.CreatedBy)
		if execErr != nil {
			return execErr
		}
		return invalidatePermissionSessions(ctx, tx, value.AccountID)
	})
	return value, err
}

func (p *Postgres) UpdateRule(ctx context.Context, value permission.Rule, mutation audit.Mutation) (permission.Rule, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE user_permission_rules SET action=$3,effect=$4,academic_year_id=NULLIF($5,'')::uuid,
			semester_id=NULLIF($6,'')::uuid,week_id=NULLIF($7,'')::uuid,inspection_type_id=NULLIF($8,'')::uuid,
			department_id=NULLIF($9,'')::uuid,class_id=NULLIF($10,'')::uuid,grade=NULLIF($11,''),note=$12,enabled=$13,updated_at=now()
			WHERE id=$1 AND account_id=$2`, value.ID, value.AccountID, value.Action, value.Effect, value.Scope.AcademicYearID,
			value.Scope.SemesterID, value.Scope.WeekID, value.Scope.InspectionTypeID, value.Scope.DepartmentID, value.Scope.ClassID,
			value.Scope.Grade, value.Note, value.Enabled)
		if execErr != nil {
			return execErr
		}
		if tag.RowsAffected() == 0 {
			return permission.ErrInvalidRule
		}
		return invalidatePermissionSessions(ctx, tx, value.AccountID)
	})
	return value, err
}

func (p *Postgres) DeleteRule(ctx context.Context, accountID, ruleID string, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `DELETE FROM user_permission_rules WHERE id=$1 AND account_id=$2`, ruleID, accountID); err != nil {
			return err
		}
		return invalidatePermissionSessions(ctx, tx, accountID)
	})
}

func invalidatePermissionSessions(ctx context.Context, tx pgx.Tx, accountID string) error {
	if _, err := tx.Exec(ctx, `UPDATE accounts SET session_version=session_version+1,updated_at=now() WHERE id=$1`, accountID); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1 AND revoked_at IS NULL`, accountID)
	return err
}

func (p *Postgres) ListMasterData(ctx context.Context, query masterdata.Query) (masterdata.Page, error) {
	selectSQL, table, codeColumn, nameColumn, ok := serverManagedMasterRecordSelect(query.Kind)
	if !ok {
		return masterdata.Page{}, masterdata.ErrInvalidKind
	}
	filter, args := masterDataFilter(query, codeColumn, nameColumn)
	var total int
	if err := p.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table+filter, args...).Scan(&total); err != nil {
		return masterdata.Page{}, err
	}
	limitPosition := len(args) + 1
	listArgs := append(append([]any{}, args...), query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := p.Pool.Query(ctx, selectSQL+filter+` ORDER BY `+codeColumn+fmt.Sprintf(` LIMIT $%d OFFSET $%d`, limitPosition, limitPosition+1), listArgs...)
	if err != nil {
		return masterdata.Page{}, err
	}
	defer rows.Close()
	result := masterdata.Page{Total: total, Page: query.Page, PageSize: query.PageSize}
	for rows.Next() {
		value, err := scanMasterRecord(rows, query.Kind)
		if err != nil {
			return masterdata.Page{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, rows.Err()
}

func masterDataFilter(query masterdata.Query, codeColumn, nameColumn string) (string, []any) {
	var enabled any
	if query.Enabled != nil {
		enabled = *query.Enabled
	}
	clauses := []string{
		fmt.Sprintf(`($1='' OR %s ILIKE '%%'||$1||'%%' OR %s ILIKE '%%'||$1||'%%')`, codeColumn, nameColumn),
		fmt.Sprintf(`($2='' OR %s ILIKE '%%'||$2||'%%')`, codeColumn),
		fmt.Sprintf(`($3='' OR %s ILIKE '%%'||$3||'%%')`, nameColumn),
		`($4::boolean IS NULL OR enabled=$4)`,
	}
	args := []any{query.Keyword, query.Code, query.Name, enabled}
	add := func(column, value string) {
		if value == "" {
			return
		}
		args = append(args, value)
		clauses = append(clauses, fmt.Sprintf(`%s=$%d`, column, len(args)))
	}
	switch query.Kind {
	case masterdata.KindClassroom:
		add("teaching_building_id", query.TeachingBuildingID)
	case masterdata.KindClassroomHygieneArea:
		add("classroom_id", query.ClassroomID)
	case masterdata.KindPublicDormitoryHygieneArea:
		add("dormitory_building_id", query.DormitoryBuildingID)
	case masterdata.KindDormitoryBuilding:
		add("dormitory_area_id", query.DormitoryAreaID)
	case masterdata.KindMajor:
		add("faculty_id", query.FacultyID)
	case masterdata.KindStaff:
		add("faculty_id", query.FacultyID)
		add("gender", query.Gender)
	case masterdata.KindClass:
		add("faculty_id", query.FacultyID)
		add("major_id", query.MajorID)
		add("head_teacher_id", query.HeadTeacherID)
		add("grade_id", query.GradeID)
	case masterdata.KindStudent:
		add("faculty_id", query.FacultyID)
		add("major_id", query.MajorID)
		add("class_id", query.ClassID)
		add("dormitory_id", query.DormitoryID)
		add("grade_id", query.GradeID)
		add("gender", query.Gender)
		add("enrollment_status", query.EnrollmentStatus)
	case masterdata.KindCourse:
	case masterdata.KindDormitory:
		add("dormitory_building_id", query.DormitoryBuildingID)
		if query.DormitoryAreaID != "" {
			args = append(args, query.DormitoryAreaID)
			clauses = append(clauses, fmt.Sprintf(`dormitory_building_id IN (SELECT id FROM dormitory_buildings WHERE dormitory_area_id=$%d)`, len(args)))
		}
	}
	return ` WHERE ` + strings.Join(clauses, ` AND `), args
}

func (p *Postgres) FindMasterData(ctx context.Context, kind, id string) (masterdata.Record, error) {
	selectSQL, _, _, _, ok := serverManagedMasterRecordSelect(kind)
	if !ok {
		return masterdata.Record{}, masterdata.ErrInvalidKind
	}
	value, err := scanMasterRecord(p.Pool.QueryRow(ctx, selectSQL+` WHERE id=$1`, id), kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return masterdata.Record{}, masterdata.ErrNotFound
	}
	return value, err
}

func (p *Postgres) MasterDataExists(ctx context.Context, kind, id string) (bool, error) {
	_, table, _, _, ok := serverManagedMasterRecordSelect(kind)
	if !ok {
		return false, masterdata.ErrInvalidKind
	}
	var exists bool
	err := p.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id=$1 AND enabled)`, id).Scan(&exists)
	return exists, err
}

func (p *Postgres) HygieneAreaInspectionScope(ctx context.Context, kind, departmentID string) (string, error) {
	code := "CLASSROOM_HYGIENE"
	if kind == masterdata.KindPublicDormitoryHygieneArea {
		code = "PUBLIC_DORMITORY_HYGIENE"
	}
	var inspectionTypeID string
	err := p.Pool.QueryRow(ctx, `SELECT it.id FROM department_inspection_scopes ds
 JOIN inspection_types it ON it.id=ds.inspection_type_id JOIN departments d ON d.id=ds.department_id
 WHERE ds.department_id=$1 AND it.code=$2 AND ds.enabled AND it.enabled AND d.enabled`, departmentID, code).Scan(&inspectionTypeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", masterdata.ErrInvalidRelation
	}
	return inspectionTypeID, err
}

func (p *Postgres) CreateMasterData(ctx context.Context, actorID string, value masterdata.Record, mutation audit.Mutation) (masterdata.Record, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if err := insertMasterRecord(ctx, tx, value); err != nil {
			return err
		}
		after, _ := json.Marshal(value)
		return insertMasterChange(ctx, tx, actorID, value.Kind, value.ID, "CREATE", nil, after)
	})
	if err != nil {
		return masterdata.Record{}, masterDataError(err)
	}
	return p.FindMasterData(ctx, value.Kind, value.ID)
}

func (p *Postgres) UpdateMasterData(ctx context.Context, actorID string, value masterdata.Record, mutation audit.Mutation) (masterdata.Record, error) {
	before, err := p.FindMasterData(ctx, value.Kind, value.ID)
	if err != nil {
		return masterdata.Record{}, err
	}
	err = p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if updateErr := updateMasterRecord(ctx, tx, value); updateErr != nil {
			return updateErr
		}
		beforeJSON, _ := json.Marshal(before)
		afterJSON, _ := json.Marshal(value)
		return insertMasterChange(ctx, tx, actorID, value.Kind, value.ID, "UPDATE", beforeJSON, afterJSON)
	})
	if err != nil {
		return masterdata.Record{}, masterDataError(err)
	}
	return p.FindMasterData(ctx, value.Kind, value.ID)
}

func masterDataError(err error) error {
	var databaseError *pgconn.PgError
	if !errors.As(err, &databaseError) {
		return err
	}
	if databaseError.Code != "23505" {
		return err
	}
	switch databaseError.ConstraintName {
	case "staff_employee_no_key", "students_student_no_key":
		return masterdata.ErrDuplicateCode
	case "faculties_name_key", "grades_name_key", "classes_name_key":
		return masterdata.ErrDuplicateName
	// PostgreSQL truncates this composite constraint name, including the name column.
	case "public_dormitory_hygiene_area_dormitory_building_id_floor_u_key":
		return masterdata.ErrDuplicateName
	default:
		if strings.Contains(databaseError.ConstraintName, "name") {
			return masterdata.ErrDuplicateName
		}
		return err
	}
}

func (p *Postgres) ListMasterDataChanges(ctx context.Context, kind, id string) ([]masterdata.Change, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id,entity_type,entity_id,action,COALESCE(before_json,'null'::jsonb),COALESCE(after_json,'null'::jsonb),
		COALESCE(actor_id::text,''),created_at FROM master_data_changes WHERE entity_type=$1 AND entity_id=$2 ORDER BY created_at DESC`, kind, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []masterdata.Change
	for rows.Next() {
		var value masterdata.Change
		if err := rows.Scan(&value.ID, &value.EntityType, &value.EntityID, &value.Action, &value.BeforeJSON, &value.AfterJSON, &value.ActorID, &value.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func serverManagedMasterRecordSelect(kind string) (string, string, string, string, bool) {
	const empty = `''::text`
	columns := []string{
		"id", empty, "name", "enabled", empty, empty, empty, empty, empty, empty, empty, empty, empty,
		empty, "0", empty, empty, empty, empty, "0", empty, empty, empty, empty, "created_at", "updated_at",
		empty, "ARRAY[]::text[]", "'[]'::jsonb", empty,
	}
	table, identifierColumn, nameColumn := "", "name", "name"
	switch kind {
	case masterdata.KindFaculty:
		table = "faculties"
	case masterdata.KindGrade:
		table = "grades"
	case masterdata.KindDormitoryArea:
		table = "dormitory_areas"
	case masterdata.KindDormitoryBuilding:
		table, columns[22] = "dormitory_buildings", "dormitory_area_id::text"
	case masterdata.KindTeachingBuilding:
		table = "teaching_buildings"
	case masterdata.KindMajor:
		table, columns[4] = "majors", "faculty_id::text"
	case masterdata.KindStaff:
		table, identifierColumn, columns[1], columns[4], columns[11], columns[12] = "staff", "employee_no", "employee_no", "COALESCE(faculty_id::text,'')", "gender", "phone"
	case masterdata.KindClass:
		table, columns[4], columns[5], columns[6] = "classes", "COALESCE(faculty_id::text,'')", "COALESCE(major_id::text,'')", "COALESCE(head_teacher_id::text,'')"
		columns[13], columns[14], columns[26] = "COALESCE((SELECT name FROM grades WHERE id=classes.grade_id),'')", "enrolled_size", "grade_id::text"
	case masterdata.KindDormitory:
		table, identifierColumn, nameColumn = "dormitories", "room_no", "room_no"
		columns[2], columns[18], columns[22], columns[23] = "room_no", "room_no", "(SELECT dormitory_area_id::text FROM dormitory_buildings WHERE id=dormitories.dormitory_building_id)", "dormitory_building_id::text"
	case masterdata.KindClassroom:
		table, identifierColumn = "classrooms", "room_no"
		nameColumn = "(SELECT name FROM teaching_buildings WHERE id=classrooms.teaching_building_id) || ' ' || room_no"
		columns[2], columns[16], columns[18] = nameColumn, "teaching_building_id::text", "room_no"
	case masterdata.KindClassroomHygieneArea:
		table, columns[10] = "classroom_hygiene_areas", "classroom_id::text"
	case masterdata.KindPublicDormitoryHygieneArea:
		table, columns[23], columns[29] = "public_dormitory_hygiene_areas", "dormitory_building_id::text", "floor_unit"
	case masterdata.KindCourse:
		table = "courses"
	case masterdata.KindStudent:
		table, identifierColumn, columns[1] = "students", "student_no", "student_no"
		columns[4], columns[5], columns[8], columns[9], columns[11] = "COALESCE(faculty_id::text,'')", "COALESCE(major_id::text,'')", "COALESCE(class_id::text,'')", "COALESCE(dormitory_id::text,'')", "gender"
		columns[13], columns[21], columns[26] = "COALESCE((SELECT name FROM grades WHERE id=students.grade_id),'')", "enrollment_status", "grade_id::text"
	default:
		return "", "", "", "", false
	}
	return "SELECT " + strings.Join(columns, ",") + " FROM " + table, table, identifierColumn, nameColumn, true
}

func scanMasterRecord(r row, kind string) (masterdata.Record, error) {
	value := masterdata.Record{Kind: kind}
	var ignoredTeacherID, ignoredScheduleText string
	var ignoredClassIDs []string
	var ignoredSchedulesJSON []byte
	var ignoredFloor int
	err := r.Scan(&value.ID, &value.Code, &value.Name, &value.Enabled, &value.FacultyID, &value.MajorID,
		&value.HeadTeacherID, &ignoredTeacherID, &value.ClassID, &value.DormitoryID, &value.ClassroomID, &value.Gender,
		&value.Phone, &value.Grade, &value.EnrolledSize, &value.CampusArea, &value.TeachingBuildingID, &value.BuildingNo,
		&value.RoomNo, &ignoredFloor, &ignoredScheduleText, &value.EnrollmentStatus, &value.DormitoryAreaID,
		&value.DormitoryBuildingID, &value.CreatedAt, &value.UpdatedAt, &value.GradeID, &ignoredClassIDs, &ignoredSchedulesJSON, &value.FloorUnit)
	return value, err
}

func insertMasterRecord(ctx context.Context, tx pgx.Tx, value masterdata.Record) error {
	var err error
	switch value.Kind {
	case masterdata.KindFaculty:
		_, err = tx.Exec(ctx, `INSERT INTO faculties(id,name,enabled) VALUES($1,$2,$3)`, value.ID, value.Name, value.Enabled)
	case masterdata.KindGrade:
		_, err = tx.Exec(ctx, `INSERT INTO grades(id,name,enabled) VALUES($1,$2,$3)`, value.ID, value.Name, value.Enabled)
	case masterdata.KindDormitoryArea:
		_, err = tx.Exec(ctx, `INSERT INTO dormitory_areas(id,name,enabled) VALUES($1,$2,$3)`, value.ID, value.Name, value.Enabled)
	case masterdata.KindDormitoryBuilding:
		_, err = tx.Exec(ctx, `INSERT INTO dormitory_buildings(id,dormitory_area_id,name,enabled) VALUES($1,$2,$3,$4)`, value.ID, value.DormitoryAreaID, value.Name, value.Enabled)
	case masterdata.KindTeachingBuilding:
		_, err = tx.Exec(ctx, `INSERT INTO teaching_buildings(id,name,enabled) VALUES($1,$2,$3)`, value.ID, value.Name, value.Enabled)
	case masterdata.KindMajor:
		_, err = tx.Exec(ctx, `INSERT INTO majors(id,faculty_id,name,enabled) VALUES($1,$2,$3,$4)`, value.ID, value.FacultyID, value.Name, value.Enabled)
	case masterdata.KindStaff:
		_, err = tx.Exec(ctx, `INSERT INTO staff(id,employee_no,name,gender,phone,faculty_id,enabled) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7)`, value.ID, value.Code, value.Name, value.Gender, value.Phone, value.FacultyID, value.Enabled)
	case masterdata.KindClass:
		_, err = tx.Exec(ctx, `INSERT INTO classes(id,name,grade_id,faculty_id,major_id,head_teacher_id,enrolled_size,enabled) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8)`, value.ID, value.Name, value.GradeID, value.FacultyID, value.MajorID, value.HeadTeacherID, value.EnrolledSize, value.Enabled)
	case masterdata.KindDormitory:
		_, err = tx.Exec(ctx, `INSERT INTO dormitories(id,room_no,dormitory_building_id,enabled) VALUES($1,$2,$3,$4)`, value.ID, value.RoomNo, value.DormitoryBuildingID, value.Enabled)
	case masterdata.KindClassroom:
		_, err = tx.Exec(ctx, `INSERT INTO classrooms(id,teaching_building_id,room_no,enabled) VALUES($1,$2,$3,$4)`, value.ID, value.TeachingBuildingID, value.RoomNo, value.Enabled)
	case masterdata.KindClassroomHygieneArea:
		_, err = tx.Exec(ctx, `INSERT INTO classroom_hygiene_areas(id,classroom_id,name,enabled) VALUES($1,$2,$3,$4)`, value.ID, value.ClassroomID, value.Name, value.Enabled)
	case masterdata.KindPublicDormitoryHygieneArea:
		_, err = tx.Exec(ctx, `INSERT INTO public_dormitory_hygiene_areas(id,dormitory_building_id,floor_unit,name,enabled) VALUES($1,$2,$3,$4,$5)`, value.ID, value.DormitoryBuildingID, value.FloorUnit, value.Name, value.Enabled)
	case masterdata.KindCourse:
		_, err = tx.Exec(ctx, `INSERT INTO courses(id,name,enabled) VALUES($1,$2,$3)`, value.ID, value.Name, value.Enabled)
	case masterdata.KindStudent:
		_, err = tx.Exec(ctx, `INSERT INTO students(id,student_no,name,gender,grade_id,faculty_id,major_id,class_id,dormitory_id,enrollment_status,enabled) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,$10,$11)`, value.ID, value.Code, value.Name, value.Gender, value.GradeID, value.FacultyID, value.MajorID, value.ClassID, value.DormitoryID, value.EnrollmentStatus, value.Enabled)
	default:
		return masterdata.ErrInvalidKind
	}
	return err
}

func updateMasterRecord(ctx context.Context, tx pgx.Tx, value masterdata.Record) error {
	var err error
	switch value.Kind {
	case masterdata.KindFaculty:
		_, err = tx.Exec(ctx, `UPDATE faculties SET name=$2,enabled=$3,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Enabled)
	case masterdata.KindGrade:
		_, err = tx.Exec(ctx, `UPDATE grades SET name=$2,enabled=$3,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Enabled)
	case masterdata.KindDormitoryArea:
		_, err = tx.Exec(ctx, `UPDATE dormitory_areas SET name=$2,enabled=$3,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Enabled)
	case masterdata.KindDormitoryBuilding:
		_, err = tx.Exec(ctx, `UPDATE dormitory_buildings SET name=$2,dormitory_area_id=$3,enabled=$4,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.DormitoryAreaID, value.Enabled)
	case masterdata.KindTeachingBuilding:
		_, err = tx.Exec(ctx, `UPDATE teaching_buildings SET name=$2,enabled=$3,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Enabled)
	case masterdata.KindMajor:
		_, err = tx.Exec(ctx, `UPDATE majors SET name=$2,faculty_id=$3,enabled=$4,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.FacultyID, value.Enabled)
	case masterdata.KindStaff:
		_, err = tx.Exec(ctx, `UPDATE staff SET name=$2,gender=$3,phone=$4,faculty_id=NULLIF($5,'')::uuid,enabled=$6,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Gender, value.Phone, value.FacultyID, value.Enabled)
	case masterdata.KindClass:
		_, err = tx.Exec(ctx, `UPDATE classes SET name=$2,grade_id=$3,faculty_id=NULLIF($4,'')::uuid,major_id=NULLIF($5,'')::uuid,head_teacher_id=NULLIF($6,'')::uuid,enrolled_size=$7,enabled=$8,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.GradeID, value.FacultyID, value.MajorID, value.HeadTeacherID, value.EnrolledSize, value.Enabled)
	case masterdata.KindDormitory:
		_, err = tx.Exec(ctx, `UPDATE dormitories SET room_no=$2,dormitory_building_id=$3,enabled=$4,updated_at=now() WHERE id=$1`, value.ID, value.RoomNo, value.DormitoryBuildingID, value.Enabled)
	case masterdata.KindClassroom:
		_, err = tx.Exec(ctx, `UPDATE classrooms SET teaching_building_id=$2,room_no=$3,enabled=$4,updated_at=now() WHERE id=$1`, value.ID, value.TeachingBuildingID, value.RoomNo, value.Enabled)
	case masterdata.KindClassroomHygieneArea:
		_, err = tx.Exec(ctx, `UPDATE classroom_hygiene_areas SET classroom_id=$2,name=$3,enabled=$4,updated_at=now() WHERE id=$1`, value.ID, value.ClassroomID, value.Name, value.Enabled)
	case masterdata.KindPublicDormitoryHygieneArea:
		_, err = tx.Exec(ctx, `UPDATE public_dormitory_hygiene_areas SET dormitory_building_id=$2,floor_unit=$3,name=$4,enabled=$5,updated_at=now() WHERE id=$1`, value.ID, value.DormitoryBuildingID, value.FloorUnit, value.Name, value.Enabled)
	case masterdata.KindCourse:
		_, err = tx.Exec(ctx, `UPDATE courses SET name=$2,enabled=$3,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Enabled)
	case masterdata.KindStudent:
		_, err = tx.Exec(ctx, `UPDATE students SET name=$2,gender=$3,grade_id=$4,faculty_id=NULLIF($5,'')::uuid,major_id=NULLIF($6,'')::uuid,class_id=NULLIF($7,'')::uuid,dormitory_id=NULLIF($8,'')::uuid,enrollment_status=$9,enabled=$10,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Gender, value.GradeID, value.FacultyID, value.MajorID, value.ClassID, value.DormitoryID, value.EnrollmentStatus, value.Enabled)
	default:
		return masterdata.ErrInvalidKind
	}
	return err
}

func insertMasterChange(ctx context.Context, tx pgx.Tx, actorID, kind, id, action string, before, after []byte) error {
	_, err := tx.Exec(ctx, `INSERT INTO master_data_changes(id,entity_type,entity_id,action,before_json,after_json,actor_id)
		VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid)`, uuid.NewString(), kind, id, action, before, after, actorID)
	return err
}

func (p *Postgres) ListDictionaryTypes(ctx context.Context) ([]dictionary.Type, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id,code,name,description,system,enabled FROM dictionary_types ORDER BY code`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []dictionary.Type
	for rows.Next() {
		var value dictionary.Type
		if err := rows.Scan(&value.ID, &value.Code, &value.Name, &value.Description, &value.System, &value.Enabled); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) ListDictionaryItems(ctx context.Context, typeID string) ([]dictionary.Item, error) {
	rows, err := p.Pool.Query(ctx, `SELECT id,type_id,value,label,description,sort_order,enabled FROM dictionary_items WHERE type_id=$1 ORDER BY sort_order,value`, typeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []dictionary.Item
	for rows.Next() {
		var value dictionary.Item
		if err := rows.Scan(&value.ID, &value.TypeID, &value.Value, &value.Label, &value.Description, &value.SortOrder, &value.Enabled); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) FindDictionaryType(ctx context.Context, id string) (dictionary.Type, error) {
	var value dictionary.Type
	err := p.Pool.QueryRow(ctx, `SELECT id,code,name,description,system,enabled FROM dictionary_types WHERE id=$1`, id).
		Scan(&value.ID, &value.Code, &value.Name, &value.Description, &value.System, &value.Enabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return value, dictionary.ErrNotFound
	}
	return value, err
}

func (p *Postgres) CreateDictionaryType(ctx context.Context, value dictionary.Type, mutation audit.Mutation) (dictionary.Type, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO dictionary_types(id,code,name,description,system,enabled) VALUES($1,$2,$3,$4,$5,$6)`, value.ID, value.Code, value.Name, value.Description, value.System, value.Enabled)
		return dictionaryError(execErr)
	})
	return value, dictionaryError(err)
}

func (p *Postgres) UpdateDictionaryType(ctx context.Context, value dictionary.Type, mutation audit.Mutation) (dictionary.Type, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE dictionary_types SET code=$2,name=$3,description=$4,enabled=$5,updated_at=now() WHERE id=$1`, value.ID, value.Code, value.Name, value.Description, value.Enabled)
		return dictionaryError(execErr)
	})
	return value, dictionaryError(err)
}

func (p *Postgres) CreateDictionaryItem(ctx context.Context, value dictionary.Item, mutation audit.Mutation) (dictionary.Item, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO dictionary_items(id,type_id,value,label,description,sort_order,enabled) VALUES($1,$2,$3,$4,$5,$6,$7)`, value.ID, value.TypeID, value.Value, value.Label, value.Description, value.SortOrder, value.Enabled)
		return dictionaryError(execErr)
	})
	return value, dictionaryError(err)
}

func (p *Postgres) UpdateDictionaryItem(ctx context.Context, value dictionary.Item, mutation audit.Mutation) (dictionary.Item, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE dictionary_items SET value=$3,label=$4,description=$5,sort_order=$6,enabled=$7,updated_at=now() WHERE id=$1 AND type_id=$2`, value.ID, value.TypeID, value.Value, value.Label, value.Description, value.SortOrder, value.Enabled)
		return dictionaryError(execErr)
	})
	return value, dictionaryError(err)
}

func dictionaryError(err error) error {
	var databaseError *pgconn.PgError
	if errors.As(err, &databaseError) && databaseError.Code == "23505" {
		return dictionary.ErrDuplicateValue
	}
	return err
}

func (p *Postgres) List(ctx context.Context) (academic.Calendar, error) {
	var result academic.Calendar
	yearRows, err := p.Pool.Query(ctx, `SELECT id,name,start_date,end_date,enabled FROM academic_years ORDER BY start_date DESC`)
	if err != nil {
		return result, err
	}
	for yearRows.Next() {
		var value academic.AcademicYear
		if err := yearRows.Scan(&value.ID, &value.Name, &value.StartDate, &value.EndDate, &value.Enabled); err != nil {
			yearRows.Close()
			return result, err
		}
		result.AcademicYears = append(result.AcademicYears, value)
	}
	yearRows.Close()
	semesterRows, err := p.Pool.Query(ctx, `SELECT id,academic_year_id,code,name,start_date,end_date,enabled FROM semesters ORDER BY start_date DESC`)
	if err != nil {
		return result, err
	}
	for semesterRows.Next() {
		var value academic.Semester
		if err := semesterRows.Scan(&value.ID, &value.AcademicYearID, &value.Code, &value.Name, &value.StartDate, &value.EndDate, &value.Enabled); err != nil {
			semesterRows.Close()
			return result, err
		}
		result.Semesters = append(result.Semesters, value)
	}
	semesterRows.Close()
	weekRows, err := p.Pool.Query(ctx, `SELECT id,semester_id,week_no,start_date,end_date,enabled,note,attendance_penalty_multiplier FROM semester_weeks ORDER BY start_date DESC`)
	if err != nil {
		return result, err
	}
	defer weekRows.Close()
	for weekRows.Next() {
		var value academic.Week
		if err := weekRows.Scan(&value.ID, &value.SemesterID, &value.WeekNo, &value.StartDate, &value.EndDate, &value.Enabled, &value.Note, &value.AttendancePenaltyMultiplier); err != nil {
			return result, err
		}
		result.Weeks = append(result.Weeks, value)
	}
	return result, weekRows.Err()
}

func (p *Postgres) CreateYear(ctx context.Context, value academic.AcademicYear, mutation audit.Mutation) (academic.AcademicYear, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO academic_years(id,name,start_date,end_date,enabled) VALUES($1,$2,$3,$4,$5)`, value.ID, value.Name, value.StartDate, value.EndDate, value.Enabled)
		return execErr
	})
	return value, err
}

func (p *Postgres) UpdateYear(ctx context.Context, value academic.AcademicYear, mutation audit.Mutation) (academic.AcademicYear, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE academic_years SET name=$2,start_date=$3,end_date=$4,enabled=$5 WHERE id=$1`, value.ID, value.Name, value.StartDate, value.EndDate, value.Enabled)
		return execErr
	})
	return value, err
}

func (p *Postgres) CreateSemester(ctx context.Context, value academic.Semester, mutation audit.Mutation) (academic.Semester, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO semesters(id,academic_year_id,code,name,start_date,end_date,enabled) VALUES($1,$2,$3,$4,$5,$6,$7)`, value.ID, value.AcademicYearID, value.Code, value.Name, value.StartDate, value.EndDate, value.Enabled)
		return execErr
	})
	return value, err
}

func (p *Postgres) UpdateSemester(ctx context.Context, value academic.Semester, mutation audit.Mutation) (academic.Semester, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if err := lockAcademicWeekWrites(ctx, tx); err != nil {
			return err
		}
		var hasOutOfRangeWeek bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM semester_weeks WHERE semester_id=$1 AND (start_date<$2 OR end_date>$3))`, value.ID, value.StartDate, value.EndDate).Scan(&hasOutOfRangeWeek); err != nil {
			return err
		}
		if hasOutOfRangeWeek {
			return academic.ErrInvalidInput
		}
		_, execErr := tx.Exec(ctx, `UPDATE semesters SET academic_year_id=$2,code=$3,name=$4,start_date=$5,end_date=$6,enabled=$7 WHERE id=$1`, value.ID, value.AcademicYearID, value.Code, value.Name, value.StartDate, value.EndDate, value.Enabled)
		return execErr
	})
	return value, err
}

func (p *Postgres) CreateWeek(ctx context.Context, value academic.Week, mutation audit.Mutation) (academic.Week, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if err := lockAcademicWeekWrites(ctx, tx); err != nil {
			return err
		}
		if err := validateAcademicWeekWrite(ctx, tx, value, ""); err != nil {
			return err
		}
		_, execErr := tx.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date,enabled,note,attendance_penalty_multiplier) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, value.ID, value.SemesterID, value.WeekNo, value.StartDate, value.EndDate, value.Enabled, value.Note, value.AttendancePenaltyMultiplier)
		return execErr
	})
	return value, err
}

func (p *Postgres) UpdateWeek(ctx context.Context, value academic.Week, mutation audit.Mutation) (academic.Week, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if err := lockAcademicWeekWrites(ctx, tx); err != nil {
			return err
		}
		if err := validateAcademicWeekWrite(ctx, tx, value, value.ID); err != nil {
			return err
		}
		_, execErr := tx.Exec(ctx, `UPDATE semester_weeks SET semester_id=$2,week_no=$3,start_date=$4,end_date=$5,enabled=$6,note=$7,attendance_penalty_multiplier=$8 WHERE id=$1`, value.ID, value.SemesterID, value.WeekNo, value.StartDate, value.EndDate, value.Enabled, value.Note, value.AttendancePenaltyMultiplier)
		return execErr
	})
	return value, err
}

func (p *Postgres) CurrentWeek(ctx context.Context, date time.Time) (*academic.Week, error) {
	snapshot, err := p.ReadWorkContext(ctx, date)
	if err != nil {
		return nil, err
	}
	return academic.ResolveWorkContext(snapshot, date).Week, nil
}

// ListAttendance applies every effective scope dimension in PostgreSQL before returning personal inspection data.
func (p *Postgres) ListAttendance(ctx context.Context, query inspection.AttendanceListQuery) (inspection.AttendancePage, error) {
	countSQL, countArgs, listSQL, listArgs, err := buildAttendanceListQueries(query)
	if err != nil {
		return inspection.AttendancePage{}, err
	}
	page := inspection.AttendancePage{Items: make([]inspection.AttendanceRecord, 0), Page: query.Page, PageSize: query.PageSize}
	if err := p.Pool.QueryRow(ctx, countSQL, countArgs...).Scan(&page.Total); err != nil {
		return inspection.AttendancePage{}, err
	}
	rows, err := p.Pool.Query(ctx, listSQL, listArgs...)
	if err != nil {
		return inspection.AttendancePage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var record inspection.AttendanceRecord
		if err := rows.Scan(
			&record.ID, &record.InspectionTypeID, &record.SemesterID, &record.WeekID, &record.DepartmentID,
			&record.StudentID, &record.StudentNoSnapshot, &record.StudentNameSnapshot, &record.ClassID,
			&record.ClassNameSnapshot, &record.GradeSnapshot, &record.CourseID, &record.CourseName,
			&record.ClassroomID, &record.ClassroomCode, &record.OccurredAt, &record.AbsenceHours, &record.PenaltyMultiplier, &record.PenaltyHours,
			&record.PeriodText, &record.Remark, &record.Source, &record.Status, &record.ImportBatchID,
			&record.DedupKey, &record.RowVersion, &record.CreatedBy, &record.CreatedAt, &record.UpdatedAt,
		); err != nil {
			return inspection.AttendancePage{}, err
		}
		page.Items = append(page.Items, record)
	}
	return page, rows.Err()
}

func buildAttendanceListQueries(query inspection.AttendanceListQuery) (string, []any, string, []any, error) {
	orderBy, err := attendanceOrderBy(query.Sort)
	if err != nil {
		return "", nil, "", nil, err
	}
	where := ` FROM inspection_records ir
		JOIN attendance_details ad ON ad.record_id = ir.id
		JOIN courses c ON c.id = ad.course_id
		LEFT JOIN classrooms cr ON cr.id = ad.classroom_id
		LEFT JOIN teaching_buildings tb ON tb.id = cr.teaching_building_id
		WHERE ir.semester_id = $1::uuid
		  AND ir.week_id = $2::uuid
		  AND ir.inspection_type_id = $3::uuid
		  AND (($4::text = '00000000-0000-0000-0000-000000000000' AND ir.department_id IS NULL)
		       OR ($4::text <> '' AND $4::text <> '00000000-0000-0000-0000-000000000000' AND ir.department_id = $4::uuid)
		       OR ($4::text = '' AND (cardinality($10::text[]) = 0 OR ir.department_id::text = ANY($10::text[])
		         OR (ir.department_id IS NULL AND '00000000-0000-0000-0000-000000000000'=ANY($10::text[])))))
		  AND ($5::text = '' OR ir.class_id = $5::uuid)
		  AND ($6::text = '' OR ir.grade_snapshot = $6)
		  AND ($7::text = '' OR ir.student_no_snapshot ILIKE '%' || $7 || '%'
		       OR ir.student_name_snapshot ILIKE '%' || $7 || '%')
		  AND ($8::text = '' OR ir.status = $8)
		  AND ($9::text = '' OR ir.source = $9)
		  AND ir.deleted_at IS NULL`
	countSQL := `SELECT count(*)` + where
	listSQL := `SELECT ir.id,ir.inspection_type_id,ir.semester_id,ir.week_id,COALESCE(ir.department_id::text,''),
		COALESCE(ir.student_id::text,''),ir.student_no_snapshot,ir.student_name_snapshot,
		COALESCE(ir.class_id::text,''),ir.class_name_snapshot,ir.grade_snapshot,
		ad.course_id,c.name,COALESCE(ad.classroom_id::text,''),COALESCE(tb.name || ' ' || cr.room_no,''),
		ir.occurred_at,ad.absence_hours,ad.penalty_multiplier,ad.penalty_hours,ad.period_text,ir.remark,ir.source,ir.status,
		COALESCE(ir.import_batch_id::text,''),ir.dedup_key,ir.row_version,ir.created_by,
		ir.created_at,ir.updated_at` + where + ` ORDER BY ` + orderBy + ` LIMIT $11 OFFSET $12`
	departmentIDs := query.DepartmentIDs
	if departmentIDs == nil {
		departmentIDs = make([]string, 0)
	}
	countArgs := []any{
		query.SemesterID, query.WeekID, query.InspectionTypeID, query.DepartmentID,
		query.ClassID, query.Grade, query.Keyword, query.Status, query.Source, departmentIDs,
	}
	listArgs := append(append([]any{}, countArgs...), query.PageSize, (query.Page-1)*query.PageSize)
	return countSQL, countArgs, listSQL, listArgs, nil
}

func attendanceOrderBy(sort string) (string, error) {
	switch sort {
	case inspection.SortOccurredAtDescending:
		return "ir.occurred_at DESC, ir.id DESC", nil
	case inspection.SortOccurredAtAscending:
		return "ir.occurred_at ASC, ir.id ASC", nil
	case inspection.SortStudentNoAscending:
		return "ir.student_no_snapshot ASC, ir.id ASC", nil
	case inspection.SortStudentNoDescending:
		return "ir.student_no_snapshot DESC, ir.id DESC", nil
	default:
		return "", inspection.ErrInvalidAttendanceQuery
	}
}

// LoadAttendanceReferences bulk-loads only the identifiers present in one authorized import batch.
func (p *Postgres) LoadAttendanceReferences(ctx context.Context, lookup importjob.AttendanceReferenceLookup) (importjob.AttendanceReferences, error) {
	result := importjob.AttendanceReferences{
		StudentsByNo: make(map[string]importjob.AttendanceStudentReference), ClassesByName: make(map[string]importjob.AttendanceClassReference),
		CoursesByName: make(map[string]importjob.AttendanceCourseReference), CoursesByID: make(map[string]importjob.AttendanceCourseReference),
		ClassroomsByCode: make(map[string]importjob.AttendanceClassroomReference),
	}
	if lookup.WeekID != "" {
		if err := p.Pool.QueryRow(ctx, `SELECT attendance_penalty_multiplier FROM semester_weeks WHERE id=$1::uuid AND semester_id=$2::uuid`, lookup.WeekID, lookup.SemesterID).Scan(&result.WeekPenaltyMultiplier); err != nil {
			return result, err
		}
	}
	studentRows, err := p.Pool.Query(ctx, `SELECT s.id,s.student_no,s.name,s.class_id,c.name,g.name
		FROM students s JOIN classes c ON c.id=s.class_id JOIN grades g ON g.id=c.grade_id
		WHERE s.enabled AND c.enabled AND g.enabled AND s.student_no = ANY($1::text[])
		  AND ($2='' OR c.id=NULLIF($2,'')::uuid) AND ($3='' OR g.name=$3)`, lookup.StudentNos, lookup.ClassID, lookup.Grade)
	if err != nil {
		return result, err
	}
	for studentRows.Next() {
		var value importjob.AttendanceStudentReference
		if err := studentRows.Scan(&value.ID, &value.StudentNo, &value.Name, &value.ClassID, &value.ClassName, &value.Grade); err != nil {
			studentRows.Close()
			return result, err
		}
		result.StudentsByNo[value.StudentNo] = value
	}
	if err := studentRows.Err(); err != nil {
		studentRows.Close()
		return result, err
	}
	studentRows.Close()

	classRows, err := p.Pool.Query(ctx, `SELECT c.id,c.name,g.name FROM classes c JOIN grades g ON g.id=c.grade_id
		WHERE c.enabled AND g.enabled AND c.name = ANY($1::text[])
		AND ($2='' OR c.id=NULLIF($2,'')::uuid) AND ($3='' OR g.name=$3)`, lookup.ClassNames, lookup.ClassID, lookup.Grade)
	if err != nil {
		return result, err
	}
	for classRows.Next() {
		var value importjob.AttendanceClassReference
		if err := classRows.Scan(&value.ID, &value.Name, &value.Grade); err != nil {
			classRows.Close()
			return result, err
		}
		result.ClassesByName[value.Name] = value
	}
	if err := classRows.Err(); err != nil {
		classRows.Close()
		return result, err
	}
	classRows.Close()

	courseRows, err := p.Pool.Query(ctx, `SELECT c.id,c.name FROM courses c
		WHERE c.enabled AND (c.name = ANY($1::text[]) OR c.id = ANY($2::uuid[])) FOR SHARE`, lookup.CourseNames, lookup.CourseIDs)
	if err != nil {
		return result, err
	}
	for courseRows.Next() {
		var value importjob.AttendanceCourseReference
		if err := courseRows.Scan(&value.ID, &value.Name); err != nil {
			courseRows.Close()
			return result, err
		}
		result.CoursesByName[value.Name] = value
		result.CoursesByID[value.ID] = value
	}
	if err := courseRows.Err(); err != nil {
		courseRows.Close()
		return result, err
	}
	courseRows.Close()

	classroomRows, err := p.Pool.Query(ctx, `SELECT classroom.id,building.name || ' ' || classroom.room_no
		FROM classrooms classroom JOIN teaching_buildings building ON building.id=classroom.teaching_building_id
		WHERE classroom.enabled AND building.enabled AND building.name || ' ' || classroom.room_no = ANY($1::text[])`, lookup.ClassroomCodes)
	if err != nil {
		return result, err
	}
	for classroomRows.Next() {
		var value importjob.AttendanceClassroomReference
		if err := classroomRows.Scan(&value.ID, &value.Code); err != nil {
			classroomRows.Close()
			return result, err
		}
		result.ClassroomsByCode[value.Code] = value
	}
	if err := classroomRows.Err(); err != nil {
		classroomRows.Close()
		return result, err
	}
	classroomRows.Close()
	return result, nil
}

// FindExistingAttendanceKeys checks duplicates inside the already-authorized import scope in one query.
func (p *Postgres) FindExistingAttendanceKeys(ctx context.Context, scope importjob.AttendanceImportScope, keys []string) (map[string]struct{}, error) {
	result := make(map[string]struct{})
	if len(keys) == 0 {
		return result, nil
	}
	rows, err := p.Pool.Query(ctx, `SELECT dedup_key FROM inspection_records
		WHERE semester_id=$1::uuid AND week_id=$2::uuid AND inspection_type_id=$3::uuid AND department_id IS NOT DISTINCT FROM NULLIF($4,'')::uuid
		  AND ($5='' OR class_id=NULLIF($5,'')::uuid) AND ($6='' OR grade_snapshot=$6)
		  AND dedup_key = ANY($7::text[]) AND deleted_at IS NULL AND ($8='' OR id<>NULLIF($8,'')::uuid)`,
		scope.SemesterID, scope.WeekID, scope.InspectionTypeID, scope.DepartmentID, scope.ClassID, scope.Grade, keys, scope.ExcludeRecordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			return nil, err
		}
		result[key] = struct{}{}
	}
	return result, rows.Err()
}

// FindImportBatch loads the batch together with its authoritative week boundaries.
func (p *Postgres) FindImportBatch(ctx context.Context, id string) (importjob.Batch, error) {
	return scanImportBatch(p.Pool.QueryRow(ctx, importBatchSelect+` WHERE b.id=$1`, id))
}

// CreateImportBatch persists the initial UPLOADED state after private storage has verified the source file.
func (p *Postgres) CreateImportBatch(ctx context.Context, batch importjob.Batch) error {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	// The same lock is acquired by compensation, preventing a batch from binding a discarded file.
	source, err := scanFile(tx.QueryRow(ctx, fileSelect+` WHERE id=$1 FOR UPDATE`, batch.OriginalFileID))
	if err != nil {
		return err
	}
	if source.Status != storage.StatusAvailable || source.RetentionType != storage.RetentionImportSource || batch.CreatedBy == "" || source.CreatedBy != batch.CreatedBy {
		return storage.ErrFileUnavailable
	}
	_, err = tx.Exec(ctx, `INSERT INTO import_batches(
		id,inspection_type_id,semester_id,week_id,department_id,class_id,grade,original_file_id,template_id,template_version,
		status,total_rows,valid_rows,invalid_rows,imported_rows,version,created_by,created_at,updated_at
	) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,$9,$10,$11,0,0,0,0,$12,$13,$14,$15)`,
		batch.ID, batch.InspectionTypeID, batch.SemesterID, batch.WeekID, batch.DepartmentID, batch.ClassID, batch.Grade, batch.OriginalFileID,
		batch.TemplateID, batch.TemplateVersion, batch.Status, batch.Version, batch.CreatedBy, batch.CreatedAt, batch.UpdatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (p *Postgres) FindPublishedImportTemplate(ctx context.Context, inspectionTypeID string) (importjob.Template, error) {
	var value importjob.Template
	err := p.Pool.QueryRow(ctx, `SELECT id,code,name,inspection_type_id,version_no,file_id,status FROM import_templates
		WHERE inspection_type_id=$1::uuid AND status='PUBLISHED'`, inspectionTypeID).Scan(
		&value.ID, &value.Code, &value.Name, &value.InspectionTypeID, &value.Version, &value.FileID, &value.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return importjob.Template{}, importjob.ErrTemplateNotFound
	}
	return value, err
}

func (p *Postgres) ListImportRows(ctx context.Context, batchID string, page, pageSize int, validationStatus string) (importjob.ImportRowPage, error) {
	result := importjob.ImportRowPage{Items: make([]importjob.ImportRow, 0), Page: page, PageSize: pageSize}
	if err := p.Pool.QueryRow(ctx, `SELECT count(*) FROM import_rows WHERE batch_id=$1 AND ($2='' OR validation_status=$2)`, batchID, validationStatus).Scan(&result.Total); err != nil {
		return importjob.ImportRowPage{}, err
	}
	rows, err := p.Pool.Query(ctx, `SELECT id,batch_id,row_no,raw_data_json,normalized_data_json,validation_status,errors_json,COALESCE(record_id::text,'')
		FROM import_rows WHERE batch_id=$1 AND ($2='' OR validation_status=$2) ORDER BY row_no LIMIT $3 OFFSET $4`, batchID, validationStatus, pageSize, (page-1)*pageSize)
	if err != nil {
		return importjob.ImportRowPage{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var value importjob.ImportRow
		var raw, normalized, validationErrors []byte
		if err := rows.Scan(&value.ID, &value.BatchID, &value.RowNo, &raw, &normalized, &value.ValidationStatus, &validationErrors, &value.RecordID); err != nil {
			return importjob.ImportRowPage{}, err
		}
		if err := json.Unmarshal(raw, &value.RawData); err != nil {
			return importjob.ImportRowPage{}, err
		}
		var typed importjob.InspectionImportRecord
		if err := json.Unmarshal(normalized, &typed); err != nil {
			return importjob.ImportRowPage{}, err
		}
		if typed.SchemaVersion != 0 {
			if typed.SchemaVersion != 1 {
				return importjob.ImportRowPage{}, importjob.ErrInvalidBatch
			}
			value.NormalizedInspection = &typed
		} else if err := json.Unmarshal(normalized, &value.NormalizedRecord); err != nil {
			return importjob.ImportRowPage{}, err
		}
		if err := json.Unmarshal(validationErrors, &value.Errors); err != nil {
			return importjob.ImportRowPage{}, err
		}
		result.Items = append(result.Items, value)
	}
	return result, rows.Err()
}

// UpdateImportBatchState applies an optimistic state transition.
func (p *Postgres) UpdateImportBatchState(ctx context.Context, id string, expectedVersion int, target string, failure importjob.BatchFailure) (importjob.Batch, error) {
	tag, err := p.Pool.Exec(ctx, `UPDATE import_batches SET status=$3::varchar,failure_code=$4,failure_message=$5,version=version+1,updated_at=now(),
		cancelled_at=CASE WHEN $3::varchar='CANCELLED' THEN now() ELSE cancelled_at END
		WHERE id=$1 AND version=$2`, id, expectedVersion, target, failure.Code, failure.Message)
	if err != nil {
		return importjob.Batch{}, err
	}
	if tag.RowsAffected() == 0 {
		return importjob.Batch{}, importjob.ErrBatchVersionConflict
	}
	return p.FindImportBatch(ctx, id)
}

// ReplaceImportPreview atomically replaces parsed rows and publishes their aggregate counts.
func (p *Postgres) ReplaceImportPreview(ctx context.Context, batch importjob.Batch, rows []importjob.ImportRow) (importjob.Batch, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return importjob.Batch{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `DELETE FROM import_rows WHERE batch_id=$1`, batch.ID); err != nil {
		return importjob.Batch{}, err
	}
	validRows, invalidRows := 0, 0
	for _, row := range rows {
		rawJSON, err := json.Marshal(row.RawData)
		if err != nil {
			return importjob.Batch{}, err
		}
		var normalized any = map[string]any{}
		if row.ValidationStatus == importjob.RowStatusValid {
			normalized = row.NormalizedRecord
			if row.NormalizedInspection != nil {
				normalized = row.NormalizedInspection
			}
			validRows++
		} else {
			invalidRows++
		}
		normalizedJSON, err := json.Marshal(normalized)
		if err != nil {
			return importjob.Batch{}, err
		}
		errorsJSON, err := json.Marshal(row.Errors)
		if err != nil {
			return importjob.Batch{}, err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO import_rows(
			id,batch_id,row_no,raw_data_json,normalized_data_json,validation_status,errors_json,record_id
		) VALUES($1,$2,$3,$4,$5,$6,$7,NULLIF($8,'')::uuid)`,
			row.ID, batch.ID, row.RowNo, rawJSON, normalizedJSON, row.ValidationStatus, errorsJSON, row.RecordID); err != nil {
			return importjob.Batch{}, err
		}
	}
	tag, err := tx.Exec(ctx, `UPDATE import_batches SET status='PREVIEW_READY',total_rows=$3,valid_rows=$4,invalid_rows=$5,
		failure_code='',failure_message='',version=version+1,updated_at=now()
		WHERE id=$1 AND version=$2 AND status='PARSING'`, batch.ID, batch.Version, len(rows), validRows, invalidRows)
	if err != nil {
		return importjob.Batch{}, err
	}
	if tag.RowsAffected() == 0 {
		return importjob.Batch{}, importjob.ErrBatchVersionConflict
	}
	if err := tx.Commit(ctx); err != nil {
		return importjob.Batch{}, err
	}
	batch.Status = importjob.BatchStatusPreviewReady
	batch.TotalRows, batch.ValidRows, batch.InvalidRows = len(rows), validRows, invalidRows
	batch.Version++
	batch.FailureCode, batch.FailureMessage = "", ""
	return batch, nil
}

const importBatchSelect = `SELECT b.id,b.inspection_type_id,b.semester_id,b.week_id,COALESCE(b.department_id::text,''),COALESCE(b.class_id::text,''),b.grade,b.original_file_id,
	b.template_id,b.template_version,b.status,b.total_rows,b.valid_rows,b.invalid_rows,b.imported_rows,b.version,
	COALESCE(b.confirm_idempotency_key,''),b.failure_code,b.failure_message,b.created_by,b.created_at,b.updated_at,
	COALESCE(b.confirmed_at,'epoch'::timestamptz),COALESCE(b.cancelled_at,'epoch'::timestamptz),w.start_date,w.end_date,it.code,t.code
	FROM import_batches b JOIN semester_weeks w ON w.id=b.week_id
	JOIN inspection_types it ON it.id=b.inspection_type_id JOIN import_templates t ON t.id=b.template_id`

type importBatchScanner interface {
	Scan(...any) error
}

func scanImportBatch(row importBatchScanner) (importjob.Batch, error) {
	var batch importjob.Batch
	err := row.Scan(
		&batch.ID, &batch.InspectionTypeID, &batch.SemesterID, &batch.WeekID, &batch.DepartmentID, &batch.ClassID, &batch.Grade, &batch.OriginalFileID,
		&batch.TemplateID, &batch.TemplateVersion, &batch.Status, &batch.TotalRows, &batch.ValidRows, &batch.InvalidRows,
		&batch.ImportedRows, &batch.Version, &batch.ConfirmIdempotencyKey, &batch.FailureCode, &batch.FailureMessage,
		&batch.CreatedBy, &batch.CreatedAt, &batch.UpdatedAt, &batch.ConfirmedAt, &batch.CancelledAt, &batch.WeekStart, &batch.WeekEnd, &batch.TypeCode, &batch.TemplateCode,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return importjob.Batch{}, importjob.ErrBatchNotFound
	}
	if err != nil {
		return importjob.Batch{}, err
	}
	return batch, nil
}

// ConfirmAttendanceImport locks, validates and persists one import confirmation and its audit atomically.
func (p *Postgres) ConfirmAttendanceImport(ctx context.Context, input importjob.ConfirmBatchInput, mutation audit.Mutation) (importjob.ConfirmResult, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return importjob.ConfirmResult{}, err
	}
	defer tx.Rollback(ctx)
	var status, idempotencyKey string
	var totalRows, validRows, invalidRows, importedRows, version, templateVersion int
	err = tx.QueryRow(ctx, `SELECT status,total_rows,valid_rows,invalid_rows,imported_rows,version,
		COALESCE(confirm_idempotency_key,''),template_version
		FROM import_batches WHERE id=$1 AND semester_id=$2::uuid AND week_id=$3::uuid
		  AND inspection_type_id=$4::uuid AND department_id IS NOT DISTINCT FROM NULLIF($5,'')::uuid
		  AND COALESCE(class_id::text,'')=$6 AND grade=$7 AND created_by=$8::uuid FOR UPDATE`,
		input.BatchID, input.Scope.SemesterID, input.Scope.WeekID, input.Scope.InspectionTypeID, input.Scope.DepartmentID, input.Scope.ClassID, input.Scope.Grade, input.Scope.ActorID,
	).Scan(&status, &totalRows, &validRows, &invalidRows, &importedRows, &version, &idempotencyKey, &templateVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return importjob.ConfirmResult{}, importjob.ErrBatchScopeMismatch
	}
	if err != nil {
		return importjob.ConfirmResult{}, err
	}
	if status == importjob.BatchStatusConfirmed {
		if idempotencyKey == input.IdempotencyKey {
			return importjob.ConfirmResult{BatchID: input.BatchID, Status: status, ImportedRows: importedRows, Version: version}, nil
		}
		return importjob.ConfirmResult{}, importjob.ErrBatchAlreadyConfirmed
	}
	if version != input.ExpectedVersion {
		return importjob.ConfirmResult{}, importjob.ErrBatchVersionConflict
	}
	if status != importjob.BatchStatusPreviewReady {
		return importjob.ConfirmResult{}, importjob.ErrInvalidBatchTransition
	}
	if totalRows <= 0 || invalidRows != 0 || validRows != totalRows {
		return importjob.ConfirmResult{}, importjob.ErrBatchHasErrors
	}
	rows, err := tx.Query(ctx, `SELECT id,normalized_data_json FROM import_rows
		WHERE batch_id=$1 AND validation_status='VALID' ORDER BY row_no FOR UPDATE`, input.BatchID)
	if err != nil {
		return importjob.ConfirmResult{}, err
	}
	type confirmedRow struct {
		id       string
		record   inspection.AttendanceRecord
		typed    *importjob.InspectionImportRecord
		prepared inspection.InspectionRecord
	}
	confirmedRows := make([]confirmedRow, 0, validRows)
	for rows.Next() {
		var row confirmedRow
		var normalized []byte
		if err := rows.Scan(&row.id, &normalized); err != nil {
			rows.Close()
			return importjob.ConfirmResult{}, err
		}
		var typed importjob.InspectionImportRecord
		if err := json.Unmarshal(normalized, &typed); err != nil {
			rows.Close()
			return importjob.ConfirmResult{}, fmt.Errorf("decode import schema: %w", err)
		}
		if typed.SchemaVersion != 0 {
			if !inspectionImportMatches(typed, input) {
				rows.Close()
				return importjob.ConfirmResult{}, importjob.ErrBatchScopeMismatch
			}
			row.typed = &typed
		} else {
			if err := json.Unmarshal(normalized, &row.record); err != nil {
				rows.Close()
				return importjob.ConfirmResult{}, fmt.Errorf("decode normalized import row: %w", err)
			}
			row.record.DedupKey = inspection.AttendanceDedupKey(row.record)
			if !confirmationRecordMatches(row.record, input) {
				rows.Close()
				return importjob.ConfirmResult{}, importjob.ErrBatchScopeMismatch
			}
		}
		confirmedRows = append(confirmedRows, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return importjob.ConfirmResult{}, err
	}
	rows.Close()
	if len(confirmedRows) != validRows {
		return importjob.ConfirmResult{}, importjob.ErrBatchHasErrors
	}
	lockedContext, err := lockInspectionImportContext(ctx, tx, input.Scope)
	if err != nil {
		return importjob.ConfirmResult{}, err
	}
	if lockedContext.TypeCode == inspection.InspectionTypeAttendance && lockedContext.AttendancePenaltyMultiplier == nil {
		return importjob.ConfirmResult{}, inspection.ErrAttendancePenaltyUnconfigured
	}
	for i := range confirmedRows {
		row := &confirmedRows[i]
		if row.typed != nil {
			row.prepared, err = prepareInspectionImport(ctx, tx, lockedContext, *row.typed, input)
			if err != nil {
				return importjob.ConfirmResult{}, err
			}
		} else if err := lockCourseImportReferences(ctx, tx, lockedContext, row.record); err != nil {
			return importjob.ConfirmResult{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE import_batches SET status='CONFIRMING',updated_at=now() WHERE id=$1`, input.BatchID); err != nil {
		return importjob.ConfirmResult{}, err
	}
	for _, row := range confirmedRows {
		if row.typed != nil {
			if _, err := (&inspectionTx{tx: tx}).Save(ctx, row.prepared, nil, inspection.MutationAccess{}); err != nil {
				return importjob.ConfirmResult{}, err
			}
			if _, err := tx.Exec(ctx, `UPDATE import_rows SET record_id=$2 WHERE id=$1 AND batch_id=$3`, row.id, row.prepared.ID, input.BatchID); err != nil {
				return importjob.ConfirmResult{}, err
			}
			continue
		}
		record := row.record
		if _, err := tx.Exec(ctx, `INSERT INTO inspection_records(
			id,inspection_type_id,semester_id,week_id,department_id,student_id,student_no_snapshot,student_name_snapshot,
			class_id,class_name_snapshot,grade_snapshot,occurred_at,violation_count,remark,source,status,import_batch_id,dedup_key,row_version,created_by
		) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,NULLIF($9,'')::uuid,$10,$11,$12,1,$13,$14,$15,$16,$17,$18,$19)`,
			record.ID, record.InspectionTypeID, record.SemesterID, record.WeekID, record.DepartmentID, record.StudentID,
			record.StudentNoSnapshot, record.StudentNameSnapshot, record.ClassID, record.ClassNameSnapshot, record.GradeSnapshot,
			record.OccurredAt, record.Remark, record.Source, record.Status, input.BatchID, record.DedupKey, record.RowVersion, record.CreatedBy); err != nil {
			if errors.Is(inspectionDBError(err), inspection.ErrAttendanceDuplicate) {
				return importjob.ConfirmResult{}, inspection.ErrAttendanceDuplicate
			}
			return importjob.ConfirmResult{}, err
		}
		var multiplier *int
		if lockedContext.TypeCode == inspection.InspectionTypeAttendance {
			multiplier = lockedContext.AttendancePenaltyMultiplier
		}
		if _, err := tx.Exec(ctx, `INSERT INTO attendance_details(record_id,course_id,absence_hours,period_text,classroom_id,penalty_multiplier)
			VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,$6)`, record.ID, record.CourseID, record.AbsenceHours, record.PeriodText, record.ClassroomID, multiplier); err != nil {
			return importjob.ConfirmResult{}, err
		}
		if _, err := tx.Exec(ctx, `UPDATE import_rows SET record_id=$2 WHERE id=$1 AND batch_id=$3`, row.id, record.ID, input.BatchID); err != nil {
			return importjob.ConfirmResult{}, err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE import_batches SET status='CONFIRMED',imported_rows=$2,confirm_idempotency_key=$3,
		version=version+1,confirmed_at=now(),updated_at=now(),failure_code='',failure_message='' WHERE id=$1`,
		input.BatchID, len(confirmedRows), input.IdempotencyKey); err != nil {
		return importjob.ConfirmResult{}, err
	}
	if err := insertAuditTx(ctx, tx, mutation.Metadata, confirmationAuditEntry(input, templateVersion, len(confirmedRows), version)); err != nil {
		return importjob.ConfirmResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return importjob.ConfirmResult{}, err
	}
	return importjob.ConfirmResult{BatchID: input.BatchID, Status: importjob.BatchStatusConfirmed, ImportedRows: len(confirmedRows), Version: version + 1}, nil
}

func confirmationAuditEntry(input importjob.ConfirmBatchInput, templateVersion, importedRows, previousVersion int) audit.Entry {
	return audit.Entry{
		Action: audit.ActionImportBatchConfirmed, ResourceType: "import_batch", ResourceID: input.BatchID,
		Before: map[string]any{"status": importjob.BatchStatusPreviewReady, "version": previousVersion},
		After: map[string]any{
			"status": importjob.BatchStatusConfirmed, "importedRows": importedRows, "templateVersion": templateVersion,
			"semesterId": input.Scope.SemesterID, "weekId": input.Scope.WeekID,
			"inspectionTypeId": input.Scope.InspectionTypeID, "departmentId": input.Scope.DepartmentID,
		},
	}
}

func confirmationRecordMatches(record inspection.AttendanceRecord, input importjob.ConfirmBatchInput) bool {
	withinClass := input.Scope.ClassID == "" || record.ClassID == input.Scope.ClassID
	withinGrade := input.Scope.Grade == "" || record.GradeSnapshot == input.Scope.Grade
	return withinClass && withinGrade && record.ID != "" && record.InspectionTypeID == input.Scope.InspectionTypeID &&
		record.SemesterID == input.Scope.SemesterID && record.WeekID == input.Scope.WeekID &&
		record.DepartmentID == input.Scope.DepartmentID && record.ImportBatchID == input.BatchID &&
		record.CreatedBy == input.Scope.ActorID && record.Source == inspection.RecordSourceExcel &&
		record.Status == inspection.RecordStatusDraft && record.DedupKey != "" && record.RowVersion == 1
}
