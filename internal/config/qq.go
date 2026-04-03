// QQ integration config for dogclaw
package config

import "os"

// QQSettings holds configuration for the QQ channel
type QQSettings struct {
	Enabled      bool     `json:"enabled,omitempty"`       // Whether QQ channel is enabled
	AppID        string   `json:"app_id,omitempty"`        // QQ bot app ID
	AppSecret    string   `json:"app_secret,omitempty"`    // QQ bot app secret
	AllowFrom    []string `json:"allow_from,omitempty"`    // Allowed user IDs (empty = allow all)
	SendMarkdown bool     `json:"send_markdown,omitempty"` // Enable markdown messages
}

// QQEnvConfig builds QQ config from env vars (legacy, for backward compatibility)
func QQEnvConfig() QQSettings {
	return QQSettings{
		AppID:     os.Getenv("QQ_APP_ID"),
		AppSecret: os.Getenv("QQ_APP_SECRET"),
		AllowFrom: nil, // allow all when set via env
	}
}

// QQSettingsFromEnv returns QQSettings by merging ~/.dogclaw/setting.json with env vars.
// Environment variables take precedence if set.
func QQSettingsFromEnv(s *Settings) QQSettings {
	var qq QQSettings
	if s.Channel != nil && s.Channel.QQ != nil {
		qq = *s.Channel.QQ
	}

	// Env vars override file config
	if v := os.Getenv("QQ_APP_ID"); v != "" {
		qq.AppID = v
	}
	if v := os.Getenv("QQ_APP_SECRET"); v != "" {
		qq.AppSecret = v
	}
	if v := os.Getenv("QQ_SEND_MARKDOWN"); v == "true" {
		qq.SendMarkdown = true
	} else if v == "false" {
		qq.SendMarkdown = false
	}

	// Mark as enabled if any credentials are configured
	if qq.AppID != "" && qq.AppSecret != "" {
		qq.Enabled = true
	}

	return qq
}
