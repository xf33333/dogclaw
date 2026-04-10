package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
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
	"dogclaw/pkg/compact"
	"dogclaw/pkg/query"
	"dogclaw/pkg/skills"
	"dogclaw/pkg/slash"
	"dogclaw/pkg/terminal"
	"dogclaw/pkg/tools"
	"dogclaw/pkg/tools/cron"
	"dogclaw/pkg/tools/skilltool"
	"dogclaw/pkg/transcript"
	"dogclaw/pkg/types"
)

// StartupMode represents the mode the program runs in
type StartupMode string

const (
	ModeAgent   StartupMode = "agent"
	ModeGateway StartupMode = "gateway"
	ModeOnboard StartupMode = "onboard"
)

func getRestartFlagPath() string {
	// 获取临时目录路径
	return filepath.Join(os.TempDir(), "dogclaw_restart.flag")
}

func setupSignalHandler(stopChan chan<- os.Signal) {
	// 监听信号
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)

	fmt.Printf("进程已启动，PID: %d\n", os.Getpid())

	// 在 goroutine 中监听信号
	go func() {
		sig := <-sigs
		fmt.Printf("\n收到信号: %v\n", sig)

		// 如果是 SIGHUP 信号，以状态码 12 退出（用于重启）
		if sig == syscall.SIGHUP {
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

func printUsage() {
	fmt.Println("Usage: dogclaw [options] <mode>")
	fmt.Println()
	fmt.Println("Options:")
	fmt.Println("  --config <path>, -c <path>  Path to custom configuration file")
	fmt.Println("  --compact                    Compact the most recent session and exit")
	fmt.Println("  --version                    Show version information")
	fmt.Println()
	fmt.Println("Modes:")
	fmt.Println("  agent    CLI interactive mode for direct communication")
	fmt.Println("  gateway  Starts all configured channels (QQ, Weixin, etc.)")
	fmt.Println("  onboard  Interactive setup for models and channels")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  dogclaw agent")
	fmt.Println("  dogclaw --config /path/to/config.json gateway")
	fmt.Println("  dogclaw -c ./myconfig.json onboard")
	fmt.Println("  dogclaw --compact")
}

func main() {
	// Print version / build info
	PrintVersion()

	// Check for help or version flags first
	for _, arg := range os.Args[1:] {
		if arg == "-h" || arg == "--help" || arg == "help" {
			printUsage()
			os.Exit(0)
		}
		if arg == "--version" || arg == "-v" {
			// Version is already printed by PrintVersion()
			os.Exit(0)
		}
	}

	// Check for --compact flag
	var compactMode bool
	var remainingArgs []string
	for _, arg := range os.Args[1:] {
		if arg == "--compact" {
			compactMode = true
		} else {
			remainingArgs = append(remainingArgs, arg)
		}
	}

	// If --compact mode, run compaction and exit
	if compactMode {
		fmt.Println("🔄 Running session compaction...")
		if err := runCompactMode(); err != nil {
			fmt.Printf("❌ Compaction failed: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("✅ Compaction completed successfully")
		os.Exit(0)
	}

	// Ensure AGENT.md exists in ~/.dogclaw
	if err := config.EnsureAgentMarkdownExists(); err != nil {
		fmt.Printf("⚠️  Warning: Failed to ensure AGENT.md exists: %v\n", err)
	}

	args := remainingArgs

	// Parse flags (--config) and get remaining args
	var configPath string
	var modeArgs []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-c" {
			if i+1 < len(args) {
				configPath = args[i+1]
				i++ // Skip the next arg which is the path
			} else {
				fmt.Println("❌ Error: --config requires a path argument")
				printUsage()
				os.Exit(1)
			}
		} else if strings.HasPrefix(arg, "--config=") {
			configPath = strings.TrimPrefix(arg, "--config=")
		} else if strings.HasPrefix(arg, "-c=") {
			configPath = strings.TrimPrefix(arg, "-c=")
		} else {
			modeArgs = append(modeArgs, arg)
		}
	}

	if len(modeArgs) == 0 {
		fmt.Println("❌ Error: Mode is required")
		printUsage()
		os.Exit(1)
	}

	startupMode := StartupMode(modeArgs[0])
	if startupMode != ModeAgent && startupMode != ModeGateway && startupMode != ModeOnboard {
		fmt.Printf("❌ Error: Invalid mode '%s'. Must be 'agent', 'gateway' or 'onboard'\n", modeArgs[0])
		printUsage()
		os.Exit(1)
	}

	// Set custom config path if specified
	if configPath != "" {
		config.SetConfigPath(configPath)
		fmt.Printf("📁 Using config file: %s\n", configPath)
	}

	// Load persistent settings
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

	// Get API key: prioritize from settings, fallback to environment variables
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	}
	if apiKey == "" {
		fmt.Println("⚠️  Warning: No API key found in settings or environment (ANTHROPIC_API_KEY / OPENROUTER_API_KEY)")
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
		stopChan := make(chan os.Signal, 12)
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
	// 检查是否需要重启，如果有标志文件则删除
	restartFlagPath := getRestartFlagPath()
	if _, err := os.Stat(restartFlagPath); err == nil {
		// 标志文件存在，删除它
		os.Remove(restartFlagPath)
	}

	fmt.Println("🦞 DogClaw - AI Assistant (Go Implementation)")
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
	qe := engineFactory("cli")

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
func newEngineFactory(cfg *config.Config, settings *config.Settings, registry *channel.Registry) func(channelName string) *query.QueryEngine {
	return func(channelName string) *query.QueryEngine {
		client := api.NewClient(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Provider)
		toolList := buildTools(registry)
		cwd, _ := os.Getwd()
		loadedSkills, _ := skills.DiscoverSkills(cwd)
		systemPrompt := query.BuildSystemPrompt(toolList, loadedSkills, "")
		qe := query.NewQueryEngine(client, toolList, systemPrompt, cfg.MaxTurns)
		qe.SetVerbose(cfg.Verbose)
		qe.SetShowToolUsageInReply(cfg.ShowToolUsageInReply)
		qe.SetShowThinkingInLog(cfg.ShowThinkingInLog)
		// Set channel name to isolate session history
		qe.SetChannelName(channelName)
		if cfg.MaxBudgetUSD > 0 {
			qe.SetMaxBudget(cfg.MaxBudgetUSD)
		}
		if cfg.MaxTokens > 0 {
			qe.SetMaxTokens(cfg.MaxTokens)
		}
		if cfg.MaxContextLength > 0 {
			qe.SetMaxContextLength(cfg.MaxContextLength)
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
	sig, e := <-stopChan
	fmt.Printf("\n📥 Received %v, shutting down... err %v\n", sig, e)
	cancel()
	for _, ch := range channels {
		ch.Stop()
	}
	fmt.Println("👋 All channels stopped.")
}

// runCompactMode runs the session compaction in standalone mode
func runCompactMode() error {
	// Load config
	settings, err := config.LoadSettings()
	if err != nil {
		return fmt.Errorf("failed to load settings: %w", err)
	}

	cfg, err := config.ConfigFromSettings(settings)
	if err != nil {
		return fmt.Errorf("failed to create config: %w", err)
	}

	// Get API key: prioritize from settings, fallback to environment variables
	apiKey := strings.TrimSpace(cfg.APIKey)
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))
	}
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENROUTER_API_KEY"))
	}
	if apiKey == "" {
		return fmt.Errorf("no API key found in settings or environment (ANTHROPIC_API_KEY / OPENROUTER_API_KEY)")
	}
	cfg.APIKey = apiKey

	// Create API client
	client := api.NewClient(cfg.APIKey, cfg.Model, cfg.BaseURL, cfg.Provider)

	// Create a minimal engine just for compaction
	// We need to create an engine and resume the latest session
	toolList := buildTools(nil)
	systemPrompt := query.BuildSystemPrompt(toolList, nil, "")
	qe := query.NewQueryEngine(client, toolList, systemPrompt, cfg.MaxTurns)
	qe.SetVerbose(true)

	// Resume the latest session
	fmt.Println("📂 Resuming latest session...")
	if err := qe.AutoResumeLatestSession(context.Background()); err != nil {
		return fmt.Errorf("failed to resume session: %w", err)
	}

	// Check current token count
	messages := qe.GetMessages()
	tokenCount := compact.EstimateMessagesTokenCount(messages)
	fmt.Printf("📊 Current session: %d messages, ~%d tokens\n", len(messages), tokenCount)

	// Force compaction (disable threshold check temporarily)
	// We'll temporarily lower the threshold to force compaction
	compactConfig := compact.DefaultAutoCompactConfig()
	compactConfig.ThresholdRatio = 0.0 // Force compaction even if under threshold

	fmt.Println("🔨 Compacting session...")
	result, err := compact.CompactMessages(context.Background(), client, messages, systemPrompt, compactConfig)
	if err != nil {
		return fmt.Errorf("compaction failed: %w", err)
	}

	if result == nil {
		fmt.Println("ℹ️  No compaction needed (not enough messages)")
		return nil
	}

	fmt.Printf("✅ Compaction complete:\n")
	fmt.Printf("   - Messages: %d -> %d\n", result.OriginalMessageCount, result.CompactedMessageCount)
	fmt.Printf("   - Tokens: %d -> %d\n", result.PreCompactTokenCount, result.PostCompactTokenCount)

	// Apply the compaction result
	compactedMessages := compact.ApplyCompactResult(messages, result)

	// Now we need to save these compacted messages back to the transcript
	// Use the transcript package directly

	// First, find the latest session
	pm, err := transcript.NewProjectManager("")
	if err != nil {
		return fmt.Errorf("failed to create transcript manager: %w", err)
	}

	sessions, err := pm.ListSessions()
	if err != nil {
		return fmt.Errorf("failed to list sessions: %w", err)
	}

	if len(sessions) == 0 {
		return fmt.Errorf("no sessions found")
	}

	// Find the most recent session by checking file modification times
	var latestSession transcript.SessionInfo
	var latestModTime int64
	for _, s := range sessions {
		info, err := os.Stat(s.FilePath)
		if err != nil {
			continue
		}
		modTime := info.ModTime().UnixMilli()
		if latestSession.SessionID == "" || modTime > latestModTime {
			latestSession = s
			latestModTime = modTime
		}
	}

	if latestSession.SessionID == "" {
		return fmt.Errorf("no valid sessions found")
	}

	fmt.Printf("💾 Saving compacted session: %s\n", latestSession.SessionID)

	// Get the transcript file
	tf := pm.GetTranscriptFile(latestSession.SessionID, latestSession.ProjectPath)

	// Serialize and save the compacted session
	compactedData, err := compact.SerializeCompactedSession(result, compactedMessages)
	if err != nil {
		return fmt.Errorf("failed to serialize compacted session: %w", err)
	}

	if err := tf.WriteMetadata(string(transcript.MetadataCompaction), compactedData); err != nil {
		return fmt.Errorf("failed to save compacted session: %w", err)
	}

	fmt.Println("✅ Compacted session saved successfully")

	return nil
}
