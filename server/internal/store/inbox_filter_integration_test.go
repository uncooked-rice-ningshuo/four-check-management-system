package store

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"four-check-management-system/server/internal/inbox"
	"github.com/google/uuid"
)

// TestInboxFilterSortAndBatchPostgres 在一次性 PostgreSQL 上验证本人消息的日期/类别/排序/分页，
// 以及批量已读的越权原子失败与重复请求幂等。需要 TEST_DATABASE_URL 指向已迁移的空库。
func TestInboxFilterSortAndBatchPostgres(t *testing.T) {
	db := openTestPostgres(t)
	ctx := context.Background()
	owner, other := uuid.New(), uuid.New()
	for _, id := range []uuid.UUID{owner, other} {
		if _, err := db.Pool.Exec(ctx, `INSERT INTO accounts(id,student_no,login_name,name,password_hash,status,system_role) VALUES($1,$2,$2,'测试收件人','test-only','ACTIVE','MEMBER')`, id, "ib"+id.String()[:8]); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_, _ = db.Pool.Exec(ctx, `DELETE FROM accounts WHERE id = ANY($1::uuid[])`, []uuid.UUID{owner, other})
	})

	beijing := time.FixedZone("Asia/Shanghai", 8*60*60)
	insert := func(account uuid.UUID, category string, at time.Time, body string) int64 {
		t.Helper()
		var id int64
		if err := db.Pool.QueryRow(ctx, `INSERT INTO account_notifications(account_id,category,body,created_at) VALUES($1,$2,$3,$4) RETURNING id`, account, category, body, at).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	// 北京时间 11-21、11-23 早、11-23 晚，两条同一时刻用于验证稳定 tie-break。
	idEarly := insert(owner, "organization", time.Date(2026, 11, 21, 9, 0, 0, 0, beijing), "较早消息")
	idA := insert(owner, "permission", time.Date(2026, 11, 23, 9, 0, 0, 0, beijing), "同刻甲")
	idB := insert(owner, "organization", time.Date(2026, 11, 23, 9, 0, 0, 0, beijing), "同刻乙")
	foreign := insert(other, "permission", time.Date(2026, 11, 23, 9, 0, 0, 0, beijing), "他人消息")

	page := func(filter inbox.PageFilter) struct {
		Items []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
			Read bool   `json:"read"`
		} `json:"items"`
		Total   int    `json:"total"`
		Unread  int    `json:"unread"`
		Through string `json:"through"`
	} {
		t.Helper()
		raw, err := db.InboxPageFiltered(ctx, owner.String(), filter)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Items []struct {
				ID   string `json:"id"`
				Body string `json:"body"`
				Read bool   `json:"read"`
			} `json:"items"`
			Total   int    `json:"total"`
			Unread  int    `json:"unread"`
			Through string `json:"through"`
		}
		if err := json.Unmarshal(raw, &result); err != nil {
			t.Fatal(err)
		}
		return result
	}

	t.Run("日期按北京时间含止日", func(t *testing.T) {
		result := page(inbox.PageFilter{Page: 1, Size: 20, From: "2026-11-23", To: "2026-11-23"})
		if result.Total != 2 {
			t.Fatalf("止日应包含当天全部消息，得到 total=%d", result.Total)
		}
		if result.Unread != 3 || result.Through == "" {
			t.Fatalf("未读数与冻结边界应不受筛选影响：%d %q", result.Unread, result.Through)
		}
		for _, item := range result.Items {
			if item.Body == "较早消息" {
				t.Fatal("日期范围外的消息被返回")
			}
		}
	})

	t.Run("类别筛选与服务端排序稳定", func(t *testing.T) {
		result := page(inbox.PageFilter{Page: 1, Size: 20, Category: "permission", Sort: "newest"})
		if result.Total != 1 || result.Items[0].ID != strconv.FormatInt(idA, 10) {
			t.Fatalf("类别筛选错误：%+v", result)
		}
		newest := page(inbox.PageFilter{Page: 1, Size: 20, Sort: "newest"})
		oldest := page(inbox.PageFilter{Page: 1, Size: 20, Sort: "oldest"})
		if newest.Items[0].ID != strconv.FormatInt(idB, 10) || oldest.Items[0].ID != strconv.FormatInt(idEarly, 10) {
			t.Fatalf("排序错误 newest=%+v oldest=%+v", newest.Items, oldest.Items)
		}
		// 同一时刻按 id 兜底， newest 应为 idB,idA，oldest 应为 idEarly,idA,idB。
		if len(newest.Items) != 3 || newest.Items[1].ID != strconv.FormatInt(idA, 10) {
			t.Fatalf("tie-break 不稳定：%+v", newest.Items)
		}
	})

	t.Run("先过滤排序后分页", func(t *testing.T) {
		first := page(inbox.PageFilter{Page: 1, Size: 2, Sort: "oldest"})
		second := page(inbox.PageFilter{Page: 2, Size: 2, Sort: "oldest"})
		if len(first.Items) != 2 || len(second.Items) != 1 || first.Items[1].ID == second.Items[0].ID {
			t.Fatalf("分页应稳定无重叠：%+v %+v", first.Items, second.Items)
		}
	})

	t.Run("批量已读限定本人且原子失败", func(t *testing.T) {
		found, changed, err := db.ReadInboxBatch(ctx, owner.String(), []int64{idEarly, foreign})
		if err != nil || found || changed != 0 {
			t.Fatalf("混入他人编号应原子失败：found=%v changed=%d err=%v", found, changed, err)
		}
		if page(inbox.PageFilter{Page: 1, Size: 20}).Unread != 3 {
			t.Fatal("原子失败后不应有任何消息被标为已读")
		}
		found, changed, err = db.ReadInboxBatch(ctx, owner.String(), []int64{idEarly, idA})
		if err != nil || !found || changed != 2 {
			t.Fatalf("批量标已读失败：found=%v changed=%d err=%v", found, changed, err)
		}
		found, changed, err = db.ReadInboxBatch(ctx, owner.String(), []int64{idEarly, idA})
		if err != nil || !found || changed != 0 {
			t.Fatalf("重复请求应幂等：found=%v changed=%d err=%v", found, changed, err)
		}
		if page(inbox.PageFilter{Page: 1, Size: 20}).Unread != 1 {
			t.Fatal("批量已读后未读数应只剩一条")
		}
		// 未读优先排序应把剩余未读排在最前。
		unreadFirst := page(inbox.PageFilter{Page: 1, Size: 20, Sort: "unread-first"})
		if unreadFirst.Items[0].Read || !unreadFirst.Items[1].Read {
			t.Fatalf("未读优先排序错误：%+v", unreadFirst.Items)
		}
	})
}
