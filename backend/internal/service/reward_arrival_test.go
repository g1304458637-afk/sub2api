package service

import (
	"context"
	"errors"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type rewardArrivalReaderStub struct {
	user  int64
	since time.Time
	fail  bool
}

func (s *rewardArrivalReaderStub) CountAvailableResetCards(context.Context, int64, time.Time) (int, error) {
	return 0, nil
}
func (s *rewardArrivalReaderStub) ListRewardArrivals(_ context.Context, user int64, since time.Time) ([]RewardArrival, error) {
	s.user = user
	s.since = since
	if s.fail {
		return nil, errors.New("ledger unavailable")
	}
	return []RewardArrival{{ID: "card:5", Type: "reset_card_received", Quantity: 1, OccurredAt: since.Add(time.Hour)}}, nil
}
func TestRewardArrivalUserScope(t *testing.T) {
	reader := &rewardArrivalReaderStub{}
	now := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	service := NewAccountStatusService(nil, nil, nil, nil, reader, true)
	service.SetNow(func() time.Time { return now })
	feed, err := service.GetRewardArrivals(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "42", feed.AccountID)
	require.Equal(t, int64(42), reader.user)
	require.Equal(t, now.Add(-7*24*time.Hour), reader.since)
	require.Len(t, feed.Events, 1)
	reader.fail = true
	feed, err = service.GetRewardArrivals(context.Background(), 42)
	require.Error(t, err)
	require.Nil(t, feed)
}
func TestRewardArrivalMissingReaderIsOptional(t *testing.T) {
	service := NewAccountStatusService(nil, nil, nil, nil, nil, true)
	feed, err := service.GetRewardArrivals(context.Background(), 42)
	require.NoError(t, err)
	require.Nil(t, feed)
}
