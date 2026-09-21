//go:build integration

package repository

import (
	"database/sql"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/service"
	"testing"
	"time"
)

// RCResetFixture exposes only the test harness to the external HTTP integration test.
func RCResetFixture(t *testing.T) (*dbent.Client, *sql.DB, *service.User, *service.Group, *service.UserSubscription, *service.ResetCardService, *service.SubscriptionService) {
	client := testEntClient(t)
	limit := 10.0
	user, group, sub := phase4MustStack(t, client, &limit, 10, -72*time.Hour)
	phase0CleanupStack(t, user.ID, group.ID, 0)
	cards, _, subs := phase6NewServices(t, client)
	return client, integrationDB, user, group, sub, cards, subs
}
