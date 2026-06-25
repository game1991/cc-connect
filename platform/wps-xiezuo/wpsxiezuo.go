package wpsxiezuo

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/chenhg5/cc-connect/core"
	"github.com/gorilla/websocket"
)

var (
	wsEndpoint          = "wss://openapi.wps.cn/v7/event/ws"
	defaultBaseURL      = "https://openapi.wps.cn"
	maxBackoff          = 60 * time.Second
	maxErrBodyBytes     = 256
	httpTimeout         = 30 * time.Second
	wpsCardMaxChars     = 15000
	wpsCardTruncateKeep = 14000
)

// Platform implements core.Platform for WPS Xiezuo (WPS 协作).
type Platform struct {
	appID          string
	appSecret      string
	baseURL        string
	cleanReply     bool
	allowFrom      string
	handler        core.MessageHandler
	ctx            context.Context // set by Start, used for context-aware waits
	cancel         context.CancelFunc
	conn           *websocket.Conn
	mu             sync.Mutex // protects conn access
	writeCh        chan any   // serializes all WebSocket writes (ACK, reactions, etc.)
	dedup          core.MessageDedup
	token          string
	tokenExpire    time.Time
	tokenMu        sync.Mutex
	stopOnce       sync.Once
	stopped        bool
	httpClient     *http.Client
	previewMu      sync.Mutex
	previewHandles map[string]*wpsPreviewHandle // key: chatID
}

// replyContext holds the context needed to reply to a specific message.
type replyContext struct {
	ChatID    string `json:"chat_id"`
	ChatType  string `json:"chat_type"`
	CompanyID string `json:"company_id"`
	MessageID string `json:"message_id"`
	SenderID  string `json:"sender_id"`
}

// wpsPreviewHandle holds state for an in-place card preview.
type wpsPreviewHandle struct {
	mu          sync.Mutex
	messageID   string
	status      core.CardStatus
	chatID      string
	lastContent string
}

func statusEmoji(s core.CardStatus) string {
	switch s {
	case core.CardStatusThinking:
		return "💭"
	case core.CardStatusWorking:
		return "🔧"
	case core.CardStatusDone:
		return "✅"
	case core.CardStatusError:
		return "❌"
	default:
		return "⏳"
	}
}

func statusLabel(s core.CardStatus) string {
	switch s {
	case core.CardStatusThinking:
		return "思考中"
	case core.CardStatusWorking:
		return "工作中"
	case core.CardStatusDone:
		return "已完成"
	case core.CardStatusError:
		return "出错"
	default:
		return "未知"
	}
}

// --- Factory ---

func init() {
	core.RegisterPlatform("wps-xiezuo", New)
}

// New creates a new WPS Xiezuo platform from config options.
func New(opts map[string]any) (core.Platform, error) {
	appID, _ := opts["app_id"].(string)
	appSecret, _ := opts["app_secret"].(string)
	if appID == "" || appSecret == "" {
		return nil, fmt.Errorf("wps-xiezuo: app_id and app_secret are required")
	}

	baseURL := defaultBaseURL
	if v, ok := opts["base_url"].(string); ok && v != "" {
		baseURL = strings.TrimRight(v, "/")
	}

	cleanReply, _ := opts["clean_reply"].(bool)
	allowFrom, _ := opts["allow_from"].(string)

	core.CheckAllowFrom("wps-xiezuo", allowFrom)

	return &Platform{
		appID:          appID,
		appSecret:      appSecret,
		baseURL:        baseURL,
		cleanReply:     cleanReply,
		allowFrom:      allowFrom,
		httpClient:     &http.Client{Timeout: httpTimeout},
		previewHandles: make(map[string]*wpsPreviewHandle),
	}, nil
}

func (p *Platform) Name() string { return "wps-xiezuo" }

// Start begins the WebSocket connection loop.
func (p *Platform) Start(handler core.MessageHandler) error {
	p.handler = handler
	ctx, cancel := context.WithCancel(context.Background())
	p.ctx = ctx
	p.cancel = cancel
	go p.connectLoop(ctx)
	return nil
}

// Stop cancels the context and closes the WebSocket connection.
func (p *Platform) Stop() error {
	p.stopOnce.Do(func() {
		p.stopped = true
		if p.cancel != nil {
			p.cancel()
		}
		p.mu.Lock()
		if p.conn != nil {
			_ = p.conn.Close()
			p.conn = nil
		}
		p.mu.Unlock()
	})
	return nil
}

// --- WebSocket connection loop with exponential backoff ---

func (p *Platform) connectLoop(ctx context.Context) {
	backoff := time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		start := time.Now()
		err := p.runConnection(ctx)
		if p.stopped || ctx.Err() != nil {
			return
		}

		// Reset backoff if connection was alive long enough
		if time.Since(start) > 2*time.Minute {
			backoff = time.Second
		}

		slog.Warn("wps-xiezuo: connection lost, reconnecting", "error", err, "backoff", backoff)
		select {
		case <-time.After(backoff):
		case <-ctx.Done():
			return
		}

		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

func (p *Platform) runConnection(ctx context.Context) error {
	slog.Info("wps-xiezuo: connecting", "endpoint", wsEndpoint)

	header, err := p.signWSHeader()
	if err != nil {
		return fmt.Errorf("sign header: %w", err)
	}

	dialer := websocket.DefaultDialer
	conn, _, err := dialer.DialContext(ctx, wsEndpoint, header)
	if err != nil {
		return fmt.Errorf("dial: %w", err)
	}

	p.mu.Lock()
	p.conn = conn
	writeCh := make(chan any, 64)
	p.writeCh = writeCh
	p.mu.Unlock()

	defer func() {
		p.mu.Lock()
		p.conn = nil
		p.writeCh = nil
		p.mu.Unlock()
		close(writeCh)
		_ = conn.Close()
	}()

	slog.Info("wps-xiezuo: connected")

	// Set up control frame handlers.
	// CRITICAL: Both handlers must reset the read deadline, otherwise
	// the connection times out even though heartbeats are flowing.
	const pingTimeout = 90 * time.Second
	conn.SetPingHandler(func(appData string) error {
		slog.Debug("wps-xiezuo: server ping received")
		_ = conn.SetReadDeadline(time.Now().Add(pingTimeout))
		p.mu.Lock()
		ch := p.writeCh
		p.mu.Unlock()
		if ch != nil {
			select {
			case ch <- pongFrame{Type: "pong", Data: appData}:
			default:
			}
		}
		return nil
	})
	conn.SetPongHandler(func(appData string) error {
		slog.Debug("wps-xiezuo: server pong received")
		_ = conn.SetReadDeadline(time.Now().Add(pingTimeout))
		return nil
	})

	// Start writer goroutine to serialize all WebSocket writes
	writeCtx, writeCancel := context.WithCancel(ctx)
	defer writeCancel()
	go p.writeLoop(writeCtx, conn, writeCh)

	// Send client pings every 25s to keep the connection alive
	go func() {
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-writeCtx.Done():
				return
			case <-ticker.C:
				p.mu.Lock()
				ch := p.writeCh
				p.mu.Unlock()
				if ch != nil {
					select {
					case ch <- pingControl{}:
					default:
					}
				}
			}
		}
	}()

	// Read deadline: 90s for PING timeout (matching Node.js SDK)
	_ = conn.SetReadDeadline(time.Now().Add(pingTimeout))

	// Read loop
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msgType, raw, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read: %w", err)
		}
		slog.Info("wps-xiezuo: message received", "type", msgType, "len", len(raw), "data", string(raw))

		// Reset deadline on successful read
		_ = conn.SetReadDeadline(time.Now().Add(pingTimeout))

		p.handleRawMessage(ctx, raw)
	}
}

// writeLoop serializes all WebSocket writes (ACK frames, pongs, pings, etc.) on a single goroutine.
// gorilla/websocket requires all writes to be serialized.
func (p *Platform) writeLoop(ctx context.Context, conn *websocket.Conn, writeCh chan any) {
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-writeCh:
			if !ok {
				return
			}
			switch v := msg.(type) {
			case pongFrame:
				if err := conn.WriteControl(websocket.PongMessage, []byte(v.Data), time.Now().Add(5*time.Second)); err != nil {
					slog.Debug("wps-xiezuo: pong write error", "error", err)
					return
				}
				slog.Debug("wps-xiezuo: pong sent")
			case pingControl:
				if err := conn.WriteControl(websocket.PingMessage, []byte{}, time.Now().Add(5*time.Second)); err != nil {
					slog.Debug("wps-xiezuo: ping write error", "error", err)
					return
				}
				slog.Debug("wps-xiezuo: client ping sent")
			default:
				if err := conn.WriteJSON(msg); err != nil {
					slog.Debug("wps-xiezuo: write error", "error", err)
					return
				}
			}
		}
	}
}

// --- KSO-1 HMAC-SHA256 signing ---

// kso1Sign computes the KSO-1 Authorization header for any HTTP request.
// stringToSign = "KSO-1" + method + uri + contentType + date + sha256Hex(body)
// For empty body, sha256Hex is omitted (empty string appended).
func (p *Platform) kso1Sign(method, uri, contentType string, body []byte) (date, authHeader string) {
	date = time.Now().UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	bodyHash := ""
	if len(body) > 0 {
		h := sha256.Sum256(body)
		bodyHash = hex.EncodeToString(h[:])
	}
	stringToSign := "KSO-1" + method + uri + contentType + date + bodyHash

	mac := hmac.New(sha256.New, []byte(p.appSecret))
	mac.Write([]byte(stringToSign))
	signature := hex.EncodeToString(mac.Sum(nil))

	authHeader = fmt.Sprintf("KSO-1 %s:%s", p.appID, signature)
	return
}

func (p *Platform) signWSHeader() (http.Header, error) {
	u, err := url.Parse(wsEndpoint)
	if err != nil {
		return nil, fmt.Errorf("parse ws url: %w", err)
	}
	uri := u.RequestURI()
	date, authHeader := p.kso1Sign("GET", uri, "", nil)

	header := http.Header{
		"X-Kso-Date":          {date},
		"X-Kso-Authorization": {authHeader},
		"X-Ack-Mode":          {"required"},
	}
	return header, nil
}

// --- Raw message dispatch ---

func (p *Platform) handleRawMessage(_ context.Context, raw []byte) {
	// Try to detect frame type
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(raw, &probe); err != nil {
		slog.Warn("wps-xiezuo: invalid json", "error", err)
		return
	}

	// GoAway frame: has "type": "goaway"
	if t, ok := probe["type"]; ok {
		var typeStr string
		if err := json.Unmarshal(t, &typeStr); err == nil {
			if typeStr == "goaway" {
				var goAway wpsGoAwayFrame
				if err := json.Unmarshal(raw, &goAway); err == nil {
					p.handleGoAway(goAway)
				}
				return
			}
			// Ignore other control frames (ack, etc.)
			slog.Debug("wps-xiezuo: control frame", "type", typeStr)
			return
		}
	}

	// Event frame: has "topic" and "operation"
	if _, hasTopic := probe["topic"]; hasTopic {
		var event wpsEventFrame
		if err := json.Unmarshal(raw, &event); err != nil {
			slog.Warn("wps-xiezuo: parse event frame failed", "error", err)
			return
		}
		p.handleEvent(event)
		return
	}

	slog.Debug("wps-xiezuo: unknown frame", "data", string(raw))
}

// --- GoAway handling ---

func (p *Platform) handleGoAway(goAway wpsGoAwayFrame) {
	slog.Warn("wps-xiezuo: goaway received", "reason", goAway.Reason, "message", goAway.Message)

	if goAway.Reason == "connection_replaced" {
		slog.Warn("wps-xiezuo: connection replaced, stopping reconnect")
		p.stopped = true
		_ = p.Stop()
		return
	}

	// For other reasons (server_shutdown etc.), wait before reconnect
	if goAway.ReconnectMs > 0 {
		if p.ctx != nil {
			select {
			case <-time.After(time.Duration(goAway.ReconnectMs) * time.Millisecond):
			case <-p.ctx.Done():
			}
		} else {
			// Fallback: before Start is called, just sleep
			time.Sleep(time.Duration(goAway.ReconnectMs) * time.Millisecond)
		}
	}
}

// --- Event handling ---

func (p *Platform) handleEvent(event wpsEventFrame) {
	// Verify signature
	if !p.verifyEventSignature(event) {
		slog.Warn("wps-xiezuo: signature verification failed", "topic", event.Topic, "nonce", event.Nonce)
		return
	}

	// Decrypt data
	plain, err := p.decryptEventData(event.Nonce, event.EncryptedData)
	if err != nil {
		slog.Warn("wps-xiezuo: decrypt failed", "error", err, "topic", event.Topic)
		return
	}

	// Dispatch by topic+operation. Avoid logging decrypted user content.
	slog.Info("wps-xiezuo: decrypted event", "topic", event.Topic, "operation", event.Operation, "payload_bytes", len(plain))
	switch {
	case event.Topic == "kso.app_chat.message" && event.Operation == "create":
		p.sendAck(event.Nonce, nil)
		p.handleChatMessage(plain)
	case event.Topic == "kso.app_chat.message.recall":
		p.sendAck(event.Nonce, nil)
		p.handleChatMessageRecall(plain)
	default:
		p.sendAck(event.Nonce, nil)
		slog.Debug("wps-xiezuo: unhandled event", "topic", event.Topic, "operation", event.Operation)
	}
}

// --- Signature verification ---

func (p *Platform) verifyEventSignature(event wpsEventFrame) bool {
	content := fmt.Sprintf("%s:%s:%s:%d:%s", p.appID, event.Topic, event.Nonce, event.Time, event.EncryptedData)
	mac := hmac.New(sha256.New, []byte(p.appSecret))
	mac.Write([]byte(content))
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	expectedSig = strings.TrimRight(expectedSig, "=")

	return hmac.Equal([]byte(event.Signature), []byte(expectedSig))
}

// --- AES-256-CBC decryption ---

func (p *Platform) decryptEventData(nonce, encryptedData string) ([]byte, error) {
	// key = MD5(appSecret).hexdigest() → 32 bytes
	hash := md5.Sum([]byte(p.appSecret))
	key := []byte(hex.EncodeToString(hash[:])) // 32 bytes

	// iv = nonce[:16]
	iv := []byte(nonce)
	if len(iv) > 16 {
		iv = iv[:16]
	}
	if len(iv) < 16 {
		// Pad with zeros if nonce is shorter than 16 bytes
		iv = append(iv, make([]byte, 16-len(iv))...)
	}

	// Base64 decode ciphertext
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedData)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	// AES-CBC decrypt
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("aes cipher: %w", err)
	}

	if len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("ciphertext not multiple of block size")
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	plaintext := make([]byte, len(ciphertext))
	mode.CryptBlocks(plaintext, ciphertext)

	// PKCS7 unpadding
	plaintext, err = pkcs7Unpad(plaintext)
	if err != nil {
		return nil, fmt.Errorf("pkcs7 unpad: %w", err)
	}

	return plaintext, nil
}

func pkcs7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padLen := int(data[len(data)-1])
	if padLen > len(data) || padLen > aes.BlockSize {
		return nil, fmt.Errorf("invalid padding length %d", padLen)
	}
	for i := len(data) - padLen; i < len(data); i++ {
		if data[i] != byte(padLen) {
			return nil, fmt.Errorf("invalid padding byte at %d", i)
		}
	}
	return data[:len(data)-padLen], nil
}

// --- ACK ---

func (p *Platform) sendAck(nonce string, err error) {
	if nonce == "" {
		return
	}
	ack := map[string]any{
		"type":  "ack",
		"nonce": nonce,
		"code":  200,
	}
	if err != nil {
		ack["code"] = 500
		ack["msg"] = err.Error()
		if len(err.Error()) > 256 {
			ack["msg"] = err.Error()[:256]
		}
	}
	p.mu.Lock()
	ch := p.writeCh
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- ack:
			slog.Info("wps-xiezuo: ack queued", "nonce", nonce)
		default:
			slog.Warn("wps-xiezuo: write channel full, dropping ack", "nonce", nonce)
		}
	}
}

// --- Chat message handling ---

func (p *Platform) handleChatMessage(plain []byte) {
	var msgData wpsMessageData
	if err := json.Unmarshal(plain, &msgData); err != nil {
		slog.Warn("wps-xiezuo: parse message data failed", "error", err)
		return
	}

	if p.dedup.IsDuplicate(msgData.Message.ID) {
		slog.Debug("wps-xiezuo: skipping duplicate message", "msg_id", msgData.Message.ID)
		return
	}

	if !core.AllowList(p.allowFrom, msgData.Sender.ID) {
		slog.Debug("wps-xiezuo: message from unauthorized user", "user", msgData.Sender.ID)
		return
	}

	// Extract text content
	text := extractText(msgData.Message.Content)
	if text == "" {
		slog.Debug("wps-xiezuo: no text content in message", "msg_id", msgData.Message.ID)
		return
	}

	// Build session key. P2P sessions include both actual chat ID and sender ID:
	// chat ID is needed for proactive sends, sender ID keeps the session user-scoped.
	sessionKey := fmt.Sprintf("wps-xiezuo:%s:%s", msgData.CompanyID, msgData.Chat.ID)
	if isP2P(msgData.Chat.Type) {
		sessionKey = fmt.Sprintf("wps-xiezuo:%s:%s:%s", msgData.CompanyID, msgData.Chat.ID, msgData.Sender.ID)
	}

	rctx := replyContext{
		ChatID:    msgData.Chat.ID, // Always use actual chat ID for WPS API
		ChatType:  msgData.Chat.Type,
		CompanyID: msgData.CompanyID,
		MessageID: msgData.Message.ID,
		SenderID:  msgData.Sender.ID,
	}

	go p.handler(p, &core.Message{
		SessionKey: sessionKey,
		Platform:   "wps-xiezuo",
		MessageID:  msgData.Message.ID,
		UserID:     msgData.Sender.ID,
		UserName:   msgData.Sender.ID, // WPS doesn't include name in event data
		Content:    text,
		ChannelKey: msgData.Chat.ID, // needed for per-group session isolation (#1217)
		ReplyCtx:   rctx,
	})
}

func (p *Platform) handleChatMessageRecall(plain []byte) {
	// Recall event has a flat structure: {"chat_id":"...","id":"...","operator":{...}}
	var recallData struct {
		ChatID    string `json:"chat_id"`
		ID        string `json:"id"`
		CompanyID string `json:"company_id"`
		Operator  struct {
			ID   string `json:"id"`
			Type string `json:"type"`
		} `json:"operator"`
	}
	if err := json.Unmarshal(plain, &recallData); err != nil {
		slog.Warn("wps-xiezuo: parse recall data failed", "error", err)
		return
	}

	sessionKey := fmt.Sprintf("wps-xiezuo:%s:%s", recallData.CompanyID, recallData.ChatID)

	rctx := replyContext{
		ChatID:    recallData.ChatID,
		ChatType:  "p2p",
		CompanyID: recallData.CompanyID,
		MessageID: recallData.ID,
		SenderID:  recallData.Operator.ID,
	}

	go p.handler(p, &core.Message{
		SessionKey: sessionKey,
		Platform:   "wps-xiezuo",
		MessageID:  recallData.ID,
		Recalled:   true,
		UserID:     recallData.Operator.ID,
		ReplyCtx:   rctx,
	})
}

// --- Text extraction ---

func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}

	// WPS v7 format: {"text":{"content":"xxx"}}
	var wpsContent struct {
		Text struct {
			Content string `json:"content"`
		} `json:"text"`
	}
	if err := json.Unmarshal(raw, &wpsContent); err == nil && wpsContent.Text.Content != "" {
		return strings.TrimSpace(wpsContent.Text.Content)
	}

	// Try {"type":"text","content":"xxx"}
	var simple struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &simple); err == nil {
		if simple.Type == "text" || simple.Type == "" {
			return extractStringContent(simple.Content)
		}
		if simple.Type == "rich_text" {
			return extractRichText(simple.Content)
		}
	}

	// Fallback: try as plain string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}

	return ""
}

func extractStringContent(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	// Try as string
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return strings.TrimSpace(s)
	}
	// Try as {"content":"xxx"}
	var obj struct {
		Content string `json:"content"`
	}
	if err := json.Unmarshal(raw, &obj); err == nil {
		return strings.TrimSpace(obj.Content)
	}
	return strings.TrimSpace(string(raw))
}

func extractRichText(raw json.RawMessage) string {
	var blocks []struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err != nil {
		return ""
	}
	parts := make([]string, 0, len(blocks))
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, " ")
}

func isP2P(chatType string) bool {
	return chatType == "p2p" || chatType == "single" || chatType == "direct"
}

// --- Reply/Send ---

// Reply sends a message back to the WPS chat via REST API.
func (p *Platform) Reply(ctx context.Context, rctx any, content string) error {
	return p.sendWPSMessage(ctx, rctx, content)
}

// Send sends a proactive message to the WPS chat via REST API.
func (p *Platform) Send(ctx context.Context, rctx any, content string) error {
	return p.sendWPSMessage(ctx, rctx, content)
}

func (p *Platform) sendWPSMessage(ctx context.Context, rctx any, content string) error {
	rc, ok := rctx.(replyContext)
	if !ok {
		return fmt.Errorf("wps-xiezuo: invalid reply context type %T", rctx)
	}
	if content == "" {
		return nil
	}

	if p.cleanReply {
		content = cleanReplyContent(content)
	}

	// WPS Open Platform v7 messages API: outer message `type` MUST be one of
	// the API-defined message-type enums (text / rich_text / image / file /
	// audio / video / card) — passing "markdown" makes the API reject the
	// request with `400000002 invalid open_v7_message_type: "markdown"`.
	//
	// Markdown rendering is opted into via the INNER `Content.Text.Type =
	// "markdown"` field (the only other inner enum is "plain"). When the
	// inner field is "markdown" WPS renders the content with a CommonMark
	// subset, and CommonMark collapses a single "\n" between non-empty
	// lines into a space. To force a real line break we must emit either
	// "  \n" (two trailing spaces) or "\n\n" — see the official docs at
	// https://open.wps.cn/documents/app-integration-dev/guide/robot/webhook
	//
	// We use the trailing-spaces form so we preserve paragraph structure
	// (no spurious blank lines) and stay safe inside fenced code blocks
	// (the two extra trailing whitespace characters inside ``` blocks are
	// preserved verbatim but visually invisible).
	content = applyWPSLineBreaks(content)

	token, err := p.getToken(ctx)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: get token: %w", err)
	}

	reqBody := wpsSendMessageRequest{
		Type: "text",
		Receiver: wpsReceiverInfo{
			Type:       "chat",
			ReceiverID: rc.ChatID,
		},
		Content: wpsMessageContent{
			Text: wpsTextContent{
				Content: content,
				Type:    "markdown",
			},
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v7/messages/create", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wps-xiezuo: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: send request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, rerr := io.ReadAll(io.LimitReader(resp.Body, int64(maxErrBodyBytes)+1))
		if rerr != nil {
			return fmt.Errorf("wps-xiezuo: send failed: status=%d (read body: %w)", resp.StatusCode, rerr)
		}
		return fmt.Errorf("wps-xiezuo: send failed: status=%d body=%s", resp.StatusCode, truncateAndRedact(respBody, token))
	}

	slog.Debug("wps-xiezuo: message sent", "chat_id", rc.ChatID, "len", len(content))
	return nil
}

// --- Token management ---

func (p *Platform) getToken(ctx context.Context) (string, error) {
	p.tokenMu.Lock()
	defer p.tokenMu.Unlock()

	if p.token != "" && time.Now().Before(p.tokenExpire.Add(-60*time.Second)) {
		return p.token, nil
	}

	formData := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {p.appID},
		"client_secret": {p.appSecret},
	}

	// Try primary endpoint first, then fallback
	endpoints := []string{p.baseURL + "/oauth2/token", p.baseURL + "/openapi/oauth2/token"}
	var lastErr error

	for _, endpoint := range endpoints {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(formData.Encode()))
		if err != nil {
			lastErr = err
			continue
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		resp, err := p.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
		_ = resp.Body.Close()
		if err != nil {
			lastErr = err
			continue
		}

		if resp.StatusCode != http.StatusOK {
			lastErr = fmt.Errorf("status=%d body=%s", resp.StatusCode, truncateAndRedact(respBody, ""))
			continue
		}

		var tokenResp wpsTokenResponse
		if err := json.Unmarshal(respBody, &tokenResp); err != nil {
			lastErr = err
			continue
		}

		if tokenResp.AccessToken == "" {
			lastErr = fmt.Errorf("empty access_token")
			continue
		}

		p.token = tokenResp.AccessToken
		if tokenResp.ExpiresIn > 0 {
			p.tokenExpire = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
		} else {
			p.tokenExpire = time.Now().Add(7200 * time.Second)
		}

		slog.Info("wps-xiezuo: token obtained", "expires_in", tokenResp.ExpiresIn)
		return p.token, nil
	}

	return "", fmt.Errorf("wps-xiezuo: all token endpoints failed: %w", lastErr)
}

// --- Reaction API (typing indicator) ---

func (p *Platform) addReaction(ctx context.Context, rctx replyContext, reactionType string) error {
	token, err := p.getToken(ctx)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: get token: %w", err)
	}

	body, err := json.Marshal(wpsReactionRequest{ReactionType: reactionType})
	if err != nil {
		slog.Error("wps-xiezuo: marshal reaction request", "error", err)
		return fmt.Errorf("wps-xiezuo: marshal reaction request: %w", err)
	}
	url := fmt.Sprintf("%s/v7/chats/%s/messages/%s/reactions/create", p.baseURL, url.PathEscape(rctx.ChatID), url.PathEscape(rctx.MessageID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wps-xiezuo: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: add reaction: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, rerr := io.ReadAll(io.LimitReader(resp.Body, int64(maxErrBodyBytes)+1))
		if rerr != nil {
			return fmt.Errorf("wps-xiezuo: add reaction failed: status=%d (read body: %w)", resp.StatusCode, rerr)
		}
		return fmt.Errorf("wps-xiezuo: add reaction failed: status=%d body=%s", resp.StatusCode, truncateAndRedact(respBody, token))
	}
	return nil
}

func (p *Platform) deleteReaction(ctx context.Context, rctx replyContext, reactionType string) error {
	token, err := p.getToken(ctx)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: get token: %w", err)
	}

	body, err := json.Marshal(wpsReactionRequest{ReactionType: reactionType})
	if err != nil {
		slog.Error("wps-xiezuo: marshal reaction request", "error", err)
		return fmt.Errorf("wps-xiezuo: marshal reaction request: %w", err)
	}
	url := fmt.Sprintf("%s/v7/chats/%s/messages/%s/reactions/delete", p.baseURL, url.PathEscape(rctx.ChatID), url.PathEscape(rctx.MessageID))

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wps-xiezuo: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: delete reaction: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, rerr := io.ReadAll(io.LimitReader(resp.Body, int64(maxErrBodyBytes)+1))
		if rerr != nil {
			return fmt.Errorf("wps-xiezuo: delete reaction failed: status=%d (read body: %w)", resp.StatusCode, rerr)
		}
		return fmt.Errorf("wps-xiezuo: delete reaction failed: status=%d body=%s", resp.StatusCode, truncateAndRedact(respBody, token))
	}
	return nil
}

// --- Optional interface: ProgressStyleProvider ---

// ProgressStyle returns "compact" so that compactProgressWriter enables the
// markdown fallback path. WPS does not support ProgressCardPayloadSupport,
// so "card" would be inappropriate; "compact" is the right value.
func (p *Platform) ProgressStyle() string { return "compact" }

// --- Optional interface: ReplyContextReconstructor ---

func (p *Platform) ReconstructReplyCtx(sessionKey string) (any, error) {
	// Formats:
	//   wps-xiezuo:{company_id}:{chat_id}             - group or legacy P2P
	//   wps-xiezuo:{company_id}:{chat_id}:{sender_id} - P2P, user-scoped
	parts := strings.SplitN(sessionKey, ":", 4)
	if len(parts) < 3 || parts[0] != "wps-xiezuo" {
		return nil, fmt.Errorf("wps-xiezuo: invalid session key %q", sessionKey)
	}
	rc := replyContext{
		ChatID:    parts[2],
		CompanyID: parts[1],
	}
	if len(parts) == 4 {
		rc.ChatType = "p2p"
		rc.SenderID = parts[3]
	}
	return rc, nil
}

// --- Optional interface: TypingIndicator ---

func (p *Platform) StartTyping(ctx context.Context, rctx any) (stop func()) {
	rc, ok := rctx.(replyContext)
	if !ok {
		return func() {}
	}
	if rc.ChatID == "" || rc.MessageID == "" {
		return func() {}
	}
	if err := p.addReaction(ctx, rc, "emoji_busy"); err != nil {
		slog.Debug("wps-xiezuo: add typing reaction failed", "error", err)
	}
	return func() {}
}

// --- Optional interface: TypingIndicatorDone ---

func (p *Platform) AddDoneReaction(rctx any) {
	rc, ok := rctx.(replyContext)
	if !ok {
		return
	}
	if rc.ChatID == "" || rc.MessageID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := p.deleteReaction(ctx, rc, "emoji_busy"); err != nil {
		slog.Debug("wps-xiezuo: delete typing reaction failed", "error", err)
	}
}

// cleanReplyPrefixes lists emoji prefixes whose lines are stripped from
// Reply output when clean_reply is enabled. These correspond to the
// thinking/tool/summary indicators the engine emits during streaming.
var cleanReplyPrefixes = []string{"💭", "🔧", "🧾"}

// --- Clean reply content ---

func cleanReplyContent(content string) string {
	lines := strings.Split(content, "\n")
	var filtered []string
	for _, line := range lines {
		trimmed := strings.TrimLeft(line, " \t")
		skip := false
		for _, prefix := range cleanReplyPrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		filtered = append(filtered, line)
	}
	result := strings.Join(filtered, "\n")
	result = strings.TrimSpace(result)
	if result == "" {
		return content
	}
	return result
}

// --- HTTP response helpers ---

func truncateAndRedact(body []byte, token string) string {
	s := string(body)
	if len(s) > maxErrBodyBytes {
		s = s[:maxErrBodyBytes] + "...(truncated)"
	}
	return core.RedactToken(s, token)
}

// --- Optional interface: PreviewStarter ---

// SendPreviewStart sends the initial card preview message and returns a handle
// for subsequent in-place updates. The Bearer token is used (no KSO-1 signing);
// KSO-1 signing is only required for the update message API.
func (p *Platform) SendPreviewStart(ctx context.Context, rctx any, content string) (any, error) {
	rc, ok := rctx.(replyContext)
	if !ok {
		return nil, fmt.Errorf("wps-xiezuo: invalid reply context type %T", rctx)
	}

	// Defense: if a preview card already exists for this chat, return it
	p.previewMu.Lock()
	if p.previewHandles == nil {
		p.previewHandles = make(map[string]*wpsPreviewHandle)
	}
	if h, exists := p.previewHandles[rc.ChatID]; exists {
		p.previewMu.Unlock()
		slog.Debug("wps-xiezuo: returning existing preview handle for chat", "chat_id", rc.ChatID, "msg_id", h.messageID)
		return h, nil
	}
	p.previewMu.Unlock()

	cardData := buildWPSCard("", core.CardStatusThinking, "")
	reqBody := wpsSendMessageRequest{
		Type: "card",
		Receiver: wpsReceiverInfo{
			Type:       "chat",
			ReceiverID: rc.ChatID,
		},
		Content: wpsMessageContent{
			Card: json.RawMessage(cardData),
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("wps-xiezuo: marshal preview request: %w", err)
	}

	token, err := p.getToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("wps-xiezuo: get token: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v7/messages/create", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("wps-xiezuo: create preview request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("wps-xiezuo: send preview request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxErrBodyBytes)+1))
	if err != nil {
		return nil, fmt.Errorf("wps-xiezuo: send preview: read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("wps-xiezuo: send preview failed: status=%d body=%s", resp.StatusCode, truncateAndRedact(respBody, token))
	}

	var apiResp wpsAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("wps-xiezuo: send preview: parse response: %w", err)
	}
	if apiResp.Code.String() != "0" {
		return nil, fmt.Errorf("wps-xiezuo: send preview failed: code=%s msg=%s", apiResp.Code, core.RedactToken(apiResp.Msg, token))
	}

	var createData wpsMessageCreateData
	if err := json.Unmarshal(apiResp.Data, &createData); err != nil {
		return nil, fmt.Errorf("wps-xiezuo: send preview: parse message_id: %w", err)
	}
	if createData.MessageID == "" {
		return nil, fmt.Errorf("wps-xiezuo: send preview: empty message_id in response")
	}

	handle := &wpsPreviewHandle{
		messageID: createData.MessageID,
		status:    core.CardStatusThinking,
		chatID:    rc.ChatID,
	}

	p.previewMu.Lock()
	p.previewHandles[rc.ChatID] = handle
	p.previewMu.Unlock()

	slog.Info("wps-xiezuo: preview message created", "msg_id", createData.MessageID, "chat_id", rc.ChatID)
	return handle, nil
}

// --- Optional interface: MessageUpdater ---

// UpdateMessage updates an existing card message in-place via
// POST /v7/messages/{message_id}/update.
// Unlike Create (which only needs Bearer), the update API requires BOTH
// Bearer token AND KSO-1 signing.
func (p *Platform) UpdateMessage(ctx context.Context, rctx any, content string) error {
	h, ok := rctx.(*wpsPreviewHandle)
	if !ok {
		return fmt.Errorf("wps-xiezuo: invalid preview handle type %T", rctx)
	}
	if h.messageID == "" {
		return fmt.Errorf("wps-xiezuo: update message: empty message_id in preview handle")
	}

	h.mu.Lock()
	if content == h.lastContent {
		h.mu.Unlock()
		return nil
	}
	h.mu.Unlock()

	cardData := resolveWPSContent(p.Name(), h, content)
	reqBody := wpsUpdateMessageRequest{
		Type: "card",
		Content: wpsMessageContent{
			Card: json.RawMessage(cardData),
		},
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: marshal update request: %w", err)
	}

	token, err := p.getToken(ctx)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: get token: %w", err)
	}

	uri := "/v7/messages/" + url.PathEscape(h.messageID) + "/update"
	date, authHeader := p.kso1Sign("POST", uri, "application/json", body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+uri, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("wps-xiezuo: create update request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Kso-Date", date)
	req.Header.Set("X-Kso-Authorization", authHeader)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("wps-xiezuo: update message request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxErrBodyBytes)+1))
	if err != nil {
		return fmt.Errorf("wps-xiezuo: update message: read response body: %w", err)
	}

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		slog.Error("wps-xiezuo: update message auth/signing error",
			"status", resp.StatusCode,
			"body", truncateAndRedact(respBody, token),
			"hint", "签名或权限问题，请在开发者后台开启接口签名并确认 kso.chat_message.readwrite 权限",
			"app_id", core.RedactToken(p.appID, p.appID))
		return fmt.Errorf("wps-xiezuo: update message failed: status=%d", resp.StatusCode)

	case resp.StatusCode == http.StatusNotFound:
		slog.Warn("wps-xiezuo: message not found, card may have been deleted",
			"msg_id", h.messageID)
		return fmt.Errorf("wps-xiezuo: update message failed: status=404 (message deleted)")

	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("wps-xiezuo: update message failed: status=%d body=%s", resp.StatusCode, truncateAndRedact(respBody, token))
	}

	var apiResp wpsAPIResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return fmt.Errorf("wps-xiezuo: update message: parse response: %w", err)
	}
	if apiResp.Code.String() != "0" {
		return fmt.Errorf("wps-xiezuo: update message failed: code=%s msg=%s", apiResp.Code, core.RedactToken(apiResp.Msg, token))
	}

	slog.Debug("wps-xiezuo: message updated", "msg_id", h.messageID)
	h.mu.Lock()
	h.lastContent = content
	h.mu.Unlock()
	return nil
}

// --- Optional interface: PreviewStatusUpdater ---

func (p *Platform) SetPreviewStatus(handle any, status core.CardStatus) {
	h, ok := handle.(*wpsPreviewHandle)
	if !ok {
		return
	}
	h.mu.Lock()
	h.status = status
	h.mu.Unlock()
}

// --- Optional interface: PreviewFinishPreference ---

func (p *Platform) KeepPreviewOnFinish() bool {
	return true
}

// --- Optional interface: PreviewCleaner ---

func (p *Platform) DeletePreviewMessage(_ context.Context, handle any) error {
	p.previewMu.Lock()
	if p.previewHandles == nil {
		p.previewHandles = make(map[string]*wpsPreviewHandle)
	}
	if h, ok := handle.(*wpsPreviewHandle); ok && h.chatID != "" {
		delete(p.previewHandles, h.chatID)
	}
	p.previewMu.Unlock()
	slog.Debug("wps-xiezuo: message deletion not supported by WPS API, card will persist")
	return nil
}

// --- Compile-time interface assertions ---

var (
	_ core.Platform                  = (*Platform)(nil)
	_ core.ProgressStyleProvider     = (*Platform)(nil)
	_ core.ReplyContextReconstructor = (*Platform)(nil)
	_ core.TypingIndicator           = (*Platform)(nil)
	_ core.TypingIndicatorDone       = (*Platform)(nil)
	_ core.PreviewStarter            = (*Platform)(nil)
	_ core.MessageUpdater            = (*Platform)(nil)
	_ core.PreviewStatusUpdater      = (*Platform)(nil)
	_ core.PreviewFinishPreference   = (*Platform)(nil)
	_ core.PreviewCleaner            = (*Platform)(nil)
)
