package service

import (
	"context"
	"strconv"
	"time"
)

// RewardArrival is a receipt of an already committed grant/application, never a balance delta.
// No administrator notes, actor identities or internal monetary amounts cross this boundary.
type RewardArrival struct {
	ID             string    `json:"id"`
	Type           string    `json:"type"`
	Quantity       int       `json:"quantity"`
	OccurredAt     time.Time `json:"occurred_at"`
	SubscriptionID *int64    `json:"subscription_id,omitempty"`
}

type RewardArrivalFeed struct {
	AccountID string          `json:"account_id"`
	Events    []RewardArrival `json:"events"`
}

type RewardArrivalReader interface {
	ListRewardArrivals(context.Context, int64, time.Time) ([]RewardArrival, error)
}

func (s *AccountStatusService) GetRewardArrivals(ctx context.Context, userID int64) (*RewardArrivalFeed, error) {
	reader, ok := s.resetCards.(RewardArrivalReader)
	if !ok || userID <= 0 {
		return nil, nil
	}
	events, err := reader.ListRewardArrivals(ctx, userID, s.now().Add(-7*24*time.Hour))
	if err != nil {
		return nil, err
	}
	if events == nil {
		events = []RewardArrival{}
	}
	return &RewardArrivalFeed{AccountID: strconv.FormatInt(userID, 10), Events: events}, nil
}
