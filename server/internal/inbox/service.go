package inbox

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

// Beijing 是消息日期筛选使用的固定时区。日期是北京时间日历日，与IANA 时区数据库无关，
// 固定偏移可以避免部署环境缺少 tzdata 时把“含止日”算成 UTC 日界。
var Beijing = time.FixedZone("Asia/Shanghai", 8*60*60)

// PageFilter 是本人消息列表的查询条件。日期为空表示不限；非空日期为北京时间 YYYY-MM-DD。
type PageFilter struct {
	Page     int
	Size     int
	Unread   bool
	Category string
	From     string
	To       string
	Sort     string
}

// DateBounds 把北京时间起止日转换为 UTC 半开区间 [from, to)，使止日按北京时间整天包含。
func DateBounds(from, to string) (time.Time, time.Time, bool) {
	if from == "" && to == "" {
		return time.Time{}, time.Time{}, true
	}
	parse := func(value string) (time.Time, bool) {
		if value == "" {
			return time.Time{}, false
		}
		t, err := time.ParseInLocation("2006-01-02", value, Beijing)
		return t, err == nil
	}
	var start, end time.Time
	if from != "" {
		var ok bool
		if start, ok = parse(from); !ok {
			return time.Time{}, time.Time{}, false
		}
	}
	if to != "" {
		var ok bool
		if end, ok = parse(to); !ok {
			return time.Time{}, time.Time{}, false
		}
		end = end.AddDate(0, 0, 1)
	}
	if !start.IsZero() && !end.IsZero() && start.After(end.AddDate(0, 0, -1)) {
		return time.Time{}, time.Time{}, false
	}
	return start.UTC(), end.UTC(), true
}

// Repository scopes every operation to the authenticated recipient.
type Repository interface {
	InboxPage(context.Context, string, int, int, bool, string) (json.RawMessage, error)
	InboxPageFiltered(context.Context, string, PageFilter) (json.RawMessage, error)
	ReadInbox(context.Context, string, int64, bool) (bool, error)
	ReadInboxDetailed(context.Context, string, int64, bool) (bool, int64, error)
	ReadInboxBatch(context.Context, string, []int64) (bool, int64, error)
}
type Service struct{ repo Repository }

func New(repo Repository) *Service { return &Service{repo} }

var ErrInvalid = errors.New("invalid inbox filter")

// Page 保留旧签名，供既有集成测试与非筛选调用使用。
func (s *Service) Page(ctx context.Context, account string, page, size int, unread bool, category string) (json.RawMessage, error) {
	return s.PageFiltered(ctx, account, PageFilter{Page: page, Size: size, Unread: unread, Category: category})
}

// PageFiltered 校验筛选条件后下推仓储；排序与分页都在数据库内完成，稳定次序由 id 兜底。
func (s *Service) PageFiltered(ctx context.Context, account string, filter PageFilter) (json.RawMessage, error) {
	if filter.Page < 1 || filter.Page > 1000000 || filter.Size < 1 || filter.Size > 100 {
		return nil, ErrInvalid
	}
	if filter.Category != "" && filter.Category != "organization" && filter.Category != "permission" && filter.Category != "discipline" {
		return nil, ErrInvalid
	}
	switch filter.Sort {
	case "", "newest", "oldest", "unread-first", "read-first":
	default:
		return nil, ErrInvalid
	}
	if _, _, ok := DateBounds(filter.From, filter.To); !ok {
		return nil, ErrInvalid
	}
	return s.repo.InboxPageFiltered(ctx, account, filter)
}

// Read 保留旧签名，供既有集成测试使用。
func (s *Service) Read(ctx context.Context, account string, id int64, all bool) (bool, error) {
	found, _, err := s.ReadDetailed(ctx, account, id, all)
	return found, err
}

// ReadDetailed 返回是否命中本人消息，以及本次真正从“未读”变为“已读”的条数。
func (s *Service) ReadDetailed(ctx context.Context, account string, id int64, all bool) (bool, int64, error) {
	return s.repo.ReadInboxDetailed(ctx, account, id, all)
}

// ReadBatch 批量标记本人消息已读。混入他人编号时原子失败，重复请求保持幂等。
func (s *Service) ReadBatch(ctx context.Context, account string, ids []int64) (bool, int64, error) {
	if len(ids) == 0 || len(ids) > 200 {
		return false, 0, ErrInvalid
	}
	seen := make(map[int64]struct{}, len(ids))
	unique := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id < 1 {
			return false, 0, ErrInvalid
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return s.repo.ReadInboxBatch(ctx, account, unique)
}
