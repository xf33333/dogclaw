package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"dogclaw/internal/config"
	"dogclaw/pkg/channel/weixin"
)

// RunOnboard runs the interactive onboarding process to configure setting.json
func RunOnboard(ctx context.Context, settings *config.Settings) error {
	// 初始化技能目录
	if err := InitializeSkills(); err != nil {
		fmt.Printf("Warning: failed to initialize skills: %v\n", err)
	}
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("🚀 Welcome to DogClaw Onboarding!")
	fmt.Println("This process will help you configure your models and messaging channels.")
	fmt.Println()

	// Step 1: Model Configuration
	fmt.Println("--- Step 1: Model Configuration ---")
	activeModel, err := settings.GetActive()
	if err != nil {
		fmt.Printf("⚠️  No active model found: %v\n", err)
	} else {
		fmt.Printf("Current Active Model: %s (%s, %s)\n", activeModel.Alias, activeModel.Provider, activeModel.Model)
		if activeModel.APIKey != "" && len(activeModel.APIKey) > 10 {
			keyPreview := activeModel.APIKey[:5] + "..." + activeModel.APIKey[len(activeModel.APIKey)-4:]
			fmt.Printf("Current API Key: %s\n", keyPreview)
		} else {
			fmt.Println("Current API Key: Not set")
		}
	}

	fmt.Print("Do you want to modify model settings? (y/N): ")
	ans, _ := reader.ReadString('\n')
	if strings.ToLower(strings.TrimSpace(ans)) == "y" {
		fmt.Print("Enter Provider (e.g., openrouter, anthropic, openai): ")
		provider, _ := reader.ReadString('\n')
		settings.Providers[0].Provider = strings.TrimSpace(provider)

		fmt.Print("Enter Model Name (e.g., anthropic/claude-3.5-sonnet): ")
		modelName, _ := reader.ReadString('\n')
		settings.Providers[0].Model = strings.TrimSpace(modelName)

		fmt.Print("Enter Base URL (leave empty for default): ")
		baseURL, _ := reader.ReadString('\n')
		settings.Providers[0].URL = strings.TrimSpace(baseURL)

		fmt.Print("Enter API Key: ")
		apiKey, _ := reader.ReadString('\n')
		settings.Providers[0].APIKey = strings.TrimSpace(apiKey)

		fmt.Println("✅ Model settings updated.")
	} else {
		fmt.Println("Keeping current model settings.")
	}
	fmt.Println()

	// Step 2: Channel Configuration
	fmt.Println("--- Step 2: Channel Configuration ---")
	fmt.Println("Which channel would you like to configure?")
	fmt.Println("1) QQ")
	fmt.Println("2) Weixin (WeChat)")
	fmt.Println("3) Ignore (don't configure any channel)")
	fmt.Print("Select (1, 2 or 3): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)

	if settings.Channel == nil {
		settings.Channel = &config.ChannelSettings{}
	}

	switch choice {
	case "1":
		fmt.Println("\n--- Configuring QQ Channel ---")
		if settings.Channel.QQ == nil {
			settings.Channel.QQ = &config.QQSettings{}
		}
		fmt.Print("Enter QQ AppID: ")
		appID, _ := reader.ReadString('\n')
		settings.Channel.QQ.AppID = strings.TrimSpace(appID)

		fmt.Print("Enter QQ AppSecret: ")
		appSecret, _ := reader.ReadString('\n')
		settings.Channel.QQ.AppSecret = strings.TrimSpace(appSecret)
		settings.Channel.QQ.Enabled = true
		fmt.Println("✅ QQ channel configured.")

	case "2":
		fmt.Println("\n--- Configuring Weixin Channel ---")
		fmt.Println("Generating QR code for login...")

		// Trigger Weixin Login Flow
		opts := weixin.AuthFlowOpts{
			BotType: "3", // Default iLink Bot
			Timeout: 5 * 10 * time.Minute,
		}

		botToken, _, accountID, baseURL, err := weixin.PerformLoginInteractive(ctx, opts)
		if err != nil {
			return fmt.Errorf("weixin login failed: %w", err)
		}

		if settings.Channel.Weixin == nil {
			settings.Channel.Weixin = &config.WeixinSettings{}
		}
		settings.Channel.Weixin.Token = botToken
		settings.Channel.Weixin.AccountID = accountID
		settings.Channel.Weixin.BaseURL = baseURL
		settings.Channel.Weixin.Enabled = true

		fmt.Println("\n✅ Weixin channel configured successfully!")

	case "3":
		fmt.Println("\n--- Skipping Channel Configuration ---")
		fmt.Println("✅ No channels will be configured. You can configure them later by running 'dogclaw onboard'.")

	default:
		fmt.Println("Invalid choice. Skipping channel configuration.")
	}

	// Save settings
	if err := settings.SaveSettings(); err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}

	fmt.Println("\n🎉 Onboarding complete! Your settings have been saved to ~/.dogclaw/setting.json")
	fmt.Println("You can now run 'dogclaw gateway' to start your bot.")

	return nil
}

// Added time import since I used time.Minute
func init() {
	// Dummy to ensure compile if I missed something
}
