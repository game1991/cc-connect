# WPS 协作卡片消息原地更新功能设计

日期: 2026-06-23
状态: 已批准

## 1. 背景

当前 cc-connect 的 WPS 平台处于引擎渲染层级的最底层（Legacy），只能通过 `POST /v7/messages/create` 发送纯文本/markdown 消息，无法原地更新。用户感知到的问题是：agent 回复时没有任何"处理中"的标识，也不知道 agent 在调用哪些工具，体验远差于飞书和 Discord 平台。

经端到端验证，WPS v7 API 支持：
- `POST /v7/messages/create`：创建消息（支持 text/rich_text/image/file/audio/video/card）
- `POST /v7/messages/{message_id}/update`：更新消息（**仅支持 type=card**），需 KSO-1 签名
- `POST /v7/chats/{chat}/messages/{msg}/reactions/create` / `delete`：表情标识

KSO-1 签名算法已通过官方文档示例验证（两个测试用例 100% 匹配），并通过真实 API 端到端测试确认卡片创建和更新均可成功调用。

## 2. 方案选型

| 方案 | 路径 | 优点 | 缺点 | 结论 |
|------|------|------|------|------|
| A | PreviewStarter + MessageUpdater（飞书模式） | 复用引擎大量逻辑，与飞书行为一致，可扩展 | 需构建 WPS 卡片 JSON | **采用** |
| B | StreamingCardPlatform（钉钉模式） | 路径简单 | 语义不匹配，无法扩展状态页脚 | 不采用 |
| C | CardSender + 自定义更新逻辑 | 完全灵活 | 重复造轮子 | 不采用 |

## 3. 架构

### 3.1 新增接口实现

WPS 平台实现以下 5 个引擎接口：

| 接口 | 方法 | 核心逻辑 |
|------|------|---------|
| `PreviewStarter` | `SendPreviewStart(ctx, rctx, content) (previewHandle, error)` | 调用 `POST /v7/messages/create` 发送 `type=card`，返回 `message_id` |
| `MessageUpdater` | `UpdateMessage(ctx, rctx, content) error` | 调用 `POST /v7/messages/{id}/update` + KSO-1 签名更新卡片 |
| `PreviewStatusUpdater` | `SetPreviewStatus(handle, status)` | 存储状态到 handle，下次 UpdateMessage 时写入 elements 首行 |
| `PreviewFinishPreference` | `KeepPreviewOnFinish() bool` | 返回 `true`（原地更新为最终回答） |
| `PreviewCleaner` | `DeletePreviewMessage(ctx, handle) error` | WPS 无消息删除 API，静默返回 nil |

### 3.2 previewHandle 数据结构

```go
type wpsPreviewHandle struct {
    mu        sync.Mutex
    MessageID string
    Status    core.CardStatus
    ChatID    string
}
```

`sync.Mutex` 保护 Status 字段的并发访问——`streamPreview.flushLocked`（流式更新线程）和 `SetPreviewStatus`（事件循环线程）会同时读写 handle。

引擎约定：`SendPreviewStart` 返回的 handle 传给 `UpdateMessage`、`SetPreviewStatus`、`DeletePreviewMessage`。

### 3.3 渲染层级

实现后 WPS 从 Legacy 升至第三层（流式预览），引擎自动检测接口能力并选择渲染路径：

```
1. StreamingCardPlatform   → 钉钉（WPS 未实现，跳过）
2. RichCardSupporter       → 飞书 Card 2.0（WPS 卡片能力有限，未实现，跳过）
3. PreviewStarter + MessageUpdater → Discord、WPS（新增）
4. Legacy                  → 兜底
```

WPS 不实现 `RichCardSupporter`，因此无法进入第二层（飞书独占）。第三层已可提供完整流式预览体验。

## 4. 卡片 JSON 构建

### 4.1 卡片模板结构

WPS 卡片 elements 格式：`{组件标签名: 组件配置}`（与飞书直接数组格式不同）。

> WPS 使用 `i18n_items` 包裹多语言元素列表（key 为 locale），而非飞书的直接 `elements` 数组。这是 WPS 卡片协议的多语言设计要求，必须按此结构构建。

```json
{
  "type": "card",
  "content": {
    "card": {
      "config": {},
      "i18n_items": [{
        "key": "zh-CN",
        "value": {
          "header": {
            "title": {"tag": "text", "text": {"type": "plain", "content": "CC"}},
            "subtitle": {"tag": "text", "text": {"type": "plain", "content": "Agent Name"}}
          },
          "elements": [
            {"text": {"tag": "text", "text": {"type": "markdown", "content": "💭 正在思考..."}}},
            {"hr": {"tag": "hr"}},
            {"text": {"tag": "text", "text": {"type": "markdown", "content": "最终回答内容"}}}
          ]
        }
      }]
    }
  }
}
```

### 4.2 状态渲染规则

| 阶段 | elements 首行 | 后续 elements | header subtitle |
|------|--------------|--------------|----------------|
| 思考中 | `💭 正在思考...` | 无 | Agent Name |
| 工具调用 | `🔧 正在工作...` | 工具列表（每行一个） | Agent Name |
| 流式文本 | `✅ 回复完成` | 分隔线 + markdown 文本 | Agent Name |
| 错误 | `❌ 处理失败` | 错误信息 | Agent Name |

工具列表格式：
```
🔧 ReadFile /src/main.go ✅
🔧 Grep "pattern" ❌
🔧 Bash "go test" ✅
```

最多显示 10 个工具调用（WPS 卡片单个 markdown 元素的 `content` 字段有 15000 字符限制），超出显示 `...还有 N 个调用`。

流式文本超过 15000 字符（单个 markdown 元素 `content` 字段限制）时，保留最后 14000 字符（截断头部），并在截断处追加 `\n\n...（内容过长，已截断）`。截断在段落边界执行：先按 `\n\n` 分割，再从最后一个完整段落开头保留。若段落分割失败，硬截断到 14000 字符。

> 已验证 WPS 卡片 markdown 组件支持 emoji 渲染，✅❌ 可正常显示。

### 4.3 构建函数签名

WPS **不实现** `ProgressCardPayloadSupport`，因此 `compactProgressWriter` 自动走 markdown 回退路径。`UpdateMessage` 只需处理纯 markdown 和空字符串两种格式，无需解析 `ProgressCardPayloadPrefix` 结构化 payload。

工具列表由 WPS 平台层自行维护，数据来源为引擎事件循环传入的 `EventToolUse` / `EventToolResult`，格式参考 `core/progress_compact.go` 中 `ProgressCardEntry` 的 markdown 回退渲染。

```go
func buildWPSCard(agentName string, status core.CardStatus, toolLines string, markdown string) []byte
func statusEmoji(s core.CardStatus) string
```

发送和更新共用同一个构建函数。`toolLines` 为预渲染的工具列表 markdown 文本（每行一个工具调用），由 `compactProgressWriter` 的 markdown 回退渲染生成后传入 `UpdateMessage`，WPS 平台层不做增量拼接。

### 4.4 引擎 content 转换

引擎传给 `UpdateMessage` 的 `content` 有两种格式（WPS 不实现 `ProgressCardPayloadSupport`，不会收到结构化 payload）：

| 格式 | 检测方式 | 处理 |
|------|---------|------|
| 纯 markdown 文本 | 默认 | 直接放入 elements markdown text 组件 |
| 空字符串 | `content == ""` | 只更新状态行，不修改正文 |

转换函数：
```go
func resolveWPSContent(agentName string, handle *wpsPreviewHandle, content string) []byte
```

## 5. KSO-1 签名

### 5.1 通用签名函数

从现有 `signWSHeader` 提取通用签名逻辑：

```go
func (p *Platform) kso1Sign(method, uri, contentType string, body []byte) (date, authHeader string)
```

签名内容：`"KSO-1" + method + uri + contentType + date + sha256Hex(body)`

现有 `signWSHeader` 改为调用：`kso1Sign("GET", uri, "", nil)`，保留 `signWSHeader` 方法名不变，仅内部分发到通用函数。外部调用方无需改动。

### 5.2 签名要求

- 更新消息 API 必须传 `X-Kso-Date` 和 `X-Kso-Authorization`
- 创建消息 API 只需 Bearer Token（不需要签名）
- 权限要求：`kso.chat_message.readwrite`
- Bearer Token 使用现有 WPS OAuth2 `client_credentials` 流程获取，token 过期时自动刷新（现有实现已覆盖）

## 6. 错误处理与降级

所有日志输出必须对 appID/appSecret 使用 `core.RedactToken()` 脱敏，防止签名头或 OAuth token 泄露到日志。

### 6.1 降级策略

```
卡片创建成功 → 卡片更新成功 → 保持卡片模式
     │                │
     │                └─→ 更新失败(401/403/签名错误) → 设置 degraded=true，回退 text 模式
     │                                        后续消息走 p.Send() 发纯文本
     │
     └─→ 创建失败 → 直接走 text 模式（现有行为不变）
```

`streamPreview` 设置 `degraded=true` 后，引擎自动走 legacy 路径。

### 6.2 错误码映射

| WPS API 错误 | 含义 | 处理 |
|-------------|------|------|
| `code=0` | 成功 | 继续 |
| `400000002` | 参数格式错误 | 日志记录，降级 |
| `401` / 签名错误 | KSO-1 未开启或密钥不匹配 | 日志提示"请在开发者后台开启接口签名"，脱敏 appID 后降级 |
| `403` / 权限不足 | 缺少 `kso.chat_message.readwrite` | 日志提示，降级 |
| `404` | message_id 不存在 | 降级（卡片可能已被删除） |
| `429` | 频率限制 | 降级 |
| `5xx` | 服务端错误 | 降级 |

### 6.3 节流控制

使用引擎框架已有节流逻辑（`StreamPreviewCfg.IntervalMs` 默认 1500ms，`MinDeltaChars` 默认 30 字符）。如果实测发现 WPS API 有更严格的频率限制，可通过配置调整。

引擎同时启动 `compactProgressWriter`（工具调用阶段）和 `streamPreview`（文本流阶段），两者通过同一个 `message_id` 原地更新。引擎事件循环串行调用 `UpdateMessage`，HTTP 请求无并发问题。但 `streamPreview` 的 timer goroutine 与事件循环会并发读写 `wpsPreviewHandle.Status`，因此 §3.2 中 handle 需要 `sync.Mutex` 保护。

## 7. 完整数据流

```
用户发消息 @WPS机器人
    │
    ▼
WPS WebSocket 推送事件 → 解密 → 构造 replyContext
    │
    ▼
引擎检测平台能力：
    ✓ PreviewStarter → 可流式预览
    ✓ MessageUpdater → 可原地更新
    ✓ PreviewStatusUpdater → 可设状态
    ✓ PreviewFinishPreference → KeepPreviewOnFinish=true
    │
    ▼
EventThinking:
    PreviewStarter.SendPreviewStart("", "")
    → POST /v7/messages/create (type=card)
    → 返回 wpsPreviewHandle{MessageID, Status:thinking}
    → 卡片显示: "💭 正在思考..."
    │
    ▼
EventToolUse (compactProgressWriter):
    首次: PreviewStarter.SendPreviewStart → POST /v7/messages/create (type=card)
    后续: MessageUpdater.UpdateMessage → POST /v7/messages/{id}/update (type=card, KSO-1签名)
    → 卡片显示: "🔧 正在工作..."
              "🔧 ReadFile /src/main.go ✅"
              "🔧 Grep \"pattern\" ✅"
    │
    ▼
EventText (streamPreview):
    MessageUpdater.UpdateMessage
    → POST /v7/messages/{id}/update (KSO-1签名)
    → 卡片显示: "✅ 回复完成"
              "---"
              "流式 markdown 文本..."
    │
    ▼
EventResult:
    streamPreview.finish(fullResponse, statusFooter)
    → 最终 UpdateMessage
    → 卡片显示: "✅ 回复完成"
              "---"
              "完整回答 markdown"
```

## 8. 文件变更清单

| 文件 | 变更类型 | 说明 |
|------|---------|------|
| `platform/wps-xiezuo/wpsxiezuo.go` | 修改 | 新增 5 个接口方法 + KSO-1 通用签名函数 + 卡片构建函数 |
| `platform/wps-xiezuo/wpsxiezuo.go` | 修改 | 接口断言 var 块新增 5 行 |
| `platform/wps-xiezuo/wpsxiezuo_test.go` | 修改 | 新增卡片构建、签名、更新 API 的测试 |

不需要新增文件。

## 9. wpsxiezuo.go 新增代码结构

```go
// --- 编译时接口断言 ---
var (
    _ core.Platform                  = (*Platform)(nil)
    _ core.ReplyContextReconstructor = (*Platform)(nil)
    _ core.TypingIndicator           = (*Platform)(nil)
    _ core.TypingIndicatorDone       = (*Platform)(nil)
    _ core.PreviewStarter            = (*Platform)(nil)   // 新增
    _ core.MessageUpdater            = (*Platform)(nil)   // 新增
    _ core.PreviewStatusUpdater      = (*Platform)(nil)   // 新增
    _ core.PreviewFinishPreference   = (*Platform)(nil)   // 新增
    _ core.PreviewCleaner            = (*Platform)(nil)   // 新增
)

// --- KSO-1 签名（通用版本） ---
func (p *Platform) kso1Sign(method, uri, contentType string, body []byte) (date, authHeader string)
// signWSHeader 内部改为调用 kso1Sign("GET", uri, "", nil)

// --- 卡片 JSON 构建 ---
func buildWPSCard(agentName string, status core.CardStatus, toolLines string, markdown string) []byte
func statusEmoji(s core.CardStatus) string

// --- 引擎 content 转换 ---
func resolveWPSContent(agentName string, handle *wpsPreviewHandle, content string) []byte

// --- PreviewStarter ---
func (p *Platform) SendPreviewStart(ctx context.Context, rctx any, content string) (any, error)

// --- MessageUpdater ---
func (p *Platform) UpdateMessage(ctx context.Context, rctx any, content string) error

// --- PreviewStatusUpdater ---
func (p *Platform) SetPreviewStatus(handle any, status core.CardStatus)

// --- PreviewFinishPreference ---
func (p *Platform) KeepPreviewOnFinish() bool

// --- PreviewCleaner ---
func (p *Platform) DeletePreviewMessage(ctx context.Context, handle any) error
```

## 10. 测试策略

| 测试类型 | 覆盖内容 |
|---------|---------|
| 单元测试 | KSO-1 签名算法（使用官方示例数据）、卡片 JSON 构建、statusEmoji、renderToolLines |
| 集成测试（mock HTTP） | SendPreviewStart 调用创建 API + 返回 handle、UpdateMessage 调用更新 API + 签名验证、降级逻辑 |
| 边界测试 | 空内容更新、15000 字符截断（段落边界 + 硬截断）、10 个以上工具调用截断、签名错误降级 |

不需要端到端真实 API 测试（已在验证阶段确认）。

## 11. 不做的事

- 不实现 `StatusFooterSender`/`StatusFooterUpdater`（WPS 卡片无独立状态栏组件，可后续通过 elements 内嵌替代）
- 不实现 `RichCardSupporter`（WPS 卡片能力有限，不值得做复杂的富卡片构建器）
- 不实现 `StreamingCardPlatform`（语义不匹配，留作未来备选）
- 不实现 `CardSender`（用 `PreviewStarter` 路径已覆盖）
- 不新增配置项（使用引擎默认节流参数，不暴露 WPS 特定配置）
- 不修改引擎核心代码（所有改动在 WPS 平台层）

### 演进预留

- `DeletePreviewMessage` 返回 nil：待 WPS 开放消息删除 API 后，可实现真正删除并切换 `KeepPreviewOnFinish()=false`
- `ProgressCardPayloadSupport`：当前不实现，若未来 WPS 卡片支持更丰富的结构化组件，可实现以获得更精细的工具调用渲染
