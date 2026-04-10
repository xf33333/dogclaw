package cron

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"dogclaw/internal/logger"
	"dogclaw/pkg/query"

	"github.com/sirupsen/logrus"
)

// Executor runs cron jobs
type Executor struct {
	engineFactory func(channelName string) *query.QueryEngine
}

func NewExecutor(factory func(channelName string) *query.QueryEngine) *Executor {
	return &Executor{
		engineFactory: factory,
	}
}

// RunJob executes a single cron job
func (e *Executor) RunJob(ctx context.Context, job CronJob) {
	// 1. Create a special logger for cron
	cronLogger := e.createCronLogger()
	
	cronLogger.Infof("Starting cron job: %s (Schedule: %s)", job.Description, job.Schedule)
	
	// 2. Create QueryEngine and configure it
	qe := e.engineFactory("cron")
	qe.SetLogger(cronLogger)
	
	// Use a unique session ID for this run
	sessionID := fmt.Sprintf("cronsession-%d", time.Now().UnixMilli())
	qe.SetSessionID(sessionID)
	
	// 3. Submit the natural language description as a message
	err := qe.SubmitMessage(ctx, job.Description)
	if err != nil {
		cronLogger.Errorf("Cron job failed: %v", err)
		return
	}
	
	// 4. Log the output
	response := qe.GetLastAssistantText()
	cronLogger.Infof("Cron job completed. Response: %s", response)
}

func (e *Executor) createCronLogger() *logrus.Logger {
	cwd, _ := os.Getwd()
	logDir := filepath.Join(cwd, "logs")
	
	cfg := logger.DefaultConfig()
	cfg.LogDir = logDir
	cfg.FilenamePrefix = "cron"
	cfg.DailyRotate = true
	cfg.OutputToStderr = false // Don't spam terminal with cron logs
	
	// We want cron-YYYY-MM-DD.log. 
	// The current internal/logger uses FilenamePrefix + "_" + Date.
	// I'll fix internal/logger to use "-" instead of "_" in a separate step or just accept "cron_yyyy-mm-dd.log"
	// if I can't easily change it without affecting others.
	// Actually, let's just use the default rotate logic and I'll fix the separator in logger.go.
	
	return logger.NewLogger(cfg)
}
