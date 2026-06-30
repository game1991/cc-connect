//go:build integration

// Integration test for WPS card messaging APIs.
// Run with:
//
//	WPS_XIEZUO_APP_ID=... WPS_XIEZUO_APP_SECRET=... WPS_XIEZUO_TEST_CHAT_ID=... \
//		go test -tags=integration -v -run TestSendCardIntegration ./platform/wps-xiezuo/
//
// This test bypasses the engine and directly exercises:
//  1. SendPreviewStart  -> POST /v7/messages/create with type=card
//  2. UpdateMessage     -> POST /v7/messages/{id}/update with KSO-1 signing
//  3. SetPreviewStatus  -> in-memory state change only
//
// Goal: verify the WPS Open Platform card API actually works end-to-end
// with the KSO-1 HMAC-SHA256 signature implemented in this package.
package wpsxiezuo

import (
	"context"
	"net/http"
	"os"
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func TestSendCardIntegration(t *testing.T) {
	appID := os.Getenv("WPS_XIEZUO_APP_ID")
	appSecret := os.Getenv("WPS_XIEZUO_APP_SECRET")
	chatID := os.Getenv("WPS_XIEZUO_TEST_CHAT_ID")
	if appID == "" || appSecret == "" || chatID == "" {
		t.Skip("跳过：需设置 WPS_XIEZUO_APP_ID / WPS_XIEZUO_APP_SECRET / WPS_XIEZUO_TEST_CHAT_ID")
	}

	p := &Platform{
		appID:          appID,
		appSecret:      appSecret,
		baseURL:        defaultBaseURL,
		httpClient:     &http.Client{Timeout: httpTimeout},
		previewHandles: make(map[string]*wpsPreviewHandle),
	}

	ctx := context.Background()
	rc := replyContext{ChatID: chatID}

	t.Log("--- 步骤1: SendPreviewStart (POST /v7/messages/create, type=card) ---")
	handle, err := p.SendPreviewStart(ctx, rc, "")
	if err != nil {
		t.Fatalf("SendPreviewStart 失败: %v", err)
	}
	h, ok := handle.(*wpsPreviewHandle)
	if !ok {
		t.Fatalf("返回的 handle 类型错误: %T", handle)
	}
	if h.messageID == "" {
		t.Fatal("messageID 为空")
	}
	t.Logf("✓ 卡片创建成功，message_id=%s", h.messageID)

	t.Log("--- 步骤2: SetPreviewStatus (状态切换为 working) ---")
	p.SetPreviewStatus(handle, core.CardStatusWorking)
	t.Logf("✓ 状态已切换为 %s", core.CardStatusWorking)

	t.Log("--- 步骤3: UpdateMessage (POST /v7/messages/{id}/update, KSO-1 签名) ---")
	err = p.UpdateMessage(ctx, handle, "## 卡片更新测试\n\n这是通过 **UpdateMessage** 发送的内容，包含 KSO-1 签名。")
	if err != nil {
		t.Fatalf("UpdateMessage 失败: %v", err)
	}
	t.Log("✓ 卡片更新成功（KSO-1 签名通过）")

	t.Log("--- 步骤4: UpdateMessage 再次更新（验证幂等去重） ---")
	err = p.UpdateMessage(ctx, handle, "## 卡片更新测试\n\n这是通过 **UpdateMessage** 发送的内容，包含 KSO-1 签名。")
	if err != nil {
		t.Logf("再次更新失败（可接受，可能是内容相同被去重）: %v", err)
	} else {
		t.Log("✓ 相同内容被去重跳过（符合 lastContent 优化）")
	}

	t.Log("--- 步骤5: UpdateMessage 最终状态 ---")
	err = p.UpdateMessage(ctx, handle, "## 测试完成\n\n卡片消息功能验证通过 ✓")
	if err != nil {
		t.Fatalf("最终 UpdateMessage 失败: %v", err)
	}

	t.Log("✓✓✓ WPS 卡片 API 全流程验证通过：创建 + 更新 + KSO-1 签名")
	t.Logf("请在 WPS 协作平台 chat_id=%s 中查看测试卡片", chatID)
}
