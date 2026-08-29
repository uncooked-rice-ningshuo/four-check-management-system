package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"four-check-management-system/server/internal/academic"
	"four-check-management-system/server/internal/audit"
	"four-check-management-system/server/internal/auth"
	"four-check-management-system/server/internal/dictionary"
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

// Ready verifies that PostgreSQL can serve a new request.
func (p *Postgres) Ready(ctx context.Context) error { return p.Pool.Ping(ctx) }

// withAudit 在同一 PostgreSQL 事务内执行一次业务变更并追加审计记录。
// action 和 before/after 值由应用服务决定；持久化层只负责脱敏、原子性与追加写。
func (p *Postgres) withAudit(ctx context.Context, metadata audit.Metadata, entry audit.Entry, mutation func(pgx.Tx) error) error {
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

	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := mutation(tx); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_logs(
		id,actor_id,action,resource_type,resource_id,before_json,after_json,reason,request_id,ip,user_agent,created_at
	) VALUES($1,NULLIF($2,'')::uuid,$3,$4,$5,$6,$7,$8,$9,NULLIF($10,'')::inet,$11,$12)`,
		entry.ID, metadata.ActorID, string(entry.Action), entry.ResourceType, entry.ResourceID,
		beforeJSON, afterJSON, metadata.Reason, metadata.RequestID, metadata.IP, metadata.UserAgent, entry.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
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

// ListFileReconciliationCandidates selects only stale recoverable rows; it never enumerates MinIO.
func (p *Postgres) ListFileReconciliationCandidates(ctx context.Context, staleBefore time.Time, limit int) ([]storage.File, error) {
	rows, err := p.Pool.Query(ctx, fileSelect+` WHERE status IN ('PENDING','FAILED') AND updated_at < $1 ORDER BY updated_at,id LIMIT $2`, staleBefore, limit)
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
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE accounts SET login_name=$2,student_no=$3,name=$4,system_role=$5,
			session_version=session_version+1,updated_at=now() WHERE id=$1 AND NOT is_super_admin`, account.ID, account.LoginName, account.StudentNo, account.Name, account.SystemRole)
		if execErr == nil && tag.RowsAffected() == 0 {
			return auth.ErrAccountNotFound
		}
		return execErr
	})
	if err != nil {
		return auth.Account{}, err
	}
	return p.FindByID(ctx, account.ID)
}

func (p *Postgres) SetStatus(ctx context.Context, accountID, status string, mutation audit.Mutation) (auth.Account, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE accounts SET status=$2,session_version=session_version+1,updated_at=now()
			WHERE id=$1 AND NOT is_super_admin`, accountID, status)
		if execErr == nil && tag.RowsAffected() == 0 {
			return auth.ErrAccountNotFound
		}
		return execErr
	})
	if err != nil {
		return auth.Account{}, err
	}
	return p.FindByID(ctx, accountID)
}

func (p *Postgres) UpdatePassword(ctx context.Context, accountID, passwordHash string, mustChange bool, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		tag, execErr := tx.Exec(ctx, `UPDATE accounts SET password_hash=$2,must_change_password=$3,session_version=session_version+1,updated_at=now() WHERE id=$1`, accountID, passwordHash, mustChange)
		if execErr == nil && tag.RowsAffected() == 0 {
			return auth.ErrAccountNotFound
		}
		return execErr
	})
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

// Save 保存已经哈希处理的刷新会话，不持久化原始刷新令牌。
func (p *Postgres) Save(ctx context.Context, s auth.RefreshSession) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO refresh_sessions(id,account_id,token_hash,expires_at) VALUES($1,$2,$3,$4)`, s.ID, s.AccountID, s.TokenHash, s.ExpiresAt)
	return err
}

// Consume 以原子方式撤销并返回一条有效刷新会话。
func (p *Postgres) Consume(ctx context.Context, hash string, now time.Time) (auth.RefreshSession, error) {
	// 行锁保证“检查有效性—撤销令牌”属于同一原子操作；如果没有 FOR UPDATE，
	// 两个并发刷新请求可能同时通过校验并创建两条有效会话。
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return auth.RefreshSession{}, err
	}
	defer tx.Rollback(ctx)
	var s auth.RefreshSession
	err = tx.QueryRow(ctx, `SELECT id,account_id,token_hash,expires_at,revoked_at FROM refresh_sessions WHERE token_hash=$1 FOR UPDATE`, hash).Scan(&s.ID, &s.AccountID, &s.TokenHash, &s.ExpiresAt, &s.RevokedAt)
	if err != nil || s.RevokedAt != nil || !s.ExpiresAt.After(now) {
		return auth.RefreshSession{}, auth.ErrInvalidRefreshToken
	}
	if _, err = tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=$2 WHERE token_hash=$1`, hash, now); err != nil {
		return auth.RefreshSession{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return auth.RefreshSession{}, err
	}
	return s, nil
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
		COALESCE(a.primary_department_id::text,''),COALESCE(u.organization_role_id::text,'')
		FROM accounts a LEFT JOIN user_org_roles u ON u.account_id=a.id ORDER BY a.created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []organization.UserAssignment
	for rows.Next() {
		var value organization.UserAssignment
		if err := rows.Scan(&value.AccountID, &value.LoginName, &value.StudentNo, &value.Name, &value.Status, &value.SystemRole,
			&value.IsSuperAdmin, &value.MustChangePassword, &value.DepartmentID, &value.OrganizationRoleID); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (p *Postgres) AssignUserRole(ctx context.Context, value organization.UserAssignment, mutation audit.Mutation) error {
	return p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE accounts SET primary_department_id=$2,session_version=session_version+1,updated_at=now() WHERE id=$1`, value.AccountID, value.DepartmentID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM user_org_roles WHERE account_id=$1`, value.AccountID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO user_org_roles(id,account_id,department_id,organization_role_id) VALUES($1,$2,$3,$4)`, uuid.NewString(), value.AccountID, value.DepartmentID, value.OrganizationRoleID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE refresh_sessions SET revoked_at=COALESCE(revoked_at,now()) WHERE account_id=$1 AND revoked_at IS NULL`, value.AccountID)
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
	selectSQL, table, codeColumn, nameColumn, ok := masterRecordSelect(query.Kind)
	if !ok {
		return masterdata.Page{}, masterdata.ErrInvalidKind
	}
	var enabled any
	if query.Enabled != nil {
		enabled = *query.Enabled
	}
	filter := fmt.Sprintf(` WHERE ($1='' OR %s ILIKE '%%'||$1||'%%' OR %s ILIKE '%%'||$1||'%%') AND ($2::boolean IS NULL OR enabled=$2)`, codeColumn, nameColumn)
	var total int
	if err := p.Pool.QueryRow(ctx, `SELECT count(*) FROM `+table+filter, query.Keyword, enabled).Scan(&total); err != nil {
		return masterdata.Page{}, err
	}
	rows, err := p.Pool.Query(ctx, selectSQL+filter+` ORDER BY `+codeColumn+` LIMIT $3 OFFSET $4`, query.Keyword, enabled, query.PageSize, (query.Page-1)*query.PageSize)
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

func (p *Postgres) FindMasterData(ctx context.Context, kind, id string) (masterdata.Record, error) {
	selectSQL, _, _, _, ok := masterRecordSelect(kind)
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
	_, table, _, _, ok := masterRecordSelect(kind)
	if !ok {
		return false, masterdata.ErrInvalidKind
	}
	var exists bool
	err := p.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM `+table+` WHERE id=$1 AND enabled)`, id).Scan(&exists)
	return exists, err
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
		return masterdata.Record{}, err
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
		return masterdata.Record{}, err
	}
	return p.FindMasterData(ctx, value.Kind, value.ID)
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

func masterRecordSelect(kind string) (string, string, string, string, bool) {
	const empty = `''::text`
	switch kind {
	case masterdata.KindFaculty:
		return `SELECT id,code,name,enabled,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,created_at,updated_at FROM faculties`, "faculties", "code", "name", true
	case masterdata.KindMajor:
		return `SELECT id,code,name,enabled,faculty_id::text,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,created_at,updated_at FROM majors`, "majors", "code", "name", true
	case masterdata.KindStaff:
		return `SELECT id,employee_no,name,enabled,COALESCE(faculty_id::text,''),` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,gender,phone,` + empty + `,0,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,created_at,updated_at FROM staff`, "staff", "employee_no", "name", true
	case masterdata.KindClass:
		return `SELECT id,code,name,enabled,COALESCE(faculty_id::text,''),COALESCE(major_id::text,''),COALESCE(head_teacher_id::text,''),` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,grade,enrolled_size,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,created_at,updated_at FROM classes`, "classes", "code", "name", true
	case masterdata.KindDormitory:
		return `SELECT id,code,code,enabled,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,campus_area,` + empty + `,building_no,room_no,0,` + empty + `,` + empty + `,created_at,updated_at FROM dormitories`, "dormitories", "code", "code", true
	case masterdata.KindClassroom:
		return `SELECT id,code,code,enabled,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,building,` + empty + `,room_no,floor,` + empty + `,` + empty + `,created_at,updated_at FROM classrooms`, "classrooms", "code", "code", true
	case masterdata.KindCourse:
		return `SELECT id,code,name,enabled,` + empty + `,` + empty + `,` + empty + `,COALESCE(teacher_id::text,''),COALESCE(class_id::text,''),` + empty + `,COALESCE(classroom_id::text,''),` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,schedule_text,` + empty + `,created_at,updated_at FROM courses`, "courses", "code", "name", true
	case masterdata.KindStudent:
		return `SELECT id,student_no,name,enabled,COALESCE(faculty_id::text,''),COALESCE(major_id::text,''),` + empty + `,` + empty + `,COALESCE(class_id::text,''),COALESCE(dormitory_id::text,''),` + empty + `,gender,` + empty + `,grade,0,` + empty + `,` + empty + `,` + empty + `,` + empty + `,0,` + empty + `,enrollment_status,created_at,updated_at FROM students`, "students", "student_no", "name", true
	default:
		return "", "", "", "", false
	}
}

func scanMasterRecord(r row, kind string) (masterdata.Record, error) {
	value := masterdata.Record{Kind: kind}
	err := r.Scan(&value.ID, &value.Code, &value.Name, &value.Enabled, &value.FacultyID, &value.MajorID,
		&value.HeadTeacherID, &value.TeacherID, &value.ClassID, &value.DormitoryID, &value.ClassroomID, &value.Gender,
		&value.Phone, &value.Grade, &value.EnrolledSize, &value.CampusArea, &value.Building, &value.BuildingNo,
		&value.RoomNo, &value.Floor, &value.ScheduleText, &value.EnrollmentStatus, &value.CreatedAt, &value.UpdatedAt)
	return value, err
}

func insertMasterRecord(ctx context.Context, tx pgx.Tx, value masterdata.Record) error {
	var err error
	switch value.Kind {
	case masterdata.KindFaculty:
		_, err = tx.Exec(ctx, `INSERT INTO faculties(id,code,name,enabled) VALUES($1,$2,$3,$4)`, value.ID, value.Code, value.Name, value.Enabled)
	case masterdata.KindMajor:
		_, err = tx.Exec(ctx, `INSERT INTO majors(id,faculty_id,code,name,enabled) VALUES($1,$2,$3,$4,$5)`, value.ID, value.FacultyID, value.Code, value.Name, value.Enabled)
	case masterdata.KindStaff:
		_, err = tx.Exec(ctx, `INSERT INTO staff(id,employee_no,name,gender,phone,faculty_id,enabled) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,$7)`, value.ID, value.Code, value.Name, value.Gender, value.Phone, value.FacultyID, value.Enabled)
	case masterdata.KindClass:
		_, err = tx.Exec(ctx, `INSERT INTO classes(id,code,name,grade,faculty_id,major_id,head_teacher_id,enrolled_size,enabled) VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,$8,$9)`, value.ID, value.Code, value.Name, value.Grade, value.FacultyID, value.MajorID, value.HeadTeacherID, value.EnrolledSize, value.Enabled)
	case masterdata.KindDormitory:
		_, err = tx.Exec(ctx, `INSERT INTO dormitories(id,code,campus_area,building_no,room_no,enabled) VALUES($1,$2,$3,$4,$5,$6)`, value.ID, value.Code, value.CampusArea, value.BuildingNo, value.RoomNo, value.Enabled)
	case masterdata.KindClassroom:
		_, err = tx.Exec(ctx, `INSERT INTO classrooms(id,code,building,room_no,floor,enabled) VALUES($1,$2,$3,$4,$5,$6)`, value.ID, value.Code, value.Building, value.RoomNo, value.Floor, value.Enabled)
	case masterdata.KindCourse:
		_, err = tx.Exec(ctx, `INSERT INTO courses(id,code,name,teacher_id,class_id,classroom_id,schedule_text,enabled) VALUES($1,$2,$3,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8)`, value.ID, value.Code, value.Name, value.TeacherID, value.ClassID, value.ClassroomID, value.ScheduleText, value.Enabled)
	case masterdata.KindStudent:
		_, err = tx.Exec(ctx, `INSERT INTO students(id,student_no,name,gender,grade,faculty_id,major_id,class_id,dormitory_id,enrollment_status,enabled) VALUES($1,$2,$3,$4,$5,NULLIF($6,'')::uuid,NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,$10,$11)`, value.ID, value.Code, value.Name, value.Gender, value.Grade, value.FacultyID, value.MajorID, value.ClassID, value.DormitoryID, value.EnrollmentStatus, value.Enabled)
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
	case masterdata.KindMajor:
		_, err = tx.Exec(ctx, `UPDATE majors SET name=$2,faculty_id=$3,enabled=$4,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.FacultyID, value.Enabled)
	case masterdata.KindStaff:
		_, err = tx.Exec(ctx, `UPDATE staff SET name=$2,gender=$3,phone=$4,faculty_id=NULLIF($5,'')::uuid,enabled=$6,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Gender, value.Phone, value.FacultyID, value.Enabled)
	case masterdata.KindClass:
		_, err = tx.Exec(ctx, `UPDATE classes SET name=$2,grade=$3,faculty_id=NULLIF($4,'')::uuid,major_id=NULLIF($5,'')::uuid,head_teacher_id=NULLIF($6,'')::uuid,enrolled_size=$7,enabled=$8,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Grade, value.FacultyID, value.MajorID, value.HeadTeacherID, value.EnrolledSize, value.Enabled)
	case masterdata.KindDormitory:
		_, err = tx.Exec(ctx, `UPDATE dormitories SET campus_area=$2,building_no=$3,room_no=$4,enabled=$5,updated_at=now() WHERE id=$1`, value.ID, value.CampusArea, value.BuildingNo, value.RoomNo, value.Enabled)
	case masterdata.KindClassroom:
		_, err = tx.Exec(ctx, `UPDATE classrooms SET building=$2,room_no=$3,floor=$4,enabled=$5,updated_at=now() WHERE id=$1`, value.ID, value.Building, value.RoomNo, value.Floor, value.Enabled)
	case masterdata.KindCourse:
		_, err = tx.Exec(ctx, `UPDATE courses SET name=$2,teacher_id=NULLIF($3,'')::uuid,class_id=NULLIF($4,'')::uuid,classroom_id=NULLIF($5,'')::uuid,schedule_text=$6,enabled=$7,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.TeacherID, value.ClassID, value.ClassroomID, value.ScheduleText, value.Enabled)
	case masterdata.KindStudent:
		_, err = tx.Exec(ctx, `UPDATE students SET name=$2,gender=$3,grade=$4,faculty_id=NULLIF($5,'')::uuid,major_id=NULLIF($6,'')::uuid,class_id=NULLIF($7,'')::uuid,dormitory_id=NULLIF($8,'')::uuid,enrollment_status=$9,enabled=$10,updated_at=now() WHERE id=$1`, value.ID, value.Name, value.Gender, value.Grade, value.FacultyID, value.MajorID, value.ClassID, value.DormitoryID, value.EnrollmentStatus, value.Enabled)
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
	weekRows, err := p.Pool.Query(ctx, `SELECT id,semester_id,week_no,start_date,end_date,enabled,note FROM semester_weeks ORDER BY start_date DESC`)
	if err != nil {
		return result, err
	}
	defer weekRows.Close()
	for weekRows.Next() {
		var value academic.Week
		if err := weekRows.Scan(&value.ID, &value.SemesterID, &value.WeekNo, &value.StartDate, &value.EndDate, &value.Enabled, &value.Note); err != nil {
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
		_, execErr := tx.Exec(ctx, `UPDATE semesters SET academic_year_id=$2,code=$3,name=$4,start_date=$5,end_date=$6,enabled=$7 WHERE id=$1`, value.ID, value.AcademicYearID, value.Code, value.Name, value.StartDate, value.EndDate, value.Enabled)
		return execErr
	})
	return value, err
}

func (p *Postgres) CreateWeek(ctx context.Context, value academic.Week, mutation audit.Mutation) (academic.Week, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `INSERT INTO semester_weeks(id,semester_id,week_no,start_date,end_date,enabled,note) VALUES($1,$2,$3,$4,$5,$6,$7)`, value.ID, value.SemesterID, value.WeekNo, value.StartDate, value.EndDate, value.Enabled, value.Note)
		return execErr
	})
	return value, err
}

func (p *Postgres) UpdateWeek(ctx context.Context, value academic.Week, mutation audit.Mutation) (academic.Week, error) {
	err := p.withAudit(ctx, mutation.Metadata, mutation.Entry, func(tx pgx.Tx) error {
		_, execErr := tx.Exec(ctx, `UPDATE semester_weeks SET semester_id=$2,week_no=$3,start_date=$4,end_date=$5,enabled=$6,note=$7 WHERE id=$1`, value.ID, value.SemesterID, value.WeekNo, value.StartDate, value.EndDate, value.Enabled, value.Note)
		return execErr
	})
	return value, err
}

func (p *Postgres) CurrentWeek(ctx context.Context, date time.Time) (*academic.Week, error) {
	var value academic.Week
	err := p.Pool.QueryRow(ctx, `SELECT id,semester_id,week_no,start_date,end_date,enabled,note FROM semester_weeks WHERE enabled AND $1::date BETWEEN start_date AND end_date ORDER BY start_date LIMIT 1`, date).Scan(&value.ID, &value.SemesterID, &value.WeekNo, &value.StartDate, &value.EndDate, &value.Enabled, &value.Note)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &value, nil
}
