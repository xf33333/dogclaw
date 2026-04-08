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
	// Increase timeout to 10m to match engine heartbeat timeout
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	// Start Typing keep-alive in background to prevent session timeout during long turns
	typingCtx, typingCancel := context.WithCancel(ctx)
	go func() {
		defer typingCancel()
		// Get typing ticket for keep-alive
		ticket, err := c.getTypingTicket(typingCtx, session.SenderID)
		if err != nil || ticket == "" {
			return
		}

		// Initial typing indicator
		_ = c.sendTypingStatus(typingCtx, session.SenderID, ticket, TypingStatusTyping)

		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-typingCtx.Done():
				// Send cancel typing status when the turn is over
				stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = c.sendTypingStatus(stopCtx, session.SenderID, ticket, TypingStatusCancel)
				stopCancel()
				return
			case <-ticker.C:
				_ = c.sendTypingStatus(typingCtx, session.SenderID, ticket, TypingStatusTyping)
			}
		}
	}()

	err := session.Engine.SubmitMessage(ctx, prompt)
	typingCancel() // Stop keep-alive as soon as message is submitted

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

	const maxRetries = 3
	for attempt := 0; attempt <= maxRetries; attempt++ {
		contextToken, ok := getContextToken()
		if !ok {
			return
		}

		resp, err := c.api.SendMessage(ctx, buildReq(contextToken))
		if err != nil {
			logger.Errorf("[weixin] (Attempt %d) Failed to send message to %s: %v", attempt+1, toUserID, err)
			return
		}

		if resp.Ret == 0 && resp.Errcode == 0 {
			logger.Infof("[weixin] Message sent to %s (%d chars) on attempt %d", toUserID, len(content), attempt+1)
			return
		}

		logger.Errorf("[weixin] (Attempt %d) SendMessage API error for %s: ret=%d errcode=%d errmsg=%s",
			attempt+1, toUserID, resp.Ret, resp.Errcode, resp.Errmsg)

		// Handle specific errors
		if isSessionExpiredStatus(resp.Ret, resp.Errcode) {
			logger.Warnf("[weixin] Session expired, deleting context token for %s and retrying (attempt %d/%d)...", toUserID, attempt+1, maxRetries)
			c.contextTokens.Delete(toUserID)
			continue
		}

		if resp.Ret == -2 {
			logger.Warnf("[weixin] Token invalid (ret=-2) for user %s, deleting cache and retrying (attempt %d/%d)...", toUserID, attempt+1, maxRetries)
			c.contextTokens.Delete(toUserID)
			if attempt < maxRetries {
				time.Sleep(1 * time.Second)
				continue
			}
		}

		// For other errors or if retries exhausted, give up
		break
	}
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
// Send implements channel.Sender
func (c *WeixinChannel) Send(ctx context.Context, chatID, message string) error {
	if message == "" {
		return nil
	}

	if chatID == "" {
		// Broadcast to all known active sessions
		count := 0
		c.sessions.Range(func(key, value any) bool {
			sess := value.(*ChatSession)
			c.sendMessage(ctx, sess.SenderID, message)
			count++
			return true
		})
		if count == 0 {
			logger.Warnf("[weixin] Broadcast requested but no active sessions found")
		}
		return nil
	}

	c.sendMessage(ctx, chatID, message)
	return nil
}
