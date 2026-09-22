package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestDualQuotaLimitsAndUpgrade(t *testing.T) {
	now := time.Now()
	start := now.Add(-time.Hour)
	short, week := 10.0, 50.0
	g := &Group{QuotaPolicy: QuotaPolicyDualWindow, ShortLimitUSD: &short, WeeklyLimitUSD: &week}
	sub := &UserSubscription{Status: SubscriptionStatusActive, ExpiresAt: now.Add(30 * 24 * time.Hour), ShortWindowStart: &start, WeeklyWindowStart: &start, ShortUsageUSD: 8, WeeklyUsageUSD: 40, DailyUsageUSD: 999, MonthlyUsageUSD: 999}
	require.NoError(t, sub.checkDualLimits(g, now, 0))
	sub.ShortUsageUSD = 10
	require.ErrorIs(t, sub.checkDualLimits(g, now, 0), ErrShortLimitExceeded)
	short = 20
	week = 100
	require.NoError(t, sub.checkDualLimits(g, now, 0))
	require.Equal(t, 50.0, remainingPercent(sub.ShortUsageUSD, short))
	require.Equal(t, 60.0, remainingPercent(sub.WeeklyUsageUSD, week))
	require.Equal(t, start, *sub.ShortWindowStart)
	sub.WeeklyUsageUSD = 100
	require.ErrorIs(t, sub.checkDualLimits(g, now, 0), ErrWeeklyLimitExceeded)
	require.NoError(t, sub.checkDualLimits(g, now.Add(7*24*time.Hour), 0))
}

func TestDualQuotaBoundariesAndDisplay(t *testing.T) {
	now := time.Now()
	start := now.Add(-5 * time.Hour)
	limit := 10.0
	g := &Group{QuotaPolicy: QuotaPolicyDualWindow, ShortLimitUSD: &limit, WeeklyLimitUSD: &limit}
	sub := &UserSubscription{Status: SubscriptionStatusActive, ExpiresAt: now.Add(24 * time.Hour), ShortWindowStart: &start, WeeklyWindowStart: &now, ShortUsageUSD: 12}
	require.NoError(t, sub.checkDualLimits(g, now, 0))
	require.ErrorIs(t, sub.checkDualLimits(g, now.Add(24*time.Hour), 0), ErrSubscriptionExpired)
	require.Equal(t, 0.0, remainingPercent(12, 10))
	require.InDelta(t, 0.01, remainingPercent(9.999, 10), 0.0001)
	require.Nil(t, quotaWindow(0, 10, &now, 7*24*time.Hour, sub.ExpiresAt).ResetsAt)
	g.ShortLimitUSD = nil
	require.ErrorIs(t, sub.checkDualLimits(g, now, 0), ErrSubscriptionInvalid)
}

func TestDualQuotaUpgradeValidation(t *testing.T) {
	a, b, c, d := 10.0, 50.0, 20.0, 100.0
	from := &Group{QuotaPolicy: QuotaPolicyDualWindow, ShortLimitUSD: &a, WeeklyLimitUSD: &b}
	to := &Group{QuotaPolicy: QuotaPolicyDualWindow, ShortLimitUSD: &c, WeeklyLimitUSD: &d}
	require.NoError(t, validateQuotaUpgrade(from, to))
	require.Error(t, validateQuotaUpgrade(to, from))
	require.Error(t, validateQuotaUpgrade(from, &Group{}))
}
func TestDualQuotaReadOnlyProjection(t *testing.T) {
	now := time.Now()
	start := now.Add(-11 * time.Hour)
	sub := &UserSubscription{ExpiresAt: now.Add(24 * time.Hour), ShortWindowStart: &start, WeeklyWindowStart: &start, ShortUsageUSD: 99, WeeklyUsageUSD: 8}
	projectDualWindows(sub, now)
	require.Zero(t, sub.ShortUsageUSD)
	require.Equal(t, 8.0, sub.WeeklyUsageUSD)
	require.Equal(t, start.Add(10*time.Hour), *sub.ShortWindowStart)
	require.Equal(t, start, *sub.WeeklyWindowStart)
}

func TestDualQuotaExpiredRenewalStartsFreshTerm(t *testing.T) {
	now := time.Now()
	anchor := now.Add(-time.Hour)
	limit := 10.0
	old := &UserSubscription{Group: &Group{QuotaPolicy: QuotaPolicyDualWindow, ShortLimitUSD: &limit, WeeklyLimitUSD: &limit}, ShortWindowStart: &anchor, WeeklyWindowStart: &anchor, ShortUsageUSD: 10, WeeklyUsageUSD: 10, ExpiresAt: now.Add(-time.Minute)}
	renewed := renewedSubscriptionTerm(old, "", now, now.Add(30*24*time.Hour))
	require.Zero(t, renewed.ShortUsageUSD)
	require.Zero(t, renewed.WeeklyUsageUSD)
	require.Equal(t, now, *renewed.ShortWindowStart)
	require.Equal(t, 10.0, old.ShortUsageUSD)
}

// Legacy window tests now need an explicit policy lookup dependency.
type legacyQuotaGroupRepo struct{ groupRepoNoop }

func (legacyQuotaGroupRepo) GetByID(_ context.Context, id int64) (*Group, error) {
	return &Group{ID: id, QuotaPolicy: "legacy"}, nil
}
