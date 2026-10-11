package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/109isaque10/mega-account-generator/pkg/i18n"
	"github.com/109isaque10/mega-account-generator/pkg/mega"
	"github.com/109isaque10/mega-account-generator/pkg/storage"
)

type LogFunc func(level, msg string)

type Scheduler struct {
	store        *storage.Store
	client       *mega.Client
	intervalDays int
	lang         string
	logFn        LogFunc

	mu          sync.Mutex
	running     bool
	lastRun     time.Time
	nextRun     time.Time
	triggerChan chan struct{}
	stopChan    chan struct{}
}

func New(store *storage.Store, client *mega.Client, intervalDays int, lang string, logFn LogFunc) *Scheduler {
	if intervalDays <= 0 {
		intervalDays = 30
	}
	return &Scheduler{
		store:        store,
		client:       client,
		intervalDays: intervalDays,
		lang:         lang,
		logFn:        logFn,
		triggerChan:  make(chan struct{}, 1),
		stopChan:     make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.nextRun = time.Now().Add(time.Duration(s.intervalDays) * 24 * time.Hour)
	s.mu.Unlock()

	go s.loop()
}

func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	close(s.stopChan)
}

func (s *Scheduler) TriggerNow() bool {
	select {
	case s.triggerChan <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Scheduler) GetStatus() (lastRun time.Time, nextRun time.Time, isRunning bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastRun, s.nextRun, s.running
}

func (s *Scheduler) loop() {
	duration := time.Duration(s.intervalDays) * 24 * time.Hour
	ticker := time.NewTicker(duration)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopChan:
			return
		case <-ticker.C:
			s.runCycle()
		case <-s.triggerChan:
			s.runCycle()
		}
	}
}

func (s *Scheduler) runCycle() {
	s.mu.Lock()
	s.lastRun = time.Now()
	s.nextRun = time.Now().Add(time.Duration(s.intervalDays) * 24 * time.Hour)
	s.mu.Unlock()

	if s.logFn != nil {
		s.logFn("info", i18n.T(s.lang, "msg_keepalive_started"))
	}

	accounts := s.store.List()
	checkedCount := 0

	for _, acc := range accounts {
		if acc.Status != "verified" {
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
		err := s.client.LoginCheck(ctx, acc.Email, acc.MegaPassword)
		cancel()

		if err == nil {
			_ = s.store.RecordLogin(acc.Email, true, "")
			if s.logFn != nil {
				s.logFn("success", i18n.T(s.lang, "msg_login_success", acc.Email))
			}
		} else {
			_ = s.store.RecordLogin(acc.Email, false, err.Error())
			if s.logFn != nil {
				s.logFn("warning", i18n.T(s.lang, "msg_login_failed", acc.Email, err.Error()))
			}
		}
		checkedCount++

		// Small delay to keep resource consumption low and avoid rate limits
		time.Sleep(1500 * time.Millisecond)
	}

	if s.logFn != nil {
		s.logFn("info", i18n.T(s.lang, "msg_keepalive_completed", checkedCount))
	}
}
