package store

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"four-check-management-system/server/internal/inbox"
	"github.com/jackc/pgx/v5"
)

// InboxPage 保留旧签名，等价于默认排序的 PageFiltered 查询。
func (p *Postgres) InboxPage(ctx context.Context, account string, page, size int, unread bool, category string) (json.RawMessage, error) {
	return p.InboxPageFiltered(ctx, account, inbox.PageFilter{Page: page, Size: size, Unread: unread, Category: category})
}

// InboxPageFiltered 在数据库内依次完成范围过滤、稳定排序和分页；未读数与冻结边界不受筛选影响。
func (p *Postgres) InboxPageFiltered(ctx context.Context, account string, filter inbox.PageFilter) (json.RawMessage, error) {
	from, to, ok := inbox.DateBounds(filter.From, filter.To)
	if !ok {
		return nil, inbox.ErrInvalid
	}
	order := "created_at DESC, id DESC"
	switch filter.Sort {
	case "oldest":
		order = "created_at ASC, id ASC"
	case "unread-first":
		order = "(read_at IS NULL) DESC, created_at DESC, id DESC"
	case "read-first":
		order = "(read_at IS NOT NULL) DESC, created_at DESC, id DESC"
	}
	sort := filter.Sort
	if sort == "" {
		sort = "newest"
	}
	var result json.RawMessage
	err := p.Pool.QueryRow(ctx, `WITH mine AS (SELECT * FROM account_notifications WHERE account_id=$1),
 filtered AS (SELECT * FROM mine WHERE (NOT $2 OR read_at IS NULL) AND ($3='' OR category=$3)
   AND ($4::timestamptz IS NULL OR created_at >= $4)
   AND ($5::timestamptz IS NULL OR created_at < $5)),
 slice AS (SELECT * FROM filtered ORDER BY `+order+` LIMIT $6 OFFSET $7)
 SELECT jsonb_build_object('items',COALESCE((SELECT jsonb_agg(jsonb_build_object('id',id::text,'category',category,'action',permission_action,'body',body,'caseId',discipline_case_id,'eventStatus',discipline_event_status,'relatedDeferredIds',related_deferred_ids,'createdAt',created_at,'read',read_at IS NOT NULL) ORDER BY `+order+`) FROM slice),'[]'),
 'total',(SELECT count(*) FROM filtered),'unread',(SELECT count(*) FROM mine WHERE read_at IS NULL),
 'through',COALESCE((SELECT max(id)::text FROM mine),'0'),'page',$8::int,'pageSize',$9::int,'sort',$10::text)`,
		account, filter.Unread, filter.Category, nullableTime(from), nullableTime(to),
		filter.Size, (filter.Page-1)*filter.Size, filter.Page, filter.Size, sort,
	).Scan(&result)
	return result, err
}

// nullableTime 把零值时间转成 NULL，让同一条 SQL 支持“不限日期”。
func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

// ReadInbox 保留旧签名，供既有集成测试使用。
func (p *Postgres) ReadInbox(ctx context.Context, account string, id int64, all bool) (bool, error) {
	found, _, err := p.ReadInboxDetailed(ctx, account, id, all)
	return found, err
}

// ReadInboxDetailed 单条或冻结边界内的全局已读。重复请求命中已读消息时 changed 为 0 但仍视为成功。
func (p *Postgres) ReadInboxDetailed(ctx context.Context, account string, id int64, all bool) (bool, int64, error) {
	if all {
		tag, err := p.Pool.Exec(ctx, `UPDATE account_notifications SET read_at=now() WHERE account_id=$1 AND id<=$2 AND read_at IS NULL`, account, id)
		return true, tag.RowsAffected(), err
	}
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // 提交后回滚是无害的；错误由调用方按返回值处理。
	var changed int64
	err = tx.QueryRow(ctx, `SELECT CASE WHEN read_at IS NULL THEN 1 ELSE 0 END FROM account_notifications WHERE account_id=$1 AND id=$2 FOR UPDATE`, account, id).Scan(&changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, 0, nil
	}
	if err != nil {
		return false, 0, err
	}
	if _, err := tx.Exec(ctx, `UPDATE account_notifications SET read_at=now() WHERE account_id=$1 AND id=$2 AND read_at IS NULL`, account, id); err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return true, changed, nil
}

// ReadInboxBatch 只修改当前账号的消息；先锁定并核对全部编号，混入他人编号则整批回滚。
func (p *Postgres) ReadInboxBatch(ctx context.Context, account string, ids []int64) (bool, int64, error) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback(ctx) //nolint:errcheck // 同上。
	rows, err := tx.Query(ctx, `SELECT read_at IS NULL FROM account_notifications WHERE account_id=$1 AND id = ANY($2::bigint[]) FOR UPDATE`, account, ids)
	if err != nil {
		return false, 0, err
	}
	var matched, changed int64
	for rows.Next() {
		var unread bool
		if err := rows.Scan(&unread); err != nil {
			rows.Close()
			return false, 0, err
		}
		matched++
		if unread {
			changed++
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return false, 0, err
	}
	rows.Close()
	if matched != int64(len(ids)) {
		return false, 0, nil
	}
	if _, err := tx.Exec(ctx, `UPDATE account_notifications SET read_at=now() WHERE account_id=$1 AND id = ANY($2::bigint[]) AND read_at IS NULL`, account, ids); err != nil {
		return false, 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, 0, err
	}
	return true, changed, nil
}
