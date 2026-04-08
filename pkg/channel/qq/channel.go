// Package qq implements a QQ channel for dogclaw using WebSocket mode.
// It supports private messages (C2C) and group @ messages.
package qq

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/dto"
	"github.com/tencent-connect/botgo/event"
	"github.com/tencent-connect/botgo/openapi"
	"github.com/tencent-connect/botgo/token"
	"golang.org/x/oauth2"

	"dogclaw/pkg/channel"
	"dogclaw/pkg/query"
)

const (
	dedupTTL      = 5 * time.Minute
	dedupInterval = 60 * time.Second
	dedupMaxSize  = 10000
	// QQ Markdown消息限制（安全阈值）
	maxMessageLength = 4500
)

// Config holds QQ bot configuration
type Config struct {
	AppID        string
	AppSecret    string
	AllowFrom    []string // Allowed sender IDs (empty = allow all)
	SendMarkdown bool
}

// Channel represents the QQ bot channel
type Channel struct {
	cfg    *Config
	api    openapi.OpenAPI
	ts     oauth2.TokenSource
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	// Per-chat query engine sessions
	sessions  sync.Map                            // chatID → *ChatSession
	sendMsgFn func(chatID string, content string) // set by getOrCreateSession for tool notifications

	// Passive reply metadata
	lastMsgID      sync.Map // chatID → string
	msgSeqCounters sync.Map // chatID → *atomic.Uint64

	// Dedup
	dedup   map[string]time.Time
	muDedup sync.Mutex
	stopMu  sync.Once
}

// ChatSession wraps a QueryEngine for a single QQ conversation
type ChatSession struct {
	Engine  *query.QueryEngine
	Creator string // first user who created this session
}

// Assert Channel implements channel.Interface
var _ channel.Interface = (*Channel)(nil)

// NewChannel creates a new QQ channel
func NewChannel(cfg Config) *Channel {
	return &Channel{
		cfg:   &cfg,
		done:  make(chan struct{}),
		dedup: make(map[string]time.Time),
	}
}

// Info implements channel.Interface
func (c *Channel) Info() channel.Info {
	return channel.Info{Name: "qq"}
}

// Start initializes and runs the QQ bot in WebSocket mode
func (c *Channel) Start(ctx context.Context, newEngine channel.EngineFactory) error {
	if c.cfg.AppID == "" || c.cfg.AppSecret == "" {
		return fmt.Errorf("QQ AppID and AppSecret are required")
	}

	c.ctx, c.cancel = context.WithCancel(ctx)

	// Create token source for auto-refresh
	creds := &token.QQBotCredentials{
		AppID:     c.cfg.AppID,
		AppSecret: c.cfg.AppSecret,
	}
	c.ts = token.NewQQBotTokenSource(creds)

	// Start auto-refresh
	if err := token.StartRefreshAccessToken(c.ctx, c.ts); err != nil {
		return fmt.Errorf("failed to start token refresh: %w", err)
	}

	// Initialize API client
	c.api = botgo.NewOpenAPI(c.cfg.AppID, c.ts).WithTimeout(10 * time.Second)

	// Get WebSocket endpoint
	wsInfo, err := c.api.WS(c.ctx, nil, "")
	if err != nil {
		return fmt.Errorf("failed to get QQ WebSocket info: %w", err)
	}

	// Register event handlers
	intent := event.RegisterHandlers(
		c.handleC2CMessage(newEngine),
		c.handleGroupATMessage(newEngine),
	)

	// Start WebSocket session
	sm := botgo.NewSessionManager()
	go func() {
		fmt.Println("🐧 Connecting to QQ WebSocket...")
		if err := sm.Start(wsInfo, c.ts, &intent); err != nil {
			fmt.Printf("❌ QQ WebSocket error: %v\n", err)
		}
	}()

	// Start dedup cleanup goroutine
	go c.dedupJanitor()

	fmt.Println("🐧 QQ bot started (direct messages & group @)")
	return nil
}

// Stop shuts down the QQ channel
func (c *Channel) Stop() {
	c.stopMu.Do(func() {
		if c.cancel != nil {
			c.cancel()
		}
		close(c.done)
	})
}

// handleC2CMessage handles QQ private (C2C) messages
func (c *Channel) handleC2CMessage(newEngine channel.EngineFactory) event.C2CMessageEventHandler {
	return func(_ *dto.WSPayload, data *dto.WSC2CMessageData) error {
		if data == nil || data.Author == nil || data.Author.ID == "" {
			return nil
		}

		// Dedup check
		if c.isDuplicate(data.ID) {
			return nil
		}

		senderID := data.Author.ID

		// Check allowed senders
		if !c.isAllowedSender(senderID) {
			return nil
		}

		chatID := senderID // C2C: use user ID as chat ID
		content := strings.TrimSpace(data.Content)
		if content == "" {
			return nil
		}

		c.lastMsgID.Store(chatID, data.ID)
		c.msgSeqCounters.Store(chatID, new(atomic.Uint64))

		sendFn := func(cid string, msg string) {
			c.sendMessage(c.ctx, cid, "direct", msg)
		}
		session := c.getOrCreateSession(chatID, senderID, newEngine, sendFn)

		go func() {
			// TextCallback delivers LLM text in real-time; getReply only returns a
			// non-empty string for error messages, so check before sending.
			if reply := c.getReply(c.ctx, session, content); reply != "" {
				c.sendMessage(c.ctx, chatID, "direct", reply)
			}
		}()

		return nil
	}
}

// handleGroupATMessage handles QQ group @ messages
func (c *Channel) handleGroupATMessage(newEngine channel.EngineFactory) event.GroupATMessageEventHandler {
	return func(_ *dto.WSPayload, data *dto.WSGroupATMessageData) error {
		if data == nil || data.Author == nil || data.Author.ID == "" {
			return nil
		}

		// Dedup check
		if c.isDuplicate(data.ID) {
			return nil
		}

		senderID := data.Author.ID

		// Check allowed senders
		if !c.isAllowedSender(senderID) {
			return nil
		}

		groupID := data.GroupID
		chatID := "group:" + groupID

		// Remove @bot mention prefix from content
		content := trimBotMention(data.Content)
		content = strings.TrimSpace(content)
		if content == "" {
			return nil
		}

		c.lastMsgID.Store(chatID, data.ID)
		c.msgSeqCounters.Store(chatID, new(atomic.Uint64))

		sendFn := func(cid string, msg string) {
			c.sendMessage(c.ctx, cid, "group", msg)
		}
		session := c.getOrCreateSession(chatID, senderID, newEngine, sendFn)

		// Build prompt with display name
		prompt := content
		if data.Member != nil && data.Member.Nick != "" {
			prompt = fmt.Sprintf("[%s]: %s", data.Member.Nick, content)
		} else if data.Author.Username != "" {
			prompt = fmt.Sprintf("[%s]: %s", data.Author.Username, content)
		}

		go func() {
			// TextCallback delivers LLM text in real-time; getReply only returns a
			// non-empty string for error messages, so check before sending.
			if reply := c.getReply(c.ctx, session, prompt); reply != "" {
				c.sendMessage(c.ctx, chatID, "group", reply)
			}
		}()

		return nil
	}
}

// getOrCreateSession returns an existing session or creates a new one
func (c *Channel) getOrCreateSession(chatID, creator string, newEngine channel.EngineFactory, sendFn func(chatID string, content string)) *ChatSession {
	if v, ok := c.sessions.Load(chatID); ok {
		return v.(*ChatSession)
	}

	engine := newEngine()

	// TextCallback: fires for every LLM text block (intermediate turns with tools
	// and the final text-only reply). Delivers LLM commentary to QQ users in real-time.
	engine.TextCallback = func(text string) {
		sendFn(chatID, text)
	}
	// ToolCallCallback: sends a brief notification when a tool is called
	engine.ToolCallCallback = func(toolName, summary string) {
		sendFn(chatID, fmt.Sprintf("🔧 %s", summary))
	}
	
	// Automatically resume latest session for this context
	engine.AutoResumeLatestSession(context.Background())

	sess := &ChatSession{
		Engine:  engine,
		Creator: creator,
	}
	c.sessions.Store(chatID, sess)
	return sess
}

// getReply runs SubmitMessage and returns a non-empty string only on API errors.
// Normal LLM text (including slash command output) is pushed to QQ via TextCallback
// registered in getOrCreateSession, so this function returns "" for successful runs
// to avoid sending a duplicate message.
func (c *Channel) getReply(ctx context.Context, session *ChatSession, prompt string) string {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	err := session.Engine.SubmitMessage(ctx, prompt)
	if err != nil {
		return fmt.Sprintf("⚠️ 处理错误: %v", err)
	}

	// TextCallback already pushed every text block (LLM reply + slash output) to QQ;
	// return "" here to avoid sending a duplicate message.
	return ""
}

// sendMessage sends a reply to a QQ user or group
func (c *Channel) sendMessage(ctx context.Context, chatID, kind string, content string) {
	var msg *dto.MessageToCreate

	if c.cfg.SendMarkdown {
		msg = &dto.MessageToCreate{
			MsgType: dto.MarkdownMsg,
			Markdown: &dto.Markdown{
				Content: content,
			},
		}
	} else {
		msg = &dto.MessageToCreate{
			Content: content,
			MsgType: dto.TextMsg,
		}
	}

	// Set passive reply metadata (msg_id for reply context)
	if v, ok := c.lastMsgID.Load(chatID); ok {
		if id, ok := v.(string); ok && id != "" {
			msg.MsgID = id

			// Increment msg_seq for multi-part replies
			if cv, ok := c.msgSeqCounters.Load(chatID); ok {
				if ctr, ok := cv.(*atomic.Uint64); ok {
					msg.MsgSeq = uint32(ctr.Add(1))
				}
			}
		}
	}

	// Sanitize URLs in group messages to prevent QQ URL blacklist
	if kind == "group" {
		if msg.Markdown != nil {
			msg.Markdown.Content = sanitizeURLs(msg.Markdown.Content)
		} else {
			msg.Content = sanitizeURLs(msg.Content)
		}
	}

	var err error
	if kind == "group" {
		groupID := strings.TrimPrefix(chatID, "group:")
		_, err = c.api.PostGroupMessage(ctx, groupID, msg)
	} else {
		_, err = c.api.PostC2CMessage(ctx, chatID, msg)
	}

	if err != nil {
		fmt.Printf("QQ send error [%s]: %v\n", kind, err)
	}
}

// isAllowedSender checks if the sender is in the allowed list
func (c *Channel) isAllowedSender(senderID string) bool {
	if len(c.cfg.AllowFrom) == 0 {
		return true
	}
	for _, id := range c.cfg.AllowFrom {
		if id == senderID {
			return true
		}
	}
	return false
}

// isDuplicate checks for duplicate message IDs within TTL
func (c *Channel) isDuplicate(messageID string) bool {
	c.muDedup.Lock()
	defer c.muDedup.Unlock()

	if ts, exists := c.dedup[messageID]; exists && time.Since(ts) < dedupTTL {
		return true
	}

	// Enforce hard cap
	if len(c.dedup) >= dedupMaxSize {
		var oldest string
		var oldestTime time.Time
		for id, t := range c.dedup {
			if oldest == "" || t.Before(oldestTime) {
				oldest = id
				oldestTime = t
			}
		}
		if oldest != "" {
			delete(c.dedup, oldest)
		}
	}

	c.dedup[messageID] = time.Now()
	return false
}

// dedupJanitor periodically evicts expired dedup entries
func (c *Channel) dedupJanitor() {
	ticker := time.NewTicker(dedupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
			c.muDedup.Lock()
			now := time.Now()
			for id, ts := range c.dedup {
				if now.Sub(ts) >= dedupTTL {
					delete(c.dedup, id)
				}
			}
			c.muDedup.Unlock()
		}
	}
}

// urlPattern matches URLs with explicit http(s):// scheme
var urlPattern = regexp.MustCompile(
	`(?i)https?://(?:[a-zA-Z0-9](?:[a-zA-Z0-9-]{0,61}[a-zA-Z0-9])?\.)+[a-zA-Z]{2,}(?:[/?#]\S*)?`,
)

// sanitizeURLs replaces dots in URL domains to prevent QQ URL blacklist
func sanitizeURLs(text string) string {
	return urlPattern.ReplaceAllStringFunc(text, func(match string) string {
		idx := strings.Index(match, "://")
		if idx < 0 {
			return match
		}
		scheme := match[:idx+3]
		rest := match[idx+3:]

		domainEnd := len(rest)
		for i, ch := range rest {
			if ch == '/' || ch == '?' || ch == '#' {
				domainEnd = i
				break
			}
		}

		domain := rest[:domainEnd]
		path := rest[domainEnd:]
		domain = strings.ReplaceAll(domain, ".", "。")

		return scheme + domain + path
	})
}

// trimBotMention removes @bot mention prefixes from message content
func trimBotMention(content string) string {
	// Remove QQ bot mention pattern: <@!BOT_ID>
	re := regexp.MustCompile(`<@!\d+>`)
	content = re.ReplaceAllString(content, "")
	return strings.TrimSpace(content)
}

// truncateMessage truncates content to fit QQ message limits
func (c *Channel) truncateMessage(content string) string {
	if len(content) <= maxMessageLength {
		return content
	}
	// 在Markdown中,我们希望在代码块边界截断
	truncated := content[:maxMessageLength-3] + "..."
	if c.cfg.SendMarkdown {
		// 确保Markdown代码块闭合
		truncated = strings.TrimSuffix(truncated, "```")
	}
	return truncated
}
// Send implements channel.Sender
func (c *Channel) Send(ctx context.Context, chatID, message string) error {
	if message == "" {
		return nil
	}

	if chatID == "" {
		// Broadcast to all active sessions
		c.sessions.Range(func(key, value any) bool {
			cid := key.(string)
			kind := "direct"
			if strings.HasPrefix(cid, "group:") {
				kind = "group"
			}
			c.sendMessage(ctx, cid, kind, message)
			return true
		})
		return nil
	}

	kind := "direct"
	if strings.HasPrefix(chatID, "group:") {
		kind = "group"
	}
	c.sendMessage(ctx, chatID, kind, message)
	return nil
}
