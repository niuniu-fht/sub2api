package service

import (
	"context"
	"log"
	"sync"
	"time"
)

// UpstreamBalanceCheckService 周期刷新上游余额快照（默认关闭）。
//
// 间隔来自环境变量 UPSTREAM_BALANCE_CHECK_MINUTES（分钟；0 或未设置 = 不启动），
// 与 new-api 的 CHANNEL_UPDATE_FREQUENCY 语义一致。每轮对全部可查询账号
//（openai 平台 apikey 型）执行 RefreshAll；账号量 <200 时单轮约
// 账号数 × 0.5s 限速 / 4 并发。
type UpstreamBalanceCheckService struct {
	balanceService *UpstreamBalanceService
	interval       time.Duration
	stopCh         chan struct{}
	stopOnce       sync.Once
	wg             sync.WaitGroup
}

func NewUpstreamBalanceCheckService(
	balanceService *UpstreamBalanceService,
	interval time.Duration,
) *UpstreamBalanceCheckService {
	return &UpstreamBalanceCheckService{
		balanceService: balanceService,
		interval:       interval,
		stopCh:         make(chan struct{}),
	}
}

func (s *UpstreamBalanceCheckService) Start() {
	if s == nil || s.balanceService == nil || s.interval <= 0 {
		return
	}
	log.Printf("[UpstreamBalance] periodic refresh started (interval=%s)", s.interval)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.interval)
		defer ticker.Stop()
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

func (s *UpstreamBalanceCheckService) runOnce() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()
	results, err := s.balanceService.RefreshAll(ctx, nil)
	if err != nil {
		log.Printf("[UpstreamBalance] refresh failed: %v", err)
		return
	}
	success, failed := 0, 0
	for _, r := range results {
		if r != nil && r.Status == "success" {
			success++
		} else {
			failed++
		}
	}
	log.Printf("[UpstreamBalance] refresh done: %d ok, %d failed", success, failed)
}

func (s *UpstreamBalanceCheckService) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		close(s.stopCh)
	})
	s.wg.Wait()
}
