package service

import (
	"context"
	"github.com/stretchr/testify/require"
	"testing"
)

type statusGroupRepo struct {
	GroupRepository
	group *Group
}

func (r statusGroupRepo) GetByID(context.Context, int64) (*Group, error) { return r.group, nil }

func TestAccountStatusMissingGroupControlled(t *testing.T) {
	svc := NewAccountStatusService(nil, nil, nil, nil, nil, true)
	_, err := svc.buildStatus(context.Background(), &UserSubscription{ID: 1, GroupID: 4})
	require.ErrorIs(t, err, ErrSubscriptionInvalid)
	svc.groupRepo = statusGroupRepo{group: &Group{ID: 4, Name: "Recovered"}}
	dto, err := svc.buildStatus(context.Background(), &UserSubscription{ID: 1, GroupID: 4})
	require.NoError(t, err)
	require.Equal(t, "Recovered", dto.DisplayName)
	_, err = svc.buildStatus(context.Background(), nil)
	require.ErrorIs(t, err, ErrSubscriptionInvalid)
}
