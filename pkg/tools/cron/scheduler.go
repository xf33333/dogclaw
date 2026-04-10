package cron

import (
	"context"
	"time"

	"dogclaw/internal/logger"
	"dogclaw/pkg/query"

	"github.com/robfig/cron/v3"
)

// Scheduler manages background cron job checking
type Scheduler struct {
	executor      *Executor
	engineFactory func(channelName string) *query.QueryEngine
}

func NewScheduler(factory func(channelName string) *query.QueryEngine) *Scheduler {
	return &Scheduler{
		executor:      NewExecutor(factory),
		engineFactory: factory,
	}
}

// Start starts the background per-minute checker
func (s *Scheduler) Start(ctx context.Context) {
	logger.Info("Cron scheduler started (checking every minute)")
	
	// Use a ticker that triggers at the start of every minute if possible, 
	// or just every 60 seconds.
	ticker := time.NewTicker(time.Minute)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				logger.Info("Cron scheduler stopping...")
				return
			case <-ticker.C:
				s.CheckAndRun(ctx)
			}
		}
	}()
}

// CheckAndRun loads config and runs tasks that are due
func (s *Scheduler) CheckAndRun(ctx context.Context) {
	config, err := LoadConfig()
	if err != nil {
		logger.Errorf("Failed to load cron config in scheduler: %v", err)
		return
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
	now := time.Now().Truncate(time.Minute)

	for _, job := range config.Tasks {
		sched, err := parser.Parse(job.Schedule)
		if err != nil {
			logger.Errorf("Failed to parse cron schedule '%s': %v", job.Schedule, err)
			continue
		}

		// Check if the job should run at the current minute.
		// Next(now - 1s) should be now.
		next := sched.Next(now.Add(-1 * time.Second)).Truncate(time.Minute)
		if next.Equal(now) {
			go s.executor.RunJob(ctx, job)
		}
	}
}
