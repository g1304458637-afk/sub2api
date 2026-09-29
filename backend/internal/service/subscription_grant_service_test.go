package service

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/stretchr/testify/require"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	"github.com/DATA-DOG/go-sqlmock"
)

// =============================================================================
// fakes：内存版台账 / claim / 订阅世界（fakeAssigner + fakeUserSubRepo 共享状态）
// =============================================================================

type fakeGrantRepo struct {
	mu        sync.Mutex
	grants    map[int64]*SubscriptionGrant
	nextID    int64
	failWrite error
	expiredN  int64
	// floors: subscriptionID -> paid floor
	paidFloors map[int64]time.Time
}

func newFakeGrantRepo() *fakeGrantRepo {
	return &fakeGrantRepo{grants: map[int64]*SubscriptionGrant{}, nextID: 1000, paidFloors: map[int64]time.Time{}}
}

func (f *fakeGrantRepo) InsertIdempotent(ctx context.Context, grant *SubscriptionGrant) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failWrite != nil {
		return false, f.failWrite
	}
	if grant.SourceKey != nil {
		for _, g := range f.grants {
			if g.Source == grant.Source && g.SourceKey != nil && *g.SourceKey == *grant.SourceKey {
				return false, nil
			}
		}
	}
	f.nextID++
	grant.ID = f.nextID
	now := time.Now()
	grant.CreatedAt = now
	grant.UpdatedAt = now
	cp := *grant
	f.grants[grant.ID] = &cp
	return true, nil
}

func (f *fakeGrantRepo) GetByID(ctx context.Context, id int64) (*SubscriptionGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.grants[id]
	if !ok {
		return nil, ErrGrantNotFound
	}
	cp := *g
	return &cp, nil
}

func (f *fakeGrantRepo) GetBySourceKey(ctx context.Context, source, sourceKey string) (*SubscriptionGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, g := range f.grants {
		if g.Source == source && g.SourceKey != nil && *g.SourceKey == sourceKey {
			cp := *g
			return &cp, nil
		}
	}
	return nil, ErrGrantNotFound
}

func (f *fakeGrantRepo) GetForUpdate(ctx context.Context, id int64) (*SubscriptionGrant, error) {
	return f.GetByID(ctx, id)
}

func (f *fakeGrantRepo) UpdateActivation(ctx context.Context, id int64, update GrantActivationUpdate) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.grants[id]
	if !ok {
		return ErrGrantNotFound
	}
	g.Status = update.Status
	g.LinkedSubscriptionID = &update.LinkedSubscriptionID
	g.ContributionStart = &update.ContributionStart
	g.ContributionEnd = &update.ContributionEnd
	g.ActivatedAt = &update.ActivatedAt
	return nil
}

func (f *fakeGrantRepo) MarkRevoked(ctx context.Context, id int64, revokedBy int64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.grants[id]
	if !ok {
		return ErrGrantNotFound
	}
	now := time.Now()
	g.Status = domain.SubscriptionGrantStatusRevoked
	g.RevokedAt = &now
	g.RevokedBy = &revokedBy
	g.RevokeReason = &reason
	return nil
}

func (f *fakeGrantRepo) MarkFailed(ctx context.Context, id int64, reason string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	g, ok := f.grants[id]
	if !ok {
		return ErrGrantNotFound
	}
	g.Status = domain.SubscriptionGrantStatusFailed
	g.FailureReason = &reason
	return nil
}

func (f *fakeGrantRepo) ListPending(ctx context.Context, limit int) ([]*SubscriptionGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*SubscriptionGrant{}
	for _, g := range f.grants {
		if g.Status == domain.SubscriptionGrantStatusPending {
			cp := *g
			out = append(out, &cp)
		}
	}
	return out, nil
}

func (f *fakeGrantRepo) ExpireFulfilledContribution(ctx context.Context, now time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var count int64
	for _, g := range f.grants {
		if g.Status == domain.SubscriptionGrantStatusFulfilled && g.ContributionEnd != nil && !g.ContributionEnd.After(now) {
			g.Status = domain.SubscriptionGrantStatusExpired
			count++
		}
	}
	f.expiredN = count
	return count, nil
}

func (f *fakeGrantRepo) PaidFloorForSubscription(ctx context.Context, subscriptionID int64) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if t, ok := f.paidFloors[subscriptionID]; ok {
		return &t, nil
	}
	return nil, nil
}

func (f *fakeGrantRepo) OtherGrantFloorForSubscription(ctx context.Context, subscriptionID, excludeGrantID int64) (*time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var floor *time.Time
	for _, g := range f.grants {
		if g.ID == excludeGrantID || g.LinkedSubscriptionID == nil || *g.LinkedSubscriptionID != subscriptionID {
			continue
		}
		if g.ContributionEnd != nil && (floor == nil || g.ContributionEnd.After(*floor)) {
			t := *g.ContributionEnd
			floor = &t
		}
	}
	return floor, nil
}

func (f *fakeGrantRepo) AdminList(ctx context.Context, filter *GrantAdminFilter) (*GrantAdminList, error) {
	return &GrantAdminList{Items: []GrantAdminItem{}}, nil
}

func (f *fakeGrantRepo) ListByUser(ctx context.Context, userID int64, limit int) ([]*SubscriptionGrant, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []*SubscriptionGrant{}
	for _, g := range f.grants {
		if g.UserID == userID {
			cp := *g
			out = append(out, &cp)
		}
	}
	return out, nil
}

type fakeClaimRepo struct {
	mu     sync.Mutex
	claims map[string]*BenefitClaim // key: benefitCode|identityKey
	byUser map[string]*BenefitClaim // key: benefitCode|userID
}

func newFakeClaimRepo() *fakeClaimRepo {
	return &fakeClaimRepo{claims: map[string]*BenefitClaim{}, byUser: map[string]*BenefitClaim{}}
}

func (f *fakeClaimRepo) InsertIdempotent(ctx context.Context, claim *BenefitClaim) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	idKey := claim.BenefitCode + "|" + claim.IdentityKey
	userKey := claim.BenefitCode + "|" + fmt.Sprint(claim.UserID)
	if _, ok := f.claims[idKey]; ok {
		return false, nil
	}
	if _, ok := f.byUser[userKey]; ok {
		return false, nil
	}
	claim.ID = int64(len(f.claims) + 1)
	claim.CreatedAt = time.Now()
	now := time.Now()
	claim.CreatedAt = now
	cp := *claim
	f.claims[idKey] = &cp
	f.byUser[userKey] = &cp
	return true, nil
}

func (f *fakeClaimRepo) GetByBenefitAndIdentity(ctx context.Context, benefitCode, identityKey string) (*BenefitClaim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.claims[benefitCode+"|"+identityKey]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

func (f *fakeClaimRepo) GetByBenefitAndUser(ctx context.Context, benefitCode string, userID int64) (*BenefitClaim, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if c, ok := f.byUser[benefitCode+"|"+fmt.Sprint(userID)]; ok {
		cp := *c
		return &cp, nil
	}
	return nil, nil
}

// fakeSubscriptionWorld 同时充当 GrantSubscriptionAssigner（激活原语）与
// UserSubscriptionRepository 的必要子集 + SubscriptionSingleActiveGuard。
type fakeSubscriptionWorld struct {
	UserSubscriptionRepository // 嵌入接口：未实现的方法测试不会触达（nil panic 即失败信号）
	now                        func() time.Time
	mu                         sync.Mutex
	subs                       map[int64]map[int64]*UserSubscription // userID -> groupID -> sub
	next                       int64
	// invalidations 记录缓存失效调用（(userID, groupID)）
	invalidations [][2]int64
}

func newFakeSubscriptionWorld() *fakeSubscriptionWorld {
	return &fakeSubscriptionWorld{subs: map[int64]map[int64]*UserSubscription{}, now: time.Now}
}

func (f *fakeSubscriptionWorld) seedSub(userID, groupID int64, status string, expiresAt time.Time) *UserSubscription {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.next++
	sub := &UserSubscription{ID: f.next, UserID: userID, GroupID: groupID, Status: status, ExpiresAt: expiresAt, StartsAt: expiresAt.AddDate(0, 0, -30)}
	if f.subs[userID] == nil {
		f.subs[userID] = map[int64]*UserSubscription{}
	}
	f.subs[userID][groupID] = sub
	return sub
}

func (f *fakeSubscriptionWorld) get(userID, groupID int64) *UserSubscription {
	if f.subs[userID] == nil {
		return nil
	}
	return f.subs[userID][groupID]
}

func (f *fakeSubscriptionWorld) findActiveConflict(userID, excludeGroupID int64) *UserSubscription {
	for gid, sub := range f.subs[userID] {
		if gid != excludeGroupID && sub.Status == SubscriptionStatusActive && sub.ExpiresAt.After(f.now()) {
			return sub
		}
	}
	return nil
}

// ---- GrantSubscriptionAssigner ----

func (f *fakeSubscriptionWorld) AssignOrExtendSubscriptionDeferredCache(ctx context.Context, input *AssignSubscriptionInput) (*UserSubscription, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if conflict := f.findActiveConflict(input.UserID, input.GroupID); conflict != nil {
		return nil, false, ErrPrimarySubscriptionExists
	}
	now := f.now()
	existing := f.get(input.UserID, input.GroupID)
	if existing != nil {
		if !existing.ExpiresAt.After(now) {
			// 过期 → 从 now 重算并激活（迁移 242 语义）
			existing.Status = SubscriptionStatusActive
			existing.StartsAt = now
			existing.ExpiresAt = now.AddDate(0, 0, input.ValidityDays)
			return existing, true, nil
		}
		existing.ExpiresAt = existing.ExpiresAt.AddDate(0, 0, input.ValidityDays)
		return existing, true, nil
	}
	f.next++
	sub := &UserSubscription{ID: f.next, UserID: input.UserID, GroupID: input.GroupID, Status: SubscriptionStatusActive, StartsAt: now, ExpiresAt: now.AddDate(0, 0, input.ValidityDays)}
	if f.subs[input.UserID] == nil {
		f.subs[input.UserID] = map[int64]*UserSubscription{}
	}
	f.subs[input.UserID][input.GroupID] = sub
	return sub, false, nil
}

func (f *fakeSubscriptionWorld) InvalidateSubCache(userID, groupID int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.invalidations = append(f.invalidations, [2]int64{userID, groupID})
}

// ---- UserSubscriptionRepository（Grant 服务用到的子集）----

func (f *fakeSubscriptionWorld) GetByUserIDAndGroupID(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sub := f.get(userID, groupID)
	if sub == nil {
		return nil, ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (f *fakeSubscriptionWorld) GetByIDForUpdate(ctx context.Context, id int64) (*UserSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, byGroup := range f.subs {
		for _, sub := range byGroup {
			if sub.ID == id {
				cp := *sub
				return &cp, nil
			}
		}
	}
	return nil, ErrSubscriptionNotFound
}

func (f *fakeSubscriptionWorld) ExtendExpiry(ctx context.Context, subscriptionID int64, newExpiresAt time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, byGroup := range f.subs {
		for _, sub := range byGroup {
			if sub.ID == subscriptionID {
				sub.ExpiresAt = newExpiresAt
				return nil
			}
		}
	}
	return ErrSubscriptionNotFound
}

func (f *fakeSubscriptionWorld) UpdateStatus(ctx context.Context, subscriptionID int64, status string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, byGroup := range f.subs {
		for _, sub := range byGroup {
			if sub.ID == subscriptionID {
				sub.Status = status
				return nil
			}
		}
	}
	return ErrSubscriptionNotFound
}

func (f *fakeSubscriptionWorld) FindActiveByUserIDExcludingGroup(ctx context.Context, userID, groupID int64) (*UserSubscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	sub := f.findActiveConflict(userID, groupID)
	if sub == nil {
		return nil, nil
	}
	cp := *sub
	return &cp, nil
}

func (f *fakeSubscriptionWorld) ExpireLapsedByUser(ctx context.Context, userID int64, now time.Time) (int64, error) {
	return 0, nil
}

// ---- Group / User repos ----

type fakeGroupRepoGrant struct {
	GroupRepository
	group *Group
	err   error
}

func (f *fakeGroupRepoGrant) GetByID(ctx context.Context, id int64) (*Group, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.group, nil
}

type fakeUserRepoGrant struct {
	UserRepository
	users map[int64]bool
}

func (f *fakeUserRepoGrant) GetByID(ctx context.Context, id int64) (*User, error) {
	if f.users[id] {
		return &User{ID: id}, nil
	}
	return nil, errors.New("not found")
}

// ---- 构造被测服务 ----

func newTestGrantService(t *testing.T) (*SubscriptionGrantService, *fakeGrantRepo, *fakeClaimRepo, *fakeSubscriptionWorld, *sqlmock.Sqlmock, *dbent.Client) {
	t.Helper()
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	t.Cleanup(func() { _ = db.Close() })
	client := dbent.NewClient(dbent.Driver(entsql.OpenDB(dialect.Postgres, db)))

	grantRepo := newFakeGrantRepo()
	claimRepo := newFakeClaimRepo()
	world := newFakeSubscriptionWorld()
	svc := NewSubscriptionGrantService(
		client, grantRepo, claimRepo,
		&fakeUserRepoGrant{users: map[int64]bool{1: true, 2: true}},
		&fakeGroupRepoGrant{group: &Group{SubscriptionType: SubscriptionTypeSubscription}},
		world, world, nil,
	)
	fixed := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	svc.SetNowFunc(func() time.Time { return fixed })
	world.now = func() time.Time { return fixed }
	return svc, grantRepo, claimRepo, world, &mock, client
}

func grantCmd(userID, groupID int64, policy string, sourceKey string) *CreateSubscriptionGrantCommand {
	return &CreateSubscriptionGrantCommand{
		UserID:          userID,
		GroupID:         groupID,
		Source:          domain.SubscriptionGrantSourceAdminGrant,
		SourceKey:       sourceKey,
		EffectivePolicy: policy,
		DurationDays:    30,
		Reason:          "test",
	}
}

// =============================================================================
// Grant 状态机测试
// =============================================================================

// 场景 18.1：新用户 → admin grant → 立即获得订阅
func TestGrantCreate_NewUserImmediateActivation(t *testing.T) {
	svc, grantRepo, _, world, mock, _ := newTestGrantService(t)
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()

	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k1"))
	require.NoError(t, err)
	require.Equal(t, GrantOutcomeActivatedNew, execution.Outcome.Action)
	require.Equal(t, domain.SubscriptionGrantStatusFulfilled, execution.Grant.Status)
	require.NotNil(t, execution.Grant.LinkedSubscriptionID)

	sub := world.get(1, 10)
	require.NotNil(t, sub)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.True(t, sub.ExpiresAt.Equal(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)))
	require.Len(t, world.invalidations, 1) // 提交后缓存失效
	_ = grantRepo
}

// 场景 18.2：同一幂等键重试多次 → 只有一个 Grant
func TestGrantCreate_IdempotentRetrySingleGrant(t *testing.T) {
	svc, grantRepo, _, _, mock, _ := newTestGrantService(t)
	for i := 0; i < 3; i++ {
		(*mock).ExpectBegin()
		(*mock).ExpectCommit()
		execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "same-key"))
		require.NoError(t, err)
		if i == 0 {
			require.Equal(t, GrantOutcomeActivatedNew, execution.Outcome.Action)
		} else {
			require.Equal(t, GrantOutcomeAlreadyGranted, execution.Outcome.Action)
			require.Equal(t, execution.Grant.ID, int64(1001))
		}
	}
	require.Len(t, grantRepo.grants, 1)
}

// 场景 18.3：同用户同套餐 active → 再赠送 → 从当前 expires_at 顺延（而非从 now 覆盖）
func TestGrantCreate_SameGroupActiveExtendsFromExpires(t *testing.T) {
	svc, _, _, world, mock, _ := newTestGrantService(t)
	base := time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC)
	world.seedSub(1, 10, SubscriptionStatusActive, base)

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-ext"))
	require.NoError(t, err)
	require.Equal(t, GrantOutcomeExtended, execution.Outcome.Action)

	sub := world.get(1, 10)
	require.True(t, sub.ExpiresAt.Equal(base.AddDate(0, 0, 30)), "should extend from old expires, got %v", sub.ExpiresAt)
	// 贡献时段 = [旧到期, 新到期]
	require.True(t, execution.Grant.ContributionStart.Equal(base))
	require.True(t, execution.Grant.ContributionEnd.Equal(base.AddDate(0, 0, 30)))
}

// 场景 18.4 + 19：expired subscription → 再赠送 → 真正重新激活（不是 stale success）
func TestGrantCreate_ExpiredSubscriptionReactivated(t *testing.T) {
	svc, grantRepo, _, world, mock, _ := newTestGrantService(t)
	expired := time.Now().Add(-48 * time.Hour)
	stale := world.seedSub(1, 10, SubscriptionStatusActive, expired) // 模拟惰性到期前的行

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-exp"))
	require.NoError(t, err)
	require.Equal(t, GrantOutcomeActivatedNew, execution.Outcome.Action)

	// 数据库行必须真正变为 active 且 expires 从 now 重算
	sub := world.get(1, 10)
	require.Equal(t, SubscriptionStatusActive, sub.Status)
	require.True(t, sub.ExpiresAt.Equal(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)))
	require.NotEqual(t, expired, sub.ExpiresAt)
	// API 访问口径：IsActive 为真
	require.True(t, sub.IsActive())
	require.Equal(t, domain.SubscriptionGrantStatusFulfilled, grantRepo.grants[execution.Grant.ID].Status)
	_ = stale
}

// 场景 18.5/18.6：不同套餐已有 active（付费会员）→ 学生赠送 pending，绝不降级/覆盖
func TestGrantCreate_CrossGroupConflictPendingFallback(t *testing.T) {
	svc, grantRepo, _, world, mock, _ := newTestGrantService(t)
	paidExpiry := time.Date(2027, 3, 29, 12, 0, 0, 0, time.UTC)
	world.seedSub(1, 20, SubscriptionStatusActive, paidExpiry) // 其他组付费订阅

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	cmd := grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-stu")
	cmd.Source = domain.SubscriptionGrantSourceStudentVerification
	cmd.PendingFallback = true
	execution, err := svc.CreateGrant(context.Background(), cmd)
	require.NoError(t, err)
	require.Equal(t, GrantOutcomePending, execution.Outcome.Action)
	require.Equal(t, domain.SubscriptionGrantStatusPending, execution.Grant.Status)

	// 付费订阅原样保留
	still := world.get(1, 20)
	require.Equal(t, SubscriptionStatusActive, still.Status)
	require.True(t, still.ExpiresAt.Equal(paidExpiry), "付费订阅到期时间未被改动")
	// 没有产生第二份 ACTIVE（目标组无订阅行）
	require.Nil(t, world.get(1, 10))
	_ = grantRepo
}

// 场景 18.5b：管理员 immediate + 跨组冲突 → 409（预览引导改选 end_of_term）
func TestGrantCreate_CrossGroupConflictNoFallbackRejects(t *testing.T) {
	svc, _, _, world, mock, _ := newTestGrantService(t)
	world.seedSub(1, 20, SubscriptionStatusActive, time.Now().AddDate(0, 1, 0))

	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	_, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-conflict"))
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrGrantConflict))
	// 注：内存 fake 无回滚语义，台账行数由集成测试在真实 PG 上验证；
	// 这里验证的是状态机行为：冲突被拒绝、用户订阅未被触碰。
	require.Nil(t, world.get(1, 10))
}

// 场景 18.7：pending grant → 当前会员结束 → worker 正确衔接
func TestActivateDuePendingGrants_ActivatesWhenConflictResolved(t *testing.T) {
	svc, grantRepo, _, world, mock, _ := newTestGrantService(t)
	world.seedSub(1, 20, SubscriptionStatusActive, time.Now().AddDate(0, 1, 0))
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	cmd := grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-pend")
	cmd.PendingFallback = true
	execution, err := svc.CreateGrant(context.Background(), cmd)
	require.NoError(t, err)
	require.Equal(t, GrantOutcomePending, execution.Outcome.Action)

	// 当前会员到期被清理（模拟到期服务/惰性收敛）
	world.mu.Lock()
	delete(world.subs[1], 20)
	world.mu.Unlock()

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()

	activated, err := svc.ActivateDuePendingGrants(context.Background(), 10)
	require.NoError(t, err)
	require.Equal(t, 1, activated)
	require.Equal(t, domain.SubscriptionGrantStatusFulfilled, grantRepo.grants[execution.Grant.ID].Status)
	sub := world.get(1, 10)
	require.NotNil(t, sub)
	require.True(t, sub.IsActive())
}

// 场景 18.8：撤销 pending grant → 不再生效，不触碰任何订阅
func TestRevokeGrant_PendingCancelledSafely(t *testing.T) {
	svc, _, _, world, mock, _ := newTestGrantService(t)
	world.seedSub(1, 20, SubscriptionStatusActive, time.Now().AddDate(0, 1, 0))
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	cmd := grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-rev")
	cmd.PendingFallback = true
	execution, err := svc.CreateGrant(context.Background(), cmd)
	require.NoError(t, err)

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	revoked, err := svc.RevokeGrant(context.Background(), execution.Grant.ID, 99, "管理员撤销")
	require.NoError(t, err)
	require.Equal(t, domain.SubscriptionGrantStatusRevoked, revoked.Status)
	require.NotNil(t, revoked.RevokedAt)

	// 用户现有订阅不受影响
	require.Equal(t, SubscriptionStatusActive, world.get(1, 20).Status)
	require.Nil(t, world.get(1, 10))
}

// 场景 18.9a：撤销 fulfilled（纯赠送订阅）→ 只回收赠送时段
func TestRevokeGrant_FulfilledPureGrantExpiresNow(t *testing.T) {
	svc, _, _, world, mock, _ := newTestGrantService(t)
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-pure"))
	require.NoError(t, err)
	require.Equal(t, GrantOutcomeActivatedNew, execution.Outcome.Action)

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	_, err = svc.RevokeGrant(context.Background(), execution.Grant.ID, 99, "滥用")
	require.NoError(t, err)

	sub := world.get(1, 10)
	require.Equal(t, SubscriptionStatusExpired, sub.Status)
	// 到期被钳制到服务时钟的 now（固定时钟），赠送时段全部回收
	require.True(t, sub.ExpiresAt.Equal(time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)),
		"grant-only subscription should expire at revoke time, got %v", sub.ExpiresAt)
}

// 场景 18.9b：撤销 fulfilled（在付费订阅上顺延 30 天）→ 只回收赠送段，付费段保留
func TestRevokeGrant_ExtendOnPaidPreservesPaidFloor(t *testing.T) {
	svc, grantRepo, _, world, mock, _ := newTestGrantService(t)
	paidEnd := time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)
	paid := world.seedSub(1, 10, SubscriptionStatusActive, paidEnd)
	grantRepo.paidFloors[paid.ID] = paidEnd

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-ext2"))
	require.NoError(t, err)
	require.True(t, world.get(1, 10).ExpiresAt.Equal(paidEnd.AddDate(0, 0, 30)))

	// 在赠送期内撤销：回收赠送段 → 到期回到付费地板
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	_, err = svc.RevokeGrant(context.Background(), execution.Grant.ID, 99, "撤销赠送")
	require.NoError(t, err)
	require.True(t, world.get(1, 10).ExpiresAt.Equal(paidEnd))
	require.Equal(t, SubscriptionStatusActive, world.get(1, 10).Status, "paid subscription stays active")
}

// 场景 18.9c：赠送创建订阅后用户付费续期 → 撤销赠送不损失付费权益（不缩到过去）
func TestRevokeGrant_PaidAfterGrantNotHarmed(t *testing.T) {
	svc, grantRepo, _, world, mock, _ := newTestGrantService(t)
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-thenpaid"))
	require.NoError(t, err)
	subID := *execution.Grant.LinkedSubscriptionID

	// 用户付费续期：term [旧到期, 旧到期+30]，expires 推到 12-31
	world.mu.Lock()
	sub := world.subs[1][10]
	sub.ExpiresAt = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	world.mu.Unlock()
	grantRepo.mu.Lock()
	grantRepo.paidFloors[subID] = time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)
	grantRepo.mu.Unlock()

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	_, err = svc.RevokeGrant(context.Background(), execution.Grant.ID, 99, "撤销")
	require.NoError(t, err)
	// 付费地板(12-31) ≥ 当前到期 → 无可回收时段，付费权益完整保留
	require.True(t, world.get(1, 10).ExpiresAt.Equal(time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)))
	require.Equal(t, SubscriptionStatusActive, world.get(1, 10).Status)
}

// 场景 18.9d：撤销只能缩短，绝不能延长（撤销已撤销/无效果情况安全）
func TestRevokeGrant_NeverExtends(t *testing.T) {
	svc, _, _, world, mock, _ := newTestGrantService(t)
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	execution, err := svc.CreateGrant(context.Background(), grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "k-neverext"))
	require.NoError(t, err)

	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	_, err = svc.RevokeGrant(context.Background(), execution.Grant.ID, 99, "first")
	require.NoError(t, err)
	firstExpiry := world.get(1, 10).ExpiresAt

	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	_, err = svc.RevokeGrant(context.Background(), execution.Grant.ID, 99, "second")
	require.Error(t, err, "double revoke is rejected")
	require.True(t, world.get(1, 10).ExpiresAt.Equal(firstExpiry))
}

// fulfilled → expired 归档（worker）
func TestExpireFulfilledGrants_ArchivesPassedContributions(t *testing.T) {
	svc, grantRepo, _, _, _, _ := newTestGrantService(t)
	// 手工构造已过贡献期的 fulfilled 台账
	past := time.Now().Add(-time.Hour)
	grantRepo.nextID++
	grantRepo.grants[grantRepo.nextID] = &SubscriptionGrant{
		ID: grantRepo.nextID, UserID: 1, GroupID: 10, Source: domain.SubscriptionGrantSourceAdminGrant,
		Status: domain.SubscriptionGrantStatusFulfilled, DurationDays: 30,
		ContributionEnd: &past,
	}
	count, err := svc.ExpireFulfilledGrants(context.Background())
	require.NoError(t, err)
	require.Equal(t, int64(1), count)
}

// =============================================================================
// Preview 测试（管理员操作前预览）
// =============================================================================

func TestPreviewGrant_Outcomes(t *testing.T) {
	svc, _, _, world, _, _ := newTestGrantService(t)
	ctx := context.Background()

	// 无订阅 → will_activate_new
	p, err := svc.PreviewGrant(ctx, grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, ""))
	require.NoError(t, err)
	require.Equal(t, GrantPreviewWillActivateNew, p.Outcome)

	// 同组 active → will_extend（从当前到期顺延）
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	world.seedSub(1, 10, SubscriptionStatusActive, base)
	p, err = svc.PreviewGrant(ctx, grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, ""))
	require.NoError(t, err)
	require.Equal(t, GrantPreviewWillExtend, p.Outcome)
	require.True(t, p.PredictedExpires.Equal(base.AddDate(0, 0, 30)))

	// 跨组 active + immediate → conflict（提示改选 end_of_term）
	world.seedSub(2, 20, SubscriptionStatusActive, time.Now().AddDate(0, 3, 0))
	p, err = svc.PreviewGrant(ctx, grantCmd(2, 10, domain.SubscriptionGrantPolicyImmediate, ""))
	require.NoError(t, err)
	require.Equal(t, GrantPreviewConflict, p.Outcome)

	// 跨组 active + end_of_term → will_be_pending
	p, err = svc.PreviewGrant(ctx, grantCmd(2, 10, domain.SubscriptionGrantPolicyEndOfTerm, ""))
	require.NoError(t, err)
	require.Equal(t, GrantPreviewWillBePending, p.Outcome)
}

// =============================================================================
// 一次性权益（benefit claims）测试
// =============================================================================

func TestGrantCreate_BenefitClaimDuplicateEmailAcrossAccounts(t *testing.T) {
	svc, grantRepo, _, _, mock, _ := newTestGrantService(t)
	// 账号 A 用邮箱 e1 领取
	(*mock).ExpectBegin()
	(*mock).ExpectCommit()
	cmdA := grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "HUBU_STUDENT_WELCOME:e1@stu.hubu.edu.cn")
	cmdA.Source = domain.SubscriptionGrantSourceStudentVerification
	cmdA.BenefitCode = "HUBU_STUDENT_WELCOME"
	cmdA.IdentityType = "hubu_email"
	cmdA.IdentityKey = "e1@stu.hubu.edu.cn"
	_, err := svc.CreateGrant(context.Background(), cmdA)
	require.NoError(t, err)

	// 账号 B 用同一邮箱领取 → 拒绝（identity_claimed_by_other_user），台账回滚
	(*mock).ExpectBegin()
	(*mock).ExpectRollback()
	cmdB := grantCmd(2, 10, domain.SubscriptionGrantPolicyImmediate, "HUBU_STUDENT_WELCOME:e1@stu.hubu.edu.cn")
	cmdB.Source = domain.SubscriptionGrantSourceStudentVerification
	cmdB.BenefitCode = "HUBU_STUDENT_WELCOME"
	cmdB.IdentityType = "hubu_email"
	cmdB.IdentityKey = "e1@stu.hubu.edu.cn"
	_, err = svc.CreateGrant(context.Background(), cmdB)
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrBenefitAlreadyClaimed))
	require.Len(t, grantRepo.grants, 1, "only account A's grant exists")
}

func TestGrantCreate_BenefitClaimSameUserDifferentEmail(t *testing.T) {
	svc, grantRepo, claimRepo, _, mock, _ := newTestGrantService(t)
	for _, email := range []string{"e1@stu.hubu.edu.cn", "e2@stu.hubu.edu.cn"} {
		(*mock).ExpectBegin()
		if email == "e1@stu.hubu.edu.cn" {
			(*mock).ExpectCommit()
		} else {
			(*mock).ExpectRollback()
		}
		cmd := grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "HUBU_STUDENT_WELCOME:"+email)
		cmd.Source = domain.SubscriptionGrantSourceStudentVerification
		cmd.BenefitCode = "HUBU_STUDENT_WELCOME"
		cmd.IdentityType = "hubu_email"
		cmd.IdentityKey = email
		_, err := svc.CreateGrant(context.Background(), cmd)
		if email == "e1@stu.hubu.edu.cn" {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
			require.True(t, errors.Is(err, ErrBenefitAlreadyClaimed))
		}
	}
	// 注：fake 无回滚语义（被拒台账行会留在内存 map，真实 DB 回滚由集成测试验证）；
	// claims 是幂等事实源，只应有 e1 一次领取。
	require.Len(t, claimRepo.claims, 1, "同一账号换邮箱不能重复领取")
	require.Equal(t, 2, len(grantRepo.grants)) // fake 伪影：1001 成功 + 1002 未回滚
}

// 并发幂等：并发两个同 source_key 发放 → 只有一个 Grant（模拟 ON CONFLICT 串行化）
func TestGrantCreate_ConcurrentSameSourceKeySingleGrant(t *testing.T) {
	svc, grantRepo, _, _, _, _ := newTestGrantService(t)
	const n = 8
	var wg sync.WaitGroup
	results := make([]*SubscriptionGrantExecution, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			// 内联事务路径：并发时 ctx 内无事务，直接走 CreateGrantInTx 的幂等
			ctx := context.Background()
			execution, err := svc.CreateGrantInTx(ctx, grantCmd(1, 10, domain.SubscriptionGrantPolicyImmediate, "race-key"))
			results[i], errs[i] = execution, err
		}(i)
	}
	wg.Wait()
	granted := 0
	for i := range results {
		if errs[i] == nil && results[i] != nil && results[i].Outcome.Action != GrantOutcomeAlreadyGranted {
			granted++
		}
	}
	require.Equal(t, 1, granted, "concurrent retries must produce exactly one real grant")
	require.Len(t, grantRepo.grants, 1)
}
