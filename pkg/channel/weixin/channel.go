package weixin

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"dogclaw/internal/config"
	"dogclaw/internal/logger"
	"dogclaw/pkg/channel"
	"dogclaw/pkg/query"
)

// WeixinChannel is the Weixin channel implementation over Tencent iLink REST API.
type WeixinChannel struct {
	api           *ApiClient
	config        config.WeixinSettings
	ctx           context.Context
	cancel        context.CancelFunc
	sessions      sync.Map // chatID → *ChatSession
	contextTokens sync.Map // from_user_id → context_token

	typingMu    sync.Mutex
	typingCache map[string]typingTicketCacheEntry
	pauseMu     sync.Mutex
	pauseUntil  time.Time

	syncBufPath       string
	contextTokensPath string
}

// ChatSession wraps a QueryEngine for a single Weixin conversation
type ChatSession struct {
	Engine   *query.QueryEngine
	SenderID string
}

// NewWeixinChannel creates a new WeixinChannel from config.
func NewWeixinChannel(cfg config.WeixinSettings) (*WeixinChannel, error) {
	api, err := NewApiClient(cfg.BaseURL, cfg.Token, "")
	if err != nil {
		return nil, fmt.Errorf("weixin: failed to create API client: %w", err)
	}

	return &WeixinChannel{
		api:               api,
		config:            cfg,
		typingCache:       make(map[string]typingTicketCacheEntry),
		syncBufPath:       buildWeixinSyncBufPath(cfg),
		contextTokensPath: buildWeixinContextTokensPath(cfg),
	}, nil
}

// Info implements channel.Interface
func (c *WeixinChannel) Info() channel.Info {
	return channel.Info{Name: "weixin"}
}

// Start implements channel.Interface
func (c *WeixinChannel) Start(ctx context.Context, factory channel.EngineFactory) error {
	logger.Info("[weixin] Starting Weixin channel")
	c.ctx, c.cancel = context.WithCancel(ctx)

	c.restoreContextTokens()
	go c.pollLoop(c.ctx, factory)

	logger.Info("[weixin] Weixin channel started")
	return nil
}

// Stop implements channel.Interface
func (c *WeixinChannel) Stop() {
	logger.Info("[weixin] Stopping Weixin channel")
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *WeixinChannel) restoreContextTokens() {
	tokens, err := loadContextTokens(c.contextTokensPath)
	if err != nil {
		logger.Warnf("[weixin] Failed to load persisted context tokens from %s: %v", c.contextTokensPath, err)
		return
	}
	if len(tokens) == 0 {
		return
	}
	for userID, token := range tokens {
		c.contextTokens.Store(userID, token)
	}
	logger.Infof("[weixin] Restored %d context tokens from disk", len(tokens))
}

func (c *WeixinChannel) persistContextTokens() {
	tokens := make(map[string]string)
	c.contextTokens.Range(func(k, v any) bool {
		if userID, ok := k.(string); ok {
			if token, ok := v.(string); ok {
				tokens[userID] = token
			}
		}
		return true
	})
	if err := saveContextTokens(c.contextTokensPath, tokens); err != nil {
		logger.Warnf("[weixin] Failed to persist context tokens: %v", err)
	}
}

func (c *WeixinChannel) pollLoop(ctx context.Context, factory channel.EngineFactory) {
	const (
		defaultPollTimeoutMs = 35_000
		retryDelay           = 2 * time.Second
		backoffDelay         = 30 * time.Second
		maxConsecutiveFails  = 3
	)

	consecutiveFails := 0
	getUpdatesBuf, err := loadGetUpdatesBuf(c.syncBufPath)
	if err != nil {
		logger.Warnf("[weixin] Failed to load persisted get_updates_buf: %v", err)
		getUpdatesBuf = ""
	}

	nextTimeoutMs := defaultPollTimeoutMs

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if err := c.waitWhileSessionPaused(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}

		pollCtx, pollCancel := context.WithTimeout(ctx, time.Duration(nextTimeoutMs+5000)*time.Millisecond)
		resp, err := c.api.GetUpdates(pollCtx, GetUpdatesReq{
			GetUpdatesBuf: getUpdatesBuf,
		})
		pollCancel()

		if err != nil {
			if ctx.Err() != nil {
				return
			}
			consecutiveFails++
			logger.Warnf("[weixin] getUpdates failed (attempt %d): %v", consecutiveFails, err)
			if consecutiveFails >= maxConsecutiveFails {
				consecutiveFails = 0
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoffDelay):
				}
			} else {
				select {
				case <-ctx.Done():
					return
				case <-time.After(retryDelay):
				}
			}
			continue
		}

		if isSessionExpiredStatus(resp.Ret, resp.Errcode) {
			remaining := c.pauseSession("getupdates", resp.Ret, resp.Errcode, resp.Errmsg)
			select {
			case <-ctx.Done():
				return
			case <-time.After(remaining):
			}
			continue
		}

		if resp.Errcode != 0 || resp.Ret != 0 {
			consecutiveFails++
			logger.Errorf("[weixin] getUpdates API error: ret=%d code=%d msg=%s", resp.Ret, resp.Errcode, resp.Errmsg)
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			continue
		}

		consecutiveFails = 0
		if resp.LongpollingTimeoutMs > 0 {
			nextTimeoutMs = resp.LongpollingTimeoutMs
		}

		if resp.GetUpdatesBuf != "" {
			getUpdatesBuf = resp.GetUpdatesBuf
			if err := saveGetUpdatesBuf(c.syncBufPath, getUpdatesBuf); err != nil {
				logger.Warnf("[weixin] Failed to persist get_updates_buf: %v", err)
			}
		}

		for _, msg := range resp.Msgs {
			c.handleInboundMessage(ctx, msg, factory)
		}
	}
}

func (c *WeixinChannel) handleInboundMessage(ctx context.Context, msg WeixinMessage, factory channel.EngineFactory) {
	fromUserID := msg.FromUserID
	if fromUserID == "" {
		return
	}

	// Ensure we have a valid context token for this user
	if msg.ContextToken == "" {
		logger.Warnf("[weixin] Message from %s has no context_token, fetching via GetConfig...", fromUserID)
		resp, err := c.api.GetConfig(ctx, GetConfigReq{
			IlinkUserID:  fromUserID,
			ContextToken: "",
		})
		if err != nil || resp.Ret != 0 || resp.Errcode != 0 {
			logger.Errorf("[weixin] Failed to get context token for user %s: %v", fromUserID, err)
			return
		}
		token := strings.TrimSpace(resp.TypingTicket)
		if token == "" {
			logger.Errorf("[weixin] GetConfig did not return a valid token for user %s", fromUserID)
			return
		}
		c.contextTokens.Store(fromUserID, token)
		c.persistContextTokens()
		msg.ContextToken = token
	} else {
		c.contextTokens.Store(fromUserID, msg.ContextToken)
		c.persistContextTokens()
	}

	var parts []string
	for _, item := range msg.ItemList {
		if item.Type == MessageItemTypeText && item.TextItem != nil {
			parts = append(parts, item.TextItem.Text)
		}
		// Add other media types as needed
	}

	content := strings.Join(parts, "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	if !c.isAllowedSender(fromUserID) {
		return
	}

	session := c.getOrCreateSession(ctx, fromUserID, factory)

	go func() {
		reply := c.getReply(ctx, session, content)
		if reply != "" {
			c.sendMessage(ctx, fromUserID, reply)
		}
	}()
}

func (c *WeixinChannel) getOrCreateSession(ctx context.Context, chatID string, factory channel.EngineFactory) *ChatSession {
	if v, ok := c.sessions.Load(chatID); ok {
		return v.(*ChatSession)
	}

	engine := factory()
	// TextCallback: fires for every LLM text block (both intermediate turns with tools
	// and the final text-only reply). This ensures all LLM commentary reaches the user.
	engine.TextCallback = func(text string) {
		c.sendMessage(c.ctx, chatID, text)
	}
	// ToolCallCallback: sends a brief notification when a tool is called
	engine.ToolCallCallback = func(toolName, summary string) {
		c.sendMessage(c.ctx, chatID, fmt.Sprintf("🔧 %s", summary))
	}
	engine.AutoResumeLatestSession(context.Background())

	sess := &ChatSession{
		Engine:   engine,
		SenderID: chatID,
	}
	c.sessions.Store(chatID, sess)
	return sess
}

// getReply runs SubmitMessage and returns a non-empty string only on API errors.
// Normal LLM text (including slash command output) is delivered to the channel
// in real-time via TextCallback registered in getOrCreateSession, so this
// function returns "" for successful runs to avoid duplicate messages.
func (c *WeixinChannel) getReply(ctx context.Context, session *ChatSession, prompt string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	err := session.Engine.SubmitMessage(ctx, prompt)
	if err != nil {
		// TextCallback may not have fired; send the error back explicitly.
		return fmt.Sprintf("⚠️ 处理错误: %v", err)
	}

	// TextCallback already pushed every text block (LLM reply + slash output) to
	// the channel, so return "" here to avoid sending a duplicate message.
	return ""
}

func (c *WeixinChannel) sendMessage(ctx context.Context, toUserID, content string) {
	if content == "" {
		return
	}

	if err := c.ensureSessionActive(); err != nil {
		logger.Warnf("[weixin] Skip sending message: %v", err)
		return
	}

	// getContextToken returns the current context_token for the user,
	// refreshing via GetConfig if not found.
	getContextToken := func() (string, bool) {
		if ct, ok := c.contextTokens.Load(toUserID); ok {
			return ct.(string), true
		}
		logger.Warnf("[weixin] Context token not found for user %s, fetching via GetConfig...", toUserID)
		resp, err := c.api.GetConfig(ctx, GetConfigReq{IlinkUserID: toUserID})
		if err != nil || resp.Ret != 0 || resp.Errcode != 0 {
			logger.Errorf("[weixin] GetConfig failed for user %s: err=%v ret=%d code=%d",
				toUserID, err, resp.Ret, resp.Errcode)
			return "", false
		}
		if resp.TypingTicket == "" {
			logger.Errorf("[weixin] GetConfig returned empty token for user %s", toUserID)
			return "", false
		}
		c.contextTokens.Store(toUserID, resp.TypingTicket)
		c.persistContextTokens()
		return resp.TypingTicket, true
	}

	contextToken, ok := getContextToken()
	if !ok {
		return
	}

	buildReq := func(token string) SendMessageReq {
		return SendMessageReq{
			Msg: WeixinMessage{
				ToUserID:     toUserID,
				ClientID:     uuid.New().String(),
				ContextToken: token,
				ItemList: []MessageItem{
					{
						Type:     MessageItemTypeText,
						TextItem: &TextItem{Text: content},
					},
				},
			},
		}
	}

	resp, err := c.api.SendMessage(ctx, buildReq(contextToken))
	if err != nil {
		logger.Errorf("[weixin] Failed to send message to %s: %v", toUserID, err)
		return
	}

	// Check business-layer error codes (API can return HTTP 200 with non-zero ret/errcode)
	if resp.Ret != 0 || resp.Errcode != 0 {
		logger.Errorf("[weixin] SendMessage API error for %s: ret=%d errcode=%d errmsg=%s",
			toUserID, resp.Ret, resp.Errcode, resp.Errmsg)

		// If session expired, refresh token and retry once
		if isSessionExpiredStatus(resp.Ret, resp.Errcode) {
			logger.Warnf("[weixin] Session expired, refreshing context token for %s and retrying...", toUserID)
			c.contextTokens.Delete(toUserID)
			newToken, ok := getContextToken()
			if !ok {
				logger.Errorf("[weixin] Token refresh failed for %s, message dropped", toUserID)
				return
			}
			resp2, err2 := c.api.SendMessage(ctx, buildReq(newToken))
			if err2 != nil {
				logger.Errorf("[weixin] Retry send failed for %s: %v", toUserID, err2)
				return
			}
			if resp2.Ret != 0 || resp2.Errcode != 0 {
				logger.Errorf("[weixin] Retry SendMessage still failed for %s: ret=%d errcode=%d errmsg=%s",
					toUserID, resp2.Ret, resp2.Errcode, resp2.Errmsg)
			} else {
				logger.Infof("[weixin] Message sent to %s after token refresh (retry)", toUserID)
			}
		}
		return
	}

	logger.Infof("[weixin] Message sent to %s (%d chars)", toUserID, len(content))
}

func (c *WeixinChannel) isAllowedSender(senderID string) bool {
	if len(c.config.AllowFrom) == 0 {
		return true
	}
	for _, id := range c.config.AllowFrom {
		if id == senderID {
			return true
		}
	}
	return false
}
