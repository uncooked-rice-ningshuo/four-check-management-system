package academic

import (
	"context"
	"errors"
	"four-check-management-system/server/internal/audit"
	"github.com/google/uuid"
	"strings"
	"time"
	"unicode/utf8"
)

var ErrWorkContextForbidden = errors.New("work context forbidden")
var ErrWorkContextConflict = errors.New("work context version conflict")

// WorkContextSnapshot includes only enabled calendar matches and the singleton override version.
type WorkContextSnapshot struct {
	Automatic *Week
	Override  *Week
	ExpiresAt *time.Time
	Version   int
}

// WorkContext is the resolved system default; explicit record scopes remain unchanged.
type WorkContext struct {
	Week      *Week      `json:"week"`
	Source    string     `json:"source"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty"`
	Version   int        `json:"version"`
}

// WeekOverrideInput clears the override when WeekID is empty. Expiry is always explicit for activation.
type WeekOverrideInput struct {
	WeekID    string     `json:"weekId"`
	ExpiresAt *time.Time `json:"expiresAt"`
	Version   int        `json:"version"`
	Reason    string     `json:"reason"`
}

// WorkContextRepository loads consistent snapshots and saves audited versioned settings.
type WorkContextRepository interface {
	ReadWorkContext(context.Context, time.Time) (WorkContextSnapshot, error)
	SaveWeekOverride(context.Context, WeekOverrideInput, time.Time, audit.Metadata) error
}

// WorkContextService applies the existing calendar administrator boundary.
type WorkContextService struct{ repository WorkContextRepository }

// NewWorkContextService uses caller-supplied time to keep expiry deterministic.
func NewWorkContextService(r WorkContextRepository) *WorkContextService {
	return &WorkContextService{r}
}

// ResolveWorkContext falls back to the calendar immediately at expiry or if the target is disabled.
func ResolveWorkContext(s WorkContextSnapshot, now time.Time) WorkContext {
	r := WorkContext{Week: s.Automatic, Source: "CALENDAR", Version: s.Version}
	if s.Override != nil && s.ExpiresAt != nil && now.Before(*s.ExpiresAt) {
		r.Week = s.Override
		r.Source = "OVERRIDE"
		r.ExpiresAt = s.ExpiresAt
	}
	if r.Week == nil {
		r.Source = "NONE"
	}
	return r
}

// Get resolves a read-only context without modifying expired settings.
func (s *WorkContextService) Get(ctx context.Context, now time.Time) (WorkContext, error) {
	snapshot, e := s.repository.ReadWorkContext(ctx, now)
	if e != nil {
		return WorkContext{}, e
	}
	return ResolveWorkContext(snapshot, now), nil
}

// Set validates activation or clearing before the repository locks and audits the setting.
func (s *WorkContextService) Set(ctx context.Context, isSuperAdmin bool, i WeekOverrideInput, now time.Time, m audit.Metadata) (WorkContext, error) {
	if !isSuperAdmin {
		return WorkContext{}, ErrWorkContextForbidden
	}
	if i.Version < 1 || strings.TrimSpace(i.Reason) == "" || utf8.RuneCountInString(i.Reason) > 255 || strings.ContainsRune(i.Reason, 0) {
		return WorkContext{}, ErrInvalidInput
	}
	if i.WeekID != "" {
		id, e := uuid.Parse(i.WeekID)
		if e != nil || id == uuid.Nil || i.ExpiresAt == nil || !i.ExpiresAt.After(now) {
			return WorkContext{}, ErrInvalidInput
		}
		i.WeekID = id.String()
	} else if i.ExpiresAt != nil {
		return WorkContext{}, ErrInvalidInput
	}
	m.Reason = strings.TrimSpace(i.Reason)
	if e := s.repository.SaveWeekOverride(ctx, i, now, m); e != nil {
		return WorkContext{}, e
	}
	return s.Get(ctx, now)
}
