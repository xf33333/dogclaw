package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"dogclaw/internal/api"
	"dogclaw/internal/config"
	"dogclaw/pkg/channel"
	"dogclaw/pkg/channel/cli"
	"dogclaw/pkg/channel/qq"
	"dogclaw/pkg/channel/weixin"
	"dogclaw/pkg/commands"
	"dogclaw/pkg/query"
	"dogclaw/pkg/skills"
	"dogclaw/pkg/slash"
	"dogclaw/pkg/terminal"
	"dogclaw/pkg/tools"
	"dogclaw/pkg/tools/cron"
	"dogclaw/pkg/tools/skilltool"
	"dogclaw/pkg/types"
)

// StartupMode represents the mode the program runs in
type StartupMode string

const (
	ModeAgent   StartupMode = "agent"
	ModeGateway StartupMode = "gateway"
	ModeOnboard StartupMode = "onboard"
)

func setupSignalHandler(stopChan chan<- os.Signal) {
	// 监听信号
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGUSR2, syscall.SIGINT)

	fmt.Printf("进程已启动，PID: %d\n", os.Getpid())

	// 在 goroutine 中监听信号
	go func() {
		sig := <-sigs
		fmt.Printf("\n收到信号: %v\n", sig)

		// 如果是 SIGUSR2 信号，以状态码 12 退出
		if sig == syscall.SIGUSR2 {
			os.Exit(12)
		}
		// 其他信号传递给 stopChan
		if stopChan != nil {
			stopChan <- sig
		} else {
			os.Exit(0)
		}
	}()
}
func main() {
	// Print version / build info
	PrintVersion()
	// Ensure AGENT.md exists in ~/.dogclaw
	if err := config.EnsureAgentMarkdownExists(); err != nil {
		fmt.Printf("⚠️  Warning: Failed to ensure AGENT.md exists: %v\n", err)
	}

	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Println("❌ Error: Mode is required")
		fmt.Println("Usage: dogclaw <mode>")
		fmt.Println("Modes:")
		fmt.Println("  agent   - CLI interactive mode for direct communication")
		fmt.Println("  gateway - Starts all configured channels (QQ, Weixin, etc.)")
		fmt.Println("  onboard - Interactive setup for models and channels")
		os.Exit(1)
	}

	startupMode := StartupMode(args[0])
	if startupMode != ModeAgent && startupMode != ModeGateway && startupMode != ModeOnboard {
		fmt.Printf("❌ Error: Invalid mode '%s'. Must be 'agent', 'gateway' or 'onboard'\n", args[0])
		fmt.Println("Usage: dogclaw <mode>")
		fmt.Println("Modes:")
		fmt.Println("  agent   - CLI interactive mode for direct communication")
		fmt.Println("  gateway - Starts all configured channels (QQ, Weixin, etc.)")
		fmt.Println("  onboard - Interactive setup for models and channels")
		os.Exit(1)
	}

	// Load persistent settings from ~/.docclaw/setting.json
	settings, err := config.LoadSettings()
	if err != nil {
		fmt.Printf("⚠️  Failed to load settings, using defaults: %v\n", err)
		settings = config.DefaultSettings()
	}

	// Build config from settings
	cfg, err := config.ConfigFromSettings(settings)
	if err != nil {
		fmt.Printf("⚠️  Active provider not found, using defaults: %v\n", err)
		cfg = config.DefaultConfig()
	}

	// Get API key from environment (supports both ANTHROPIC_API_KEY and OPENROUTER_API_KEY)
	apiKey := strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	}
	if apiKey == "" {
		fmt.Println("⚠️  Warning: No API key found in ANTHROPIC_API_KEY or OPENROUTER_API_KEY")
		return
	}
	cfg.APIKey = apiKey

	// Debug: Print provider and key status
	provider := settings.ActiveAlias
	keyPreview := ""
	if len(apiKey) > 10 {
		keyPreview = apiKey[:5] + "..." + apiKey[len(apiKey)-4:]
	} else {
		keyPreview = apiKey
	}
	fmt.Printf("🔑 Using %s, provider: %s (Key: %s, BaseURL: %s, Model: %s)\n",
		provider, provider, keyPreview, cfg.BaseURL, cfg.Model)

	// Start based on mode
	switch startupMode {
	case ModeAgent:
		setupSignalHandler(nil)
		fmt.Println("🤖 Starting in AGENT mode (CLI communication)...")
		runAgent(cfg, settings)
	case ModeGateway:
		stopChan := make(chan os.Signal, 1)
		setupSignalHandler(stopChan)
		fmt.Println("🌐 Starting in GATEWAY mode (channel communication)...")
		runGateway(cfg, settings, stopChan)
	case ModeOnboard:
		setupSignalHandler(nil)
		fmt.Println("🚀 Starting in ONBOARD mode (setup)...")
		if err := commands.RunOnboard(context.Background(), settings); err != nil {
			fmt.Printf("❌ Onboarding failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// runAgent starts the agent in CLI interactive mode
func runAgent(cfg *config.Config, settings *config.Settings) {
	fmt.Println("🦞 DogClaw - AI Coding Assistant (Go Implementation)")
	fmt.Println("Type your message or /help for commands. Ctrl+C to exit.")
	fmt.Println()

	// Initialize terminal manager with readline (supports Up/Down history, Ctrl+R search)
	tm, err := terminal.New(nil)
	if err != nil {
		fmt.Printf("Failed to initialize terminal: %v\n", err)
		return
	}
	defer tm.Close()

	// Initialize channel registry for CLI mode (allows notifications to show up in terminal)
	registry := channel.NewRegistry()
	registry.Register("cli", cli.NewChannel())

	engineFactory := newEngineFactory(cfg, settings, registry)
	qe := engineFactory()

	// Try to resume the most recent session automatically
	qe.AutoResumeLatestSession(context.Background())

	// Update cron scheduler with registry-aware factory
	cronScheduler := cron.NewScheduler(engineFactory)
	cronScheduler.Start(context.Background())

	// Interactive loop with readline
	for {
		input, err := tm.ReadLine()
		if err != nil {
			break // EOF or error
		}

		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}

		// Handle exit commands natively
		if slash.IsSlashCommand(input) {
			cmd, _ := slash.ParseCommand(input)
			if cmd == "exit" || cmd == "quit" || cmd == "q" {
				tm.Println("👋 Goodbye!")
				return
			}
		}

		// Process everything through QueryEngine, which knows how to handle slash commands natively
		ctx := context.Background()
		err = qe.SubmitMessage(ctx, input)
		if err != nil {
			tm.Printf("Error: %v\n", err)
			continue
		}

		// Print response
		response := qe.GetLastAssistantText()
		if response != "" {
			tm.Println(response)
		}
	}
}

// buildTools returns the standard tool list
func buildTools(registry *channel.Registry) []types.Tool {
	toolsList := []types.Tool{
		tools.NewBashTool(),
		tools.NewFileReadTool(),
		tools.NewFileWriteTool(),
		tools.NewFileEditTool(),
		tools.NewGrepTool(),
		tools.NewGlobTool(),
		tools.NewWebSearchTool(),
		tools.NewRestartGatewayTool(),
		cron.NewCronTool(),
		skilltool.NewSkillTool(),
	}

	if registry != nil {
		toolsList = append(toolsList, tools.NewNotifyChannelTool(registry))
	}

	return toolsList
}

// newEngineFactory creates a factory function for building QueryEngine instances
func newEngineFactory(cfg *config.Config, settings *config.Settings, registry *channel.Registry) func() *query.QueryEngine {
	return func() *query.QueryEngine {
		client := api.NewClient(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Provider)
		toolList := buildTools(registry)
		cwd, _ := os.Getwd()
		loadedSkills, _ := skills.DiscoverSkills(cwd)
		systemPrompt := query.BuildSystemPrompt(toolList, loadedSkills, "")
		qe := query.NewQueryEngine(client, toolList, systemPrompt, cfg.MaxTurns)
		qe.SetVerbose(cfg.Verbose)
		qe.SetShowToolUsageInReply(cfg.ShowToolUsageInReply)
		qe.SetShowThinkingInLog(cfg.ShowThinkingInLog)
		if cfg.MaxBudgetUSD > 0 {
			qe.SetMaxBudget(cfg.MaxBudgetUSD)
		}
		if cfg.MaxTokens > 0 {
			qe.SetMaxTokens(cfg.MaxTokens)
		}
		// Apply heartbeat configuration from settings
		qe.SetHeartbeatEnabled(settings.EnableHeartbeat)
		qe.SetHeartbeatInterval(time.Duration(settings.HeartbeatPeriod) * time.Minute)
		qe.SetHeartbeatTimeout(time.Duration(settings.HeartbeatTimeout) * time.Minute)
		return qe
	}
}

// runGateway starts all configured channels (gateway mode)
func runGateway(cfg *config.Config, settings *config.Settings, stopChan <-chan os.Signal) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize channel registry for Gateway mode
	registry := channel.NewRegistry()

	var channels []channel.Interface

	// QQ channel
	if qqCfg := config.QQSettingsFromEnv(settings); qqCfg.Enabled && qqCfg.AppID != "" && qqCfg.AppSecret != "" {
		ch := qq.NewChannel(qq.Config{
			AppID:        qqCfg.AppID,
			AppSecret:    qqCfg.AppSecret,
			AllowFrom:    qqCfg.AllowFrom,
			SendMarkdown: qqCfg.SendMarkdown,
		})
		channels = append(channels, ch)
		registry.Register("qq", ch)
	}

	// Weixin channel
	if wxCfg := config.WeixinSettingsFromEnv(settings); wxCfg.Enabled && wxCfg.Token != "" {
		ch, err := weixin.NewWeixinChannel(wxCfg)
		if err != nil {
			fmt.Printf("❌ Failed to initialize Weixin channel: %v\n", err)
		} else {
			channels = append(channels, ch)
			registry.Register("weixin", ch)
		}
	}

	if len(channels) == 0 {
		fmt.Println("⚠️  No channels configured for gateway mode. Configure at least one channel (e.g., QQ) in environment.")
		fmt.Println("Falling back to agent mode...")
		runAgent(cfg, settings)
		return
	}

	// Start cron scheduler
	engineFactory := newEngineFactory(cfg, settings, registry)
	cronScheduler := cron.NewScheduler(engineFactory)
	cronScheduler.Start(ctx)

	// Start all channels
	fmt.Printf("🚀 Starting %d channel(s)...\n", len(channels))
	for _, ch := range channels {
		if err := ch.Start(ctx, engineFactory); err != nil {
			fmt.Printf("❌ Failed to start channel: %v\n", err)
		}
	}

	// Handle shutdown signals
	sig := <-stopChan
	fmt.Printf("\n📥 Received %v, shutting down...\n", sig)
	cancel()
	for _, ch := range channels {
		ch.Stop()
	}
	fmt.Println("👋 All channels stopped.")
}
