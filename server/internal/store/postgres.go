package store

import (
	"context"
	"errors"
	"time"

	"four-check-management-system/server/internal/auth"
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

// Create 持久化账号，并将数据库唯一约束冲突转换为稳定的领域错误。
func (p *Postgres) Create(ctx context.Context, a auth.Account) error {
	_, err := p.Pool.Exec(ctx, `INSERT INTO accounts(id,student_no,name,password_hash,status,created_at) VALUES($1,$2,$3,$4,$5,$6)`, a.ID, a.StudentNo, a.Name, a.PasswordHash, a.Status, a.CreatedAt)
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return auth.ErrStudentNoExists
	}
	return err
}

// FindByStudentNo 按学号读取账号，用于登录凭证校验。
func (p *Postgres) FindByStudentNo(ctx context.Context, no string) (auth.Account, error) {
	return scanAccount(p.Pool.QueryRow(ctx, `SELECT id,student_no,name,password_hash,status,created_at FROM accounts WHERE student_no=$1`, no))
}

// FindByID 读取账号实时状态，用于令牌校验和账号禁用后的会话失效。
func (p *Postgres) FindByID(ctx context.Context, id string) (auth.Account, error) {
	return scanAccount(p.Pool.QueryRow(ctx, `SELECT id,student_no,name,password_hash,status,created_at FROM accounts WHERE id=$1`, id))
}

type row interface{ Scan(...any) error }

func scanAccount(r row) (auth.Account, error) {
	var a auth.Account
	err := r.Scan(&a.ID, &a.StudentNo, &a.Name, &a.PasswordHash, &a.Status, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, auth.ErrAccountNotFound
	}
	return a, err
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
