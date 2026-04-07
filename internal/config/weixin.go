package config

import "os"

// WeixinSettings holds configuration for the Weixin channel
type WeixinSettings struct {
	Enabled   bool     `json:"enabled,omitempty"`    // Whether Weixin channel is enabled
	Token     string   `json:"token,omitempty"`      // Weixin bot token
	AccountID string   `json:"account_id,omitempty"` // Weixin account ID
	BaseURL   string   `json:"base_url,omitempty"`   // API base URL
	AllowFrom []string `json:"allow_from,omitempty"` // Allowed user IDs
}

// WeixinSettingsFromEnv returns WeixinSettings by merging ~/.dogclaw/setting.json with env vars.
func WeixinSettingsFromEnv(s *Settings) WeixinSettings {
	var wx WeixinSettings
	if s.Channel != nil && s.Channel.Weixin != nil {
		wx = *s.Channel.Weixin
	}

	// Env vars override file config
	if v := os.Getenv("WEIXIN_TOKEN"); v != "" {
		wx.Token = v
	}
	if v := os.Getenv("WEIXIN_ACCOUNT_ID"); v != "" {
		wx.AccountID = v
	}
	if v := os.Getenv("WEIXIN_BASE_URL"); v != "" {
		wx.BaseURL = v
	}

	// Mark as enabled if token is configured
	if wx.Token != "" {
		wx.Enabled = true
	}

	return wx
}
