package service

import (
	"context"
	"errors"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/pkg/errors"
	"math"
	"time"
)

var ErrShortLimitExceeded = infraerrors.TooManyRequests("SHORT_LIMIT_EXCEEDED", "5-hour usage limit exceeded")

// DualWindowRepository is an optional capability so legacy repositories keep
// their existing contract. Selecting the new policy without it fails closed.
type DualWindowRepository interface {
	MaintainDualWindows(context.Context, int64, time.Time, bool) (*UserSubscription, error)
	ResetDualWindows(context.Context, *WeeklyResetInput) error
}

func maintainDualWindows(ctx context.Context, repo UserSubscriptionRepository, id int64, now time.Time, activate bool) (*UserSubscription, error) {
	r, ok := repo.(DualWindowRepository)
	if !ok {
		return nil, errors.New("dual-window repository unavailable")
	}
	return r.MaintainDualWindows(ctx, id, now, activate)
}

func (s *UserSubscription) checkDualLimits(g *Group, now time.Time, additional float64) error {
	if !g.ValidDualLimits() {
		return ErrSubscriptionInvalid
	}
	if s.Status != SubscriptionStatusActive || !now.Before(s.ExpiresAt) {
		return ErrSubscriptionExpired
	}
	short, week := s.ShortUsageUSD, s.WeeklyUsageUSD
	if s.ShortWindowStart == nil || !now.Before(s.ShortWindowStart.Add(5*time.Hour)) {
		short = 0
	}
	if s.WeeklyWindowStart == nil || !now.Before(s.WeeklyWindowStart.Add(7*24*time.Hour)) {
		week = 0
	}
	if short >= *g.ShortLimitUSD || short+additional > *g.ShortLimitUSD {
		return ErrShortLimitExceeded
	}
	if week >= *g.WeeklyLimitUSD || week+additional > *g.WeeklyLimitUSD {
		return ErrWeeklyLimitExceeded
	}
	return nil
}

func remainingPercent(used, limit float64) float64 {
	if limit <= 0 || math.IsNaN(used) || math.IsNaN(limit) || math.IsInf(used, 0) || math.IsInf(limit, 0) {
		return 0
	}
	return math.Max(0, math.Min(100, (limit-used)/limit*100))
}

type SubscriptionQuotaWindow struct {
	RemainingPercent float64    `json:"remaining_percent"`
	StartsAt         *time.Time `json:"starts_at"`
	ResetsAt         *time.Time `json:"resets_at"`
	Exhausted        bool       `json:"exhausted"`
}

func quotaWindow(used, limit float64, start *time.Time, duration time.Duration, expires time.Time) *SubscriptionQuotaWindow {
	w := &SubscriptionQuotaWindow{RemainingPercent: remainingPercent(used, limit), StartsAt: start, Exhausted: used >= limit}
	if start != nil {
		end := start.Add(duration)
		if end.Before(expires) {
			w.ResetsAt = &end
		}
	}
	return w
}

func (s *SubscriptionService) subscriptionGroup(ctx context.Context, sub *UserSubscription) (*Group, error) {
	if sub.Group != nil {
		return sub.Group, nil
	}
	if s.groupRepo != nil {
		return s.groupRepo.GetByID(ctx, sub.GroupID)
	}
	return sub.Group, nil
}

// projectDualWindows gives read-only views the same effective quota as admission.
func projectDualWindows(sub *UserSubscription, now time.Time) {
	if !now.Before(sub.ExpiresAt) {
		now = sub.ExpiresAt.Add(-time.Microsecond)
	}
	advance := func(start **time.Time, used *float64, period time.Duration) {
		if *start == nil {
			*used = 0
			return
		}
		if elapsed := now.Sub(**start); elapsed >= period {
			next := (*start).Add((elapsed / period) * period)
			*start = &next
			*used = 0
		}
	}
	advance(&sub.ShortWindowStart, &sub.ShortUsageUSD, 5*time.Hour)
	advance(&sub.WeeklyWindowStart, &sub.WeeklyUsageUSD, 7*24*time.Hour)
}
