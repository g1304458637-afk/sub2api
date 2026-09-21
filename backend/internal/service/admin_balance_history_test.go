package service

import (
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/pkg/pagination"
	"github.com/stretchr/testify/require"
)

func TestMergeBalanceHistoryCodesIncludesAffiliateTransfersByDefault(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	older := now.Add(-2 * time.Hour)
	newer := now.Add(time.Hour)

	usedBy := int64(10)
	redeemCodes := []RedeemCode{
		{
			ID:        1,
			Type:      RedeemTypeBalance,
			Value:     8,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &now,
			CreatedAt: now,
		},
		{
			ID:        2,
			Type:      RedeemTypeConcurrency,
			Value:     1,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &older,
			CreatedAt: older,
		},
	}
	affiliateCodes := []RedeemCode{
		{
			ID:        -20,
			Type:      RedeemTypeAffiliateBalance,
			Value:     3.5,
			Status:    StatusUsed,
			UsedBy:    &usedBy,
			UsedAt:    &newer,
			CreatedAt: newer,
		},
	}

	got := mergeBalanceHistoryCodes(redeemCodes, affiliateCodes, pagination.PaginationParams{
		Page:     1,
		PageSize: 2,
	})

	require.Len(t, got, 2)
	require.Equal(t, RedeemTypeAffiliateBalance, got[0].Type)
	require.Equal(t, RedeemTypeBalance, got[1].Type)
}

func TestMergeBalanceHistoryCodesPaginatesAfterCombiningSources(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	usedBy := int64(10)
	at := func(hours int) *time.Time {
		v := base.Add(time.Duration(hours) * time.Hour)
		return &v
	}

	got := mergeBalanceHistoryCodes(
		[]RedeemCode{
			{ID: 1, Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(4), CreatedAt: *at(4)},
			{ID: 2, Type: RedeemTypeConcurrency, UsedBy: &usedBy, UsedAt: at(2), CreatedAt: *at(2)},
		},
		[]RedeemCode{
			{ID: -3, Type: RedeemTypeAffiliateBalance, UsedBy: &usedBy, UsedAt: at(3), CreatedAt: *at(3)},
			{ID: -4, Type: RedeemTypeAffiliateBalance, UsedBy: &usedBy, UsedAt: at(1), CreatedAt: *at(1)},
		},
		pagination.PaginationParams{Page: 2, PageSize: 2},
	)

	require.Len(t, got, 2)
	require.Equal(t, RedeemTypeConcurrency, got[0].Type)
	require.Equal(t, int64(-4), got[1].ID)
}

func TestMergeBalanceHistoryCodesIncludesRewardGrants(t *testing.T) {
	t.Parallel()

	base := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	usedBy := int64(10)
	at := func(hours int) *time.Time {
		v := base.Add(time.Duration(hours) * time.Hour)
		return &v
	}

	redeemCodes := []RedeemCode{
		{ID: 1, Type: RedeemTypeBalance, UsedBy: &usedBy, UsedAt: at(4), CreatedAt: *at(4)},
	}
	affiliateCodes := []RedeemCode{
		{ID: -3, Type: RedeemTypeAffiliateBalance, UsedBy: &usedBy, UsedAt: at(3), CreatedAt: *at(3)},
	}
	rewardCodes := []RedeemCode{
		{ID: -9, Type: RedeemTypeRewardGrant, Value: 20, Notes: "student_verification · campaign 2026_fall", UsedBy: &usedBy, UsedAt: at(5), CreatedAt: *at(5)},
		{ID: -8, Type: RedeemTypeRewardGrant, Value: 20, UsedBy: &usedBy, UsedAt: at(1), CreatedAt: *at(1)},
	}

	got := mergeBalanceHistoryCodes(redeemCodes, affiliateCodes, pagination.PaginationParams{Page: 1, PageSize: 10}, rewardCodes)

	require.Len(t, got, 4)
	// 时间倒序：reward(5h) → redeem(4h) → affiliate(3h) → reward(1h)
	require.Equal(t, RedeemTypeRewardGrant, got[0].Type)
	require.Equal(t, RedeemTypeBalance, got[1].Type)
	require.Equal(t, RedeemTypeAffiliateBalance, got[2].Type)
	require.Equal(t, RedeemTypeRewardGrant, got[3].Type)
}

func TestNewRewardGrantHistoryCodeMapping(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
	code := newRewardGrantHistoryCode(55, 42, RewardSourceStudentVerification, "2026_fall", 20, createdAt)

	require.Equal(t, int64(-55), code.ID)
	require.Equal(t, "RWD-55", code.Code)
	require.Equal(t, RedeemTypeRewardGrant, code.Type)
	require.Equal(t, float64(20), code.Value)
	require.Equal(t, StatusUsed, code.Status)
	require.NotNil(t, code.UsedBy)
	require.Equal(t, int64(42), *code.UsedBy)
	require.NotNil(t, code.UsedAt)
	require.Equal(t, createdAt, *code.UsedAt)
	require.Contains(t, code.Notes, "student_verification")
	require.Contains(t, code.Notes, "2026_fall")
}
