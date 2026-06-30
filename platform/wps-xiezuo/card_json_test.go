package wpsxiezuo

import (
	"encoding/json"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

// TestBuildWPSCardStructure 断言 buildWPSCard 返回的 JSON 结构。
// WPS API 期望 content.card 直接是 {config, i18n_items}，
// 不应该有多余的 {type, content} 嵌套层。
func TestBuildWPSCardStructure(t *testing.T) {
	b := buildWPSCard("测试Agent", core.CardStatusThinking, "## 标题\n\n正文")

	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatalf("解析失败: %v", err)
	}

	// 不应该有顶层 type 和 content 字段
	if _, exists := top["type"]; exists {
		t.Errorf("buildWPSCard 不应包含顶层 type 字段（多余嵌套），实际包含: %v", keys(top))
	}
	if _, exists := top["content"]; exists {
		t.Errorf("buildWPSCard 不应包含顶层 content 字段（多余嵌套），实际包含: %v", keys(top))
	}

	// 应该直接有 config 和 i18n_items 字段
	if _, exists := top["config"]; !exists {
		t.Errorf("buildWPSCard 应直接包含 config 字段，实际包含: %v", keys(top))
	}
	i18nRaw, exists := top["i18n_items"]
	if !exists {
		t.Fatalf("buildWPSCard 应直接包含 i18n_items 字段，实际包含: %v", keys(top))
	}

	// i18n_items 必须非空（WPS API min=1 校验）
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(i18nRaw, &items); err != nil {
		t.Fatalf("解析 i18n_items 失败: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("i18n_items 不能为空（WPS API min=1 校验）")
	}

	// 验证完整请求 JSON 的 content.card 路径直接是 {config, i18n_items}
	reqBody := wpsSendMessageRequest{
		Type: "card",
		Receiver: wpsReceiverInfo{
			Type:       "chat",
			ReceiverID: "lLomJ37",
		},
		Content: wpsMessageContent{
			Card: json.RawMessage(b),
		},
	}
	reqJSON, _ := json.Marshal(reqBody)
	var req map[string]json.RawMessage
	if err := json.Unmarshal(reqJSON, &req); err != nil {
		t.Fatalf("解析请求 JSON 失败: %v", err)
	}
	var contentField map[string]json.RawMessage
	if err := json.Unmarshal(req["content"], &contentField); err != nil {
		t.Fatalf("解析 content 失败: %v", err)
	}
	var cardField map[string]json.RawMessage
	if err := json.Unmarshal(contentField["card"], &cardField); err != nil {
		t.Fatalf("解析 content.card 失败: %v", err)
	}
	if _, exists := cardField["i18n_items"]; !exists {
		t.Errorf("content.card 必须直接包含 i18n_items，实际包含: %v", keys(cardField))
	}
	if _, exists := cardField["type"]; exists {
		t.Errorf("content.card 不应包含 type 字段（多余嵌套），实际包含: %v", keys(cardField))
	}
}

func TestBuildWPSCardJSON(t *testing.T) {
	// 打印 buildWPSCard 生成的 JSON 结构，验证是否有多余嵌套
	b := buildWPSCard("测试Agent", core.CardStatusThinking, "## 标题\n\n正文")
	t.Logf("buildWPSCard 输出:\n%s", string(b))

	// 解析并打印层级结构
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	t.Logf("顶层字段: %v", keys(raw))

	// 同时打印完整请求 JSON（模拟 SendPreviewStart 的请求结构）
	reqBody := wpsSendMessageRequest{
		Type: "card",
		Receiver: wpsReceiverInfo{
			Type:       "chat",
			ReceiverID: "lLomJ37",
		},
		Content: wpsMessageContent{
			Card: json.RawMessage(b),
		},
	}
	reqJSON, _ := json.MarshalIndent(reqBody, "", "  ")
	t.Logf("完整请求 JSON:\n%s", string(reqJSON))
}

func keys(m map[string]json.RawMessage) []string {
	k := make([]string, 0, len(m))
	for key := range m {
		k = append(k, key)
	}
	return k
}

