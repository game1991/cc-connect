package wpsxiezuo

import (
	"encoding/json"
)

// --- WPS event frame types ---

type wpsEventFrame struct {
	Topic         string `json:"topic"`
	Operation     string `json:"operation"`
	Time          int64  `json:"time"`
	Nonce         string `json:"nonce"`
	Signature     string `json:"signature"`
	EncryptedData string `json:"encrypted_data"`
	AccessKey     string `json:"access_key"`
}

type wpsGoAwayFrame struct {
	Type        string `json:"type"`
	Reason      string `json:"reason"`
	Message     string `json:"message"`
	ReconnectMs int64  `json:"reconnect_ms,omitempty"`
}

type pongFrame struct {
	Type string `json:"-"`
	Data string
}

type pingControl struct{}

// --- Decrypted event data ---

type wpsMessageData struct {
	Chat      wpsChatInfo    `json:"chat"`
	CompanyID string         `json:"company_id"`
	Message   wpsMessageInfo `json:"message"`
	SendTime  int64          `json:"send_time"`
	Sender    wpsSenderInfo  `json:"sender"`
}

type wpsChatInfo struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

type wpsMessageInfo struct {
	Content json.RawMessage `json:"content"`
	ID      string          `json:"id"`
	Type    string          `json:"type"`
}

type wpsSenderInfo struct {
	ID   string `json:"id"`
	Type string `json:"type"`
}

// --- Message create API ---

type wpsSendMessageRequest struct {
	Type     string            `json:"type"`
	Receiver wpsReceiverInfo   `json:"receiver"`
	Content  wpsMessageContent `json:"content"`
}

type wpsReceiverInfo struct {
	Type       string `json:"type"`
	ReceiverID string `json:"receiver_id"`
}

type wpsMessageContent struct {
	Text wpsTextContent  `json:"text"`
	Card json.RawMessage `json:"card,omitempty"`
}

type wpsTextContent struct {
	Content string `json:"content"`
	Type    string `json:"type"`
}

// --- WPS API response ---

type wpsAPIResponse struct {
	Code json.Number    `json:"code"`
	Msg  string         `json:"msg"`
	Data json.RawMessage `json:"data"`
}

type wpsMessageCreateData struct {
	MessageID string `json:"message_id"`
}

// --- Message update API ---

type wpsUpdateMessageRequest struct {
	Type    string            `json:"type"`
	Content wpsMessageContent `json:"content"`
}

// --- Token API ---

type wpsTokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"`
}

// --- Reaction API ---

type wpsReactionRequest struct {
	ReactionType string `json:"reaction_type"`
}
