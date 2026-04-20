// Package qq implements a QQ channel for dogclaw using WebSocket mode.
// It supports private messages (C2C) and group @ messages.
package qq

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/tencent-connect/botgo"
	"github.com/tencent-connect/botgo/constant"
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
	// 文件下载目录
	downloadDir = "~/.dogclaw/download/qq"
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

// Assert Channel implements channel.Interface and channel.FileSender
var _ channel.Interface = (*Channel)(nil)
var _ channel.FileSender = (*Channel)(nil)
var _ channel.ActiveChatter = (*Channel)(nil)

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

// SystemPrompt implements channel.Interface
func (c *Channel) SystemPrompt() string {
	return `## QQ频道能力说明

### 文件接收
- 用户发送的文件会自动下载到 ~/.dogclaw/download/qq 目录
- 支持接收图片、语音、视频、文件等附件
- 下载完成后会将文件路径信息包含在消息中

### 文件发送
- 可以使用 channel_send_file 工具向用户发送文件
- 文件类型: 1=图片, 2=视频, 3=语音(仅支持silk格式), 4=文件
- 支持两种方式:
  1. 远程URL: file_url为HTTP/HTTPS链接
  2. 本地文件: file_url为本地文件路径，会通过base64编码上传
- 重要限制: 群聊不支持发送文件类型(4)，仅支持图片、视频、语音
- 示例:
  - 远程图片: channel_send_file(channel="qq", file_type=1, file_url="https://example.com/image.png")
  - 本地文件: channel_send_file(channel="qq", file_type=4, file_url="/path/to/file.pdf", file_name="document.pdf")

### 消息发送
- 支持发送文本消息和Markdown格式消息
- 私聊消息直接发送给用户
- 群消息需要@机器人才能触发回复

### 注意事项
- 群消息中的URL会被处理以避免QQ的URL黑名单机制
- 消息长度有限制，超长消息会被截断
- 本地文件上传使用base64编码，大文件可能需要较长时间`
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

// ActiveChatIDs implements channel.ActiveChatter
func (c *Channel) ActiveChatIDs() []string {
	var ids []string
	c.sessions.Range(func(key, _ any) bool {
		if id, ok := key.(string); ok {
			ids = append(ids, id)
		}
		return true
	})
	return ids
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

		// Process attachments if any
		attachmentInfo := c.processAttachments(c.ctx, data.Attachments)
		if attachmentInfo != "" {
			if content != "" {
				content = content + "\n" + attachmentInfo
			} else {
				content = attachmentInfo
			}
		}

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

		// Process attachments if any
		attachmentInfo := c.processAttachments(c.ctx, data.Attachments)
		if attachmentInfo != "" {
			if content != "" {
				content = content + "\n" + attachmentInfo
			} else {
				content = attachmentInfo
			}
		}

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

	engine := newEngine("qq")

	// TextCallback: fires for every LLM text block (intermediate turns with tools
	// and the final text-only reply).
	// Run in goroutine to avoid blocking the query engine loop if platform API is slow.
	engine.TextCallback = func(text string, isFinish bool) {
		go sendFn(chatID, text)
	}
	// ToolCallCallback: sends a brief notification when a tool is called
	// Run in goroutine to ensure UI updates don't delay tool execution.
	engine.ToolCallCallback = func(toolName, summary string) {
		go sendFn(chatID, fmt.Sprintf("🔧 %s", summary))
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
	ctx, cancel := context.WithTimeout(ctx, 24*time.Hour)
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
	// Create a dedicated context with a 30s timeout for outbound message delivery
	// to prevent platform API hangs from blocking the engine or background tasks.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

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

// SendFile implements channel.FileSender
// Supports both remote URLs and local file paths.
// Uses two-step flow: upload first to get file_info, then send message.
func (c *Channel) SendFile(ctx context.Context, chatID string, fileType int, fileURL, fileName string) error {
	log.Printf("[QQ] SendFile called: chatID=%s, fileType=%d, fileURL=%s, fileName=%s", chatID, fileType, fileURL, fileName)

	if fileURL == "" {
		return fmt.Errorf("file URL or path is required")
	}

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	chatKind := "direct"
	targetID := chatID
	if strings.HasPrefix(chatID, "group:") {
		chatKind = "group"
		targetID = strings.TrimPrefix(chatID, "group:")
	}

	log.Printf("[QQ] SendFile: chatKind=%s, targetID=%s", chatKind, targetID)

	// Group chat does not support file_type=4 (file)
	if chatKind == "group" && fileType == channel.FileTypeFile {
		return fmt.Errorf("QQ群聊暂不支持发送文件类型(仅支持图片、视频、语音)")
	}

	// Upload file first
	fileInfo, err := c.uploadFile(ctx, chatKind, targetID, fileType, fileURL, fileName)
	if err != nil {
		log.Printf("[QQ] SendFile upload failed: %v", err)
		return fmt.Errorf("failed to upload file: %w", err)
	}

	log.Printf("[QQ] SendFile upload success, fileInfo length=%d", len(fileInfo))

	// Send message with file_info
	err = c.sendFileMessage(ctx, chatKind, targetID, fileInfo)
	if err != nil {
		log.Printf("[QQ] SendFile send message failed: %v", err)
		return err
	}

	log.Printf("[QQ] SendFile completed successfully")
	return nil
}

// isHTTPURL checks if the string is an HTTP/HTTPS URL
func isHTTPURL(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// qqMediaUpload is the payload for uploading media to QQ
type qqMediaUpload struct {
	FileType   uint64 `json:"file_type"`
	URL        string `json:"url,omitempty"`
	FileData   string `json:"file_data,omitempty"`
	FileName   string `json:"file_name,omitempty"`
	SrvSendMsg bool   `json:"srv_send_msg"`
}

// qqMediaUploadResponse is the response from uploading media
type qqMediaUploadResponse struct {
	FileUUID string `json:"file_uuid"`
	FileInfo string `json:"file_info"`
	TTL      int    `json:"ttl"`
	ID       string `json:"id,omitempty"`
}

// uploadFile uploads a file and returns file_info
func (c *Channel) uploadFile(ctx context.Context, chatKind, targetID string, fileType int, fileSource, fileName string) ([]byte, error) {
	payload := &qqMediaUpload{
		FileType:   uint64(fileType),
		SrvSendMsg: false,
	}

	if isHTTPURL(fileSource) {
		payload.URL = fileSource
	} else {
		data, err := os.ReadFile(fileSource)
		if err != nil {
			return nil, fmt.Errorf("failed to read local file: %w", err)
		}
		payload.FileData = base64.StdEncoding.EncodeToString(data)
		log.Printf("[QQ] uploadFile: read local file %s, size=%d bytes", fileSource, len(data))
	}

	if fileName != "" && fileType == channel.FileTypeFile {
		payload.FileName = fileName
	}

	uploadURL := c.mediaUploadURL(chatKind, targetID)

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	log.Printf("[QQ] uploadFile: URL=%s, payload=%s", uploadURL, string(payloadJSON))

	// Get access token
	tk, err := c.ts.Token()
	if err != nil {
		return nil, fmt.Errorf("failed to get access token: %w", err)
	}

	// Build HTTP request manually
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, bytes.NewReader(payloadJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", tk.TokenType+" "+tk.AccessToken)
	req.Header.Set("X-Union-Appid", c.cfg.AppID)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[QQ] uploadFile: status=%d, response=%s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	var uploaded qqMediaUploadResponse
	if err := json.Unmarshal(body, &uploaded); err != nil {
		return nil, fmt.Errorf("failed to parse upload response: %w (body: %s)", err, string(body))
	}
	if uploaded.FileInfo == "" {
		return nil, fmt.Errorf("upload response missing file_info (body: %s)", string(body))
	}

	log.Printf("[QQ] uploadFile: got file_info, ttl=%d", uploaded.TTL)
	return []byte(uploaded.FileInfo), nil
}

type qqFileMessageRequest struct {
	MsgType int    `json:"msg_type"`
	MsgID   string `json:"msg_id,omitempty"`
	MsgSeq  uint32 `json:"msg_seq,omitempty"`
	Media   struct {
		FileInfo string `json:"file_info"`
	} `json:"media"`
}

func (c *Channel) sendFileMessage(ctx context.Context, chatKind, targetID string, fileInfo []byte) error {
	req := &qqFileMessageRequest{
		MsgType: 7,
	}
	req.Media.FileInfo = string(fileInfo)

	c.applyPassiveReplyMetadataToFile(targetID, req)

	var apiURL string
	if chatKind == "group" {
		apiURL = fmt.Sprintf("https://api.sgroup.qq.com/v2/groups/%s/messages", targetID)
	} else {
		apiURL = fmt.Sprintf("https://api.sgroup.qq.com/v2/users/%s/messages", targetID)
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	tk, err := c.ts.Token()
	if err != nil {
		return fmt.Errorf("failed to get access token: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", tk.TokenType+" "+tk.AccessToken)
	httpReq.Header.Set("X-Union-Appid", c.cfg.AppID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[QQ] sendFileMessage: status=%d, response=%s", resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("send message failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func (c *Channel) applyPassiveReplyMetadataToFile(chatID string, req *qqFileMessageRequest) {
	if v, ok := c.lastMsgID.Load(chatID); ok {
		if id, ok := v.(string); ok && id != "" {
			req.MsgID = id
			if cv, ok := c.msgSeqCounters.Load(chatID); ok {
				if ctr, ok := cv.(*atomic.Uint64); ok {
					req.MsgSeq = uint32(ctr.Add(1))
				}
			}
		}
	}
}

// applyPassiveReplyMetadata sets MsgID and MsgSeq for passive reply
func (c *Channel) applyPassiveReplyMetadata(chatID string, msg *dto.MessageToCreate) {
	if v, ok := c.lastMsgID.Load(chatID); ok {
		if msgID, ok := v.(string); ok && msgID != "" {
			msg.MsgID = msgID

			// Increment msg_seq atomically for multi-part replies
			if counterVal, ok := c.msgSeqCounters.Load(chatID); ok {
				if counter, ok := counterVal.(*atomic.Uint64); ok {
					seq := counter.Add(1)
					msg.MsgSeq = uint32(seq)
				}
			}
		}
	}
}

// mediaUploadURL returns the media upload URL for group or C2C
func (c *Channel) mediaUploadURL(chatKind, targetID string) string {
	if chatKind == "group" {
		return fmt.Sprintf("%s/v2/groups/%s/files", constant.APIDomain, targetID)
	}
	return fmt.Sprintf("%s/v2/users/%s/files", constant.APIDomain, targetID)
}

// downloadFile downloads a file from URL to the download directory
func downloadFile(ctx context.Context, url, filename string) (string, error) {
	// Expand home directory
	dir := os.ExpandEnv(downloadDir)
	if strings.HasPrefix(dir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}
		dir = filepath.Join(home, dir[2:])
	}

	// Create directory if not exists
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create download directory: %w", err)
	}

	// Create request with context
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Download file
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Create destination file
	destPath := filepath.Join(dir, filename)
	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer out.Close()

	// Write file content
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return destPath, nil
}

// processAttachments downloads all attachments and returns a summary message
func (c *Channel) processAttachments(ctx context.Context, attachments []*dto.MessageAttachment) string {
	if len(attachments) == 0 {
		return ""
	}

	var results []string
	for _, att := range attachments {
		if att == nil || att.URL == "" {
			continue
		}

		filename := att.FileName
		if filename == "" {
			filename = fmt.Sprintf("file_%d", time.Now().Unix())
		}

		destPath, err := downloadFile(ctx, att.URL, filename)
		if err != nil {
			results = append(results, fmt.Sprintf("❌ 下载文件 %s 失败: %v", filename, err))
			continue
		}

		sizeInfo := ""
		if att.Size > 0 {
			sizeInfo = fmt.Sprintf(" (%d bytes)", att.Size)
		}
		results = append(results, fmt.Sprintf("📎 已下载文件: %s%s", destPath, sizeInfo))
	}

	return strings.Join(results, "\n")
}
