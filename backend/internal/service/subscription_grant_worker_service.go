package service

import (
	"context"
	"database/sql"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	// subscriptionGrantWorkerInterval 与订阅到期扫描同频：pending Grant 的衔接
	// 延迟上限为一分钟（当前订阅过期 → BatchUpdateExpiredStatus 落库 → 同周期
	// 内 ExpireLapsedByUser 收敛 → Grant 激活）。
	subscriptionGrantWorkerInterval      = time.Minute
	subscriptionGrantWorkerLeaderLockKey = "subscription:grant:worker:leader"
	subscriptionGrantWorkerLeaderLockTTL = 5 * time.Minute
	// subscriptionGrantWorkerBatch 单周期处理的 pending 台账上限。
	subscriptionGrantWorkerBatch = 200
)

// SubscriptionGrantWorkerService 周期维护 Grant 台账：
//  1. ExpireFulfilledGrants：贡献期自然结束的 fulfilled → expired（归档）；
//  2. ActivateDuePendingGrants：end_of_term / 冲突转 pending 的权益在
//     「目标组无冲突 active 订阅」时激活衔接（FIFO）。
type SubscriptionGrantWorkerService struct {
	grantService *SubscriptionGrantService
	interval     time.Duration
	stopCh       chan struct{}
	stopOnce     sync.Once
	wg           sync.WaitGroup

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
}

func NewSubscriptionGrantWorkerService(grantService *SubscriptionGrantService) *SubscriptionGrantWorkerService {
	return &SubscriptionGrantWorkerService{
		grantService: grantService,
		interval:     subscriptionGrantWorkerInterval,
		stopCh:       make(chan struct{}),
		instanceID:   uuid.NewString(),
	}
}

// SetLeaderLock 注入 leader 锁（多实例部署时单实例执行扫描；nil = 单实例/测试直跑）。
func (s *SubscriptionGrantWorkerService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *SubscriptionGrantWorkerService) Start() {
	if s == nil || s.grantService == nil || s.interval <= 0 {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()

		s.runOnce()
		for {
			select {
			case <-ticker.C:
				s.runOnce()
			case <-s.stopCh:
				return
			}
		}
	}()
}

func (s *SubscriptionGrantWorkerService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}

func (s *SubscriptionGrantWorkerService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// 多实例互斥：单 leader 扫描，避免重复激活（激活本身幂等，这里省扫描开销）。
	if s.lockCache != nil && s.db != nil {
		release, ok := tryAcquireSingletonLeaderLock(ctx, s.lockCache, s.db, subscriptionGrantWorkerLeaderLockKey, s.instanceID, subscriptionGrantWorkerLeaderLockTTL)
		if !ok {
			return
		}
		defer release()
	}

	if expired, err := s.grantService.ExpireFulfilledGrants(ctx); err != nil {
		log.Printf("[SubscriptionGrantWorker] expire fulfilled grants failed: %v", err)
	} else if expired > 0 {
		log.Printf("[SubscriptionGrantWorker] archived %d expired grant contributions", expired)
	}

	activated, err := s.grantService.ActivateDuePendingGrants(ctx, subscriptionGrantWorkerBatch)
	if err != nil {
		log.Printf("[SubscriptionGrantWorker] activate pending grants failed: %v", err)
		return
	}
	if activated > 0 {
		log.Printf("[SubscriptionGrantWorker] activated %d pending subscription grants", activated)
	}
}
