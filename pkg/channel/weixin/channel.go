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

	retryMu    sync.Mutex
	retryQueue []retryEntry

	sendMu sync.Map // userID → *sync.Mutex for per-user send rate limiting

	// Message queue for batching (per user)
	queueMu      sync.Mutex
	messageQueue map[string]*userMessageQueue // userID → queue
}

type userMessageQueue struct {
	messages     []string
	sendCount    int // messages sent with current token
	waitForFlush bool
}

type retryEntry struct {
	ToUserID string
	Content  string
	Retries  int
	NextTry  time.Time
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
		messageQueue:      make(map[string]*userMessageQueue),
	}, nil
}

// Info implements channel.Interface
func (c *WeixinChannel) Info() channel.Info {
	return channel.Info{Name: "weixin"}
}

// SystemPrompt implements channel.Interface
func (c *WeixinChannel) SystemPrompt() string {
	return `## 微信频道能力说明

### 文件接收
- 用户发送的文件会自动下载
- 支持接收图片、语音、视频、文件等附件

### 消息发送
- 支持发送文本消息
- 支持发送图片、文件等媒体类型

### 注意事项
- 微信频道通过企业微信接口实现`
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

		c.processRetryQueue(ctx)
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
		token := strings.TrimSpace(resp.ContextToken)
		if token == "" {
			token = strings.TrimSpace(resp.TypingTicket)
		}
		if token == "" {
			logger.Errorf("[weixin] GetConfig did not return a valid token for user %s", fromUserID)
			return
		}
		c.contextTokens.Store(fromUserID, token)
		c.persistContextTokens()
		c.resetQueueCount(fromUserID)
		msg.ContextToken = token
	} else {
		c.contextTokens.Store(fromUserID, msg.ContextToken)
		c.persistContextTokens()
		c.resetQueueCount(fromUserID)
	}

	var parts []string
	for _, item := range msg.ItemList {
		if item.Type == MessageItemTypeText && item.TextItem != nil {
			parts = append(parts, item.TextItem.Text)
		}
	}

	content := strings.Join(parts, "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}

	if !c.isAllowedSender(fromUserID) {
		return
	}

	if testMode {
		c.handleTestMessage(ctx, fromUserID)
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

	engine := factory("weixin")
	// TextCallback: fires for every LLM text block (both intermediate turns with tools
	// and the final text-only reply).
	// Uses message queue to batch messages (max 10 per context token).
	engine.TextCallback = func(text string, isFinish bool) {
		c.queueMessage(ctx, chatID, text, isFinish)
	}
	// ToolCallCallback: sends a brief notification when a tool is called
	// Uses message queue to batch messages.
	engine.ToolCallCallback = func(toolName, summary string) {
		c.queueMessage(ctx, chatID, fmt.Sprintf("🔧 %s", summary), false)
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
	ctx, cancel := context.WithTimeout(ctx, 24*time.Hour)
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

const (
	maxMessagesPerToken = 10
	flushThreshold      = 3
	warningThreshold    = 9
)

func (c *WeixinChannel) queueMessage(ctx context.Context, toUserID, text string, isFinish bool) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	queue, ok := c.messageQueue[toUserID]
	if !ok {
		queue = &userMessageQueue{
			messages:  []string{},
			sendCount: 0,
		}
		c.messageQueue[toUserID] = queue
	}

	if text != "" {
		queue.messages = append(queue.messages, text)
	}

	if isFinish {
		c.flushQueue(ctx, toUserID, queue)
		return
	}

	if queue.waitForFlush {
		return
	}

	if len(queue.messages) >= flushThreshold {
		c.flushQueue(ctx, toUserID, queue)
	}
}

func (c *WeixinChannel) flushQueue(ctx context.Context, toUserID string, queue *userMessageQueue) {
	if len(queue.messages) == 0 {
		return
	}

	if queue.sendCount >= warningThreshold && queue.sendCount < maxMessagesPerToken {
		queue.waitForFlush = true
		logger.Infof("[weixin] Message queue for %s at %d/%d, waiting for finish", toUserID, queue.sendCount, maxMessagesPerToken)
		return
	}

	if queue.sendCount >= maxMessagesPerToken {
		logger.Warnf("[weixin] Message queue for %s exceeded max %d messages, flushing anyway", toUserID, maxMessagesPerToken)
	}

	merged := strings.Join(queue.messages, "\n\n")
	queue.messages = nil

	go c.sendMessage(c.ctx, toUserID, merged)
	queue.sendCount++

	logger.Infof("[weixin] Flushed queue for %s, sendCount now %d", toUserID, queue.sendCount)
}

func (c *WeixinChannel) resetQueueCount(toUserID string) {
	c.queueMu.Lock()
	defer c.queueMu.Unlock()

	if queue, ok := c.messageQueue[toUserID]; ok {
		queue.sendCount = 0
		queue.waitForFlush = false
		logger.Infof("[weixin] Reset send count for %s", toUserID)
	}
}

func (c *WeixinChannel) sendMessage(ctx context.Context, toUserID, content string) {
	if content == "" {
		return
	}

	umuInterface, _ := c.sendMu.LoadOrStore(toUserID, &sync.Mutex{})
	umu := umuInterface.(*sync.Mutex)
	umu.Lock()
	defer umu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

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
		logger.Warnf("[weixin] Context token not found for user %s, fetching via GetConfig...", toUserID)
		token := strings.TrimSpace(resp.ContextToken)
		if token == "" {
			token = strings.TrimSpace(resp.TypingTicket)
		}
		if token == "" {
			logger.Errorf("[weixin] GetConfig returned empty token for user %s", toUserID)
			return "", false
		}
		logger.Infof("[weixin] save %s token %s", toUserID, token)
		c.contextTokens.Store(toUserID, token)
		c.persistContextTokens()
		time.Sleep(500 * time.Millisecond)
		return token, true
	}

	buildReq := func(token string) SendMessageReq {
		return SendMessageReq{
			Msg: WeixinMessage{
				ToUserID:     toUserID,
				ClientID:     "dogclaw-" + uuid.New().String(),
				MessageType:  MessageTypeBot,
				MessageState: MessageStateFinish,
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

		logger.Infof("[weixin] Sending message to %s using token: %s... (attempt %d)", toUserID, contextToken[:20], attempt+1)
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

		if isSessionExpiredStatus(resp.Ret, resp.Errcode) {
			logger.Warnf("[weixin] Session expired, deleting context token for %s and retrying (attempt %d/%d)...", toUserID, attempt+1, maxRetries)
			c.contextTokens.Delete(toUserID)
			continue
		}

		break
	}

	c.addToRetryQueue(toUserID, content)
}

func (c *WeixinChannel) addToRetryQueue(toUserID, content string) {
	c.retryMu.Lock()
	defer c.retryMu.Unlock()
	const maxQueueSize = 100
	if len(c.retryQueue) >= maxQueueSize {
		logger.Warnf("[weixin] Retry queue full, dropping oldest entry for %s", toUserID)
		c.retryQueue = c.retryQueue[1:]
	}
	c.retryQueue = append(c.retryQueue, retryEntry{
		ToUserID: toUserID,
		Content:  content,
		Retries:  0,
		NextTry:  time.Now().Add(5 * time.Second),
	})
	logger.Infof("[weixin] Added message to retry queue for %s (queue size: %d)", toUserID, len(c.retryQueue))
}

func (c *WeixinChannel) processRetryQueue(ctx context.Context) {
	c.retryMu.Lock()
	if len(c.retryQueue) == 0 {
		c.retryMu.Unlock()
		return
	}
	now := time.Now()
	var pending []retryEntry
	for _, entry := range c.retryQueue {
		if now.Before(entry.NextTry) {
			pending = append(pending, entry)
			continue
		}
		c.retryMu.Unlock()
		c.sendMessage(ctx, entry.ToUserID, entry.Content)
		c.retryMu.Lock()
	}
	c.retryQueue = pending
	c.retryMu.Unlock()
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
