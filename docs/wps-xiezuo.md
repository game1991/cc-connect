# WPS 协作 (Xiezuo) 接入指南

本文档介绍如何将 **cc-connect** 接入 WPS 协作（WPS 365），让你可以通过 WPS 应用聊天远程调用 Claude Code 等 AI 编码代理。

## 前置要求

- WPS 开放平台企业自建应用，已启用应用聊天能力
- `app_id` 和 `app_secret` 凭证
- 一台可运行 cc-connect 的设备（无需公网 IP）
- 已安装并配置好的 AI 编码代理（如 Claude Code、Codex、Gemini CLI）

> **优势**：使用 WebSocket 长连接接收事件，无需公网 IP、无需域名、无需反向代理

---

## 连接模型

WPS 协作平台使用 WebSocket 事件推送 + REST API 双通道架构：

| 通道 | 方向 | 协议 | 认证方式 |
|------|------|------|---------|
| 事件接收 | WPS → cc-connect | WebSocket (`wss://openapi.wps.cn/v7/event/ws`) | KSO-1 HMAC-SHA256 签名 |
| 消息发送 | cc-connect → WPS | REST API (`POST /v7/messages/create`) | Bearer 令牌 |
| 消息更新 | cc-connect → WPS | REST API (`POST /v7/messages/{id}/update`) | Bearer 令牌 + KSO-1 签名 |
| 表态接口 | cc-connect → WPS | REST API (`POST /v7/messages/{id}/reactions/*`) | Bearer 令牌 |

事件载荷使用 AES-256-CBC 加密、HMAC-SHA256 签名验证，cc-connect 通过 WebSocket 写入循环发送 ACK 帧，无需公网回调地址。

---

## 第一步：创建 WPS 开放平台应用

1. 登录 [WPS 开放平台](https://open.wps.cn) 管理后台
2. 创建一个企业自建应用
3. 填写应用名称、描述和图标

---

## 第二步：获取凭证

1. 进入应用的「凭据」页面
2. 记录 **App ID** 和 **App Secret**
3. 确保应用已启用「接口签名」能力（UpdateMessage API 需要 KSO-1 签名）

---

## 第三步：配置应用能力

在应用管理页面，启用以下能力：

- **应用聊天 / 消息**：允许应用收发聊天消息
- **事件 WebSocket 推送**：选择长连接模式接收事件

---

## 第四步：订阅事件

在事件订阅页面，添加以下事件：

| 事件名称 | 事件标识 | 用途 |
|---------|---------|------|
| 应用聊天消息 | `kso.app_chat.message` | 接收用户发送的聊天消息 |
| 应用聊天消息撤回 | `kso.app_chat.message.recall` | 接收消息撤回通知（可选） |

保存事件配置并发布应用版本以使配置生效。

---

## 第五步：配置权限

在权限管理页面，申请以下权限：

| 权限标识 | 用途 |
|---------|------|
| `kso.chat_message.readwrite` | 发送/更新应用聊天消息、添加/删除表态 |

发布权限申请后，需要管理员审批通过。

---

## 第六步：配置 cc-connect

在 `config.toml` 中添加 WPS 协作平台配置：

```toml
[[projects]]
name = "my-project"

[projects.agent]
type = "claudecode"

[projects.agent.options]
work_dir = "/path/to/your/project"

[[projects.platforms]]
type = "wps-xiezuo"

[projects.platforms.options]
app_id = "your-wps-xiezuo-app-id"
app_secret = "your-wps-xiezuo-app-secret"
allow_from = "*"        # 可选；生产环境请设置为 WPS 用户 ID
clean_reply = false     # 可选；过滤思考和工具进度行
```

### 配置项说明

| 配置项 | 必填 | 默认值 | 说明 |
|--------|------|--------|------|
| `app_id` | 是 | - | WPS 开放平台应用 ID |
| `app_secret` | 是 | - | WPS 开放平台应用密钥 |
| `allow_from` | 否 | 所有用户 | 允许使用机器人的 WPS 用户 ID 列表（逗号分隔），生产环境务必设置 |
| `clean_reply` | 否 | `false` | 过滤回复中的思考（💭）和工具调用（🔧、🧾）进度行 |
| `base_url` | 否 | `https://openapi.wps.cn` | WPS REST API 基础地址覆盖，用于私有部署或测试环境 |
| `progress_style` | 否 | `compact` | 进度显示风格，`compact` 在卡片中内联显示工具进度 |

> **安全提示**：请勿将 `app_secret` 提交到版本控制。`config.toml` 支持环境变量替换，例如 `app_secret = "${WPS_XIEZUO_APP_SECRET}"`。

---

## 第七步：启动并验证

```bash
cc-connect -config /path/to/config.toml
```

正常启动后日志应包含：

```text
level=INFO msg="wps-xiezuo: connecting" endpoint=wss://openapi.wps.cn/v7/event/ws
level=INFO msg="wps-xiezuo: connected"
level=INFO msg="platform started" project=my-project platform=wps-xiezuo
```

在 WPS 中向应用聊天发送一条消息。cc-connect 应收到加密事件、完成 ACK、将文本转发给配置的编码代理、并通过 WPS 消息 API 将回复发回。

---

## 功能详解

### 实时卡片预览

WPS 协作平台支持**卡片就地更新**（in-place card update），cc-connect 利用此能力实现实时预览：

1. **发送预览卡片** — 用户发送消息后，cc-connect 立即通过 `POST /v7/messages/create` 创建一张初始卡片，显示 "💭 思考中" 状态
2. **就地更新卡片** — 代理工作过程中，cc-connect 通过 `POST /v7/messages/{id}/update` 就地更新卡片内容和状态（🔧 工作中 → ✅ 完成 / ❌ 出错）
3. **防重复创建** — 平台级去重机制确保同一聊天不会创建多张卡片（`previewHandles` 按 chatID 去重）
4. **内容去重** — 当卡片内容未变化时自动跳过 API 调用，减少无效请求
5. **完成保留** — 卡片在代理完成后保留（`KeepPreviewOnFinish() = true`），WPS API 不支持消息删除

卡片结构：

```json
{
  "type": "card",
  "content": {
    "card": {
      "config": {},
      "i18n_items": [{
        "key": "zh-CN",
        "value": {
          "header": { "title": "CC", "subtitle": "<代理名>" },
          "elements": [
            { "text": { "tag": "text", "text": { "type": "markdown", "content": "✅ 完成" } } },
            { "hr": { "tag": "hr" } },
            { "text": { "tag": "text", "text": { "type": "markdown", "content": "<代理回复内容>" } } }
          ]
        }
      }]
    }
  }
}
```

### 紧凑进度风格

WPS 平台实现了 `ProgressStyleProvider`，返回 `"compact"` 风格。这使引擎使用 `compactProgressWriter` 在卡片中**内联显示工具调用进度**（如 `🔧 Bash: ls -la`），而非占用独立消息。

### 输入表态

cc-connect 利用 WPS 表态（Reaction）API 实现"正在输入"指示：

- 收到消息后，调用 `POST /v7/messages/{id}/reactions/add` 添加 `emoji_busy` 表态
- 代理完成后，调用 `POST /v7/messages/{id}/reactions/delete` 移除该表态

### Markdown 渲染

WPS 协作平台使用 CommonMark 子集渲染 Markdown，但**单换行符 `\n` 被折叠为空格**。cc-connect 自动将 `\n` 转换为 `  \n`（双空格 + 换行）以强制硬换行，此转换对代码块内无害且幂等。

### 内容截断

WPS 卡片内容有 15,000 字符上限（`wpsCardMaxChars = 15000`）。超长内容截断策略：

1. 保留最后 14,000 字符（`wpsCardTruncateKeep = 14000`）
2. 优先在段落边界（`\n\n`）处截断，保证断点整洁
3. 找不到段落边界时硬截断
4. 追加截断提示 `...（内容过长，已截断）`
5. 使用 `utf8.RuneCountInString` 按**字符数**（非字节数）计算，确保中日韩文字正确处理

### 回复内容清洗

启用 `clean_reply = true` 后，cc-connect 会过滤回复中的以下进度行：

| Emoji | 含义 |
|-------|------|
| 💭 | 思考过程 |
| 🔧 | 工具调用 |
| 🧾 | 工具输出摘要 |

如果过滤后内容为空，则返回原始内容不变。

### 消息撤回

当收到 `kso.app_chat.message.recall` 事件时，cc-connect 会通知代理停止当前会话（如果存在活跃会话）。

---

## 架构图

```
┌─────────────────────────────────────────────────────────────┐
│                       WPS 云端                               │
│                                                              │
│   用户消息 ──→ WPS 开放平台 ──→ WebSocket Gateway            │
│                                      │                       │
└──────────────────────────────────────┼───────────────────────┘
                                       │
                                       │ WebSocket 长连接
                                       │ (无需公网IP)
                                       ▼
┌─────────────────────────────────────────────────────────────┐
│                      你的本地环境                            │
│                                                              │
│   cc-connect ◄──► Claude Code CLI ◄──► 你的项目代码         │
│       │                                                     │
│       ├── WebSocket 读取循环（事件解密 + ACK）               │
│       ├── WebSocket 写入循环（ACK + 表态 + 心跳）           │
│       ├── REST API 发送/更新卡片消息                        │
│       └── 指数退避自动重连（max 60s）                       │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

---

## KSO-1 签名机制

WPS 开放平台 v7 API 的消息更新接口需要双重认证：

1. **Bearer 令牌** — 通过 `client_credentials` 授权模式获取
2. **KSO-1 签名** — HMAC-SHA256 请求签名

签名过程：

```
date = UTC 时间（RFC 7231 格式）
bodyHash = SHA256Hex(requestBody)       // 请求体 SHA-256 哈希
stringToSign = "KSO-1" + method + uri + contentType + date + bodyHash
signature = HMAC-SHA256(appSecret, stringToSign)
authHeader = "KSO-1 {appID}:{signature}"
```

cc-connect 自动完成签名和令牌缓存，无需手动操作。令牌在过期前 60 秒自动刷新，支持 `/oauth2/token` 和 `/openapi/oauth2/token` 两个端点自动回退。

---

## 安全注意事项

- **生产环境务必设置 `allow_from`**，限制允许使用机器人的用户 ID
- **切勿将 `app_secret` 提交到版本控制**，使用环境变量替换 `${WPS_XIEZUO_APP_SECRET}`
- Debug 日志默认不打印解密后的 WPS 消息载荷
- URL 路径中的 MessageID 和 ChatID 已使用 `url.PathEscape` 防注入
- HTTP 响应体读取使用 `io.LimitReader` 限制为 64KB，防止内存耗尽
- API 响应码使用 `json.Number` 解析，防御字符串/整数类型混用

---

## 常见问题

### Q: 连接后立即断开？

- 确认 `app_id` 和 `app_secret` 正确
- 确认应用已启用 WebSocket 事件推送
- 确认应用已发布/对目标组织可用
- 检查是否启用了「接口签名」能力

### Q: 消息能收到但回复发送失败？

- 确认应用拥有 `kso.chat_message.readwrite` 权限
- 检查租户是否需要特定的 token 端点（cc-connect 自动尝试两个端点）
- 确认目标聊天允许应用消息

### Q: 卡片更新报 401/403？

- 确认应用已启用 KSO-1 接口签名能力
- 确认应用拥有 `kso.chat_message.readwrite` 权限
- 检查系统时钟是否准确（签名依赖 UTC 时间）

### Q: 卡片更新报 404？

- 原始消息可能已被用户删除，cc-connect 会记录警告并返回错误

### Q: 机器人回复了不该回复的用户？

- 设置 `allow_from` 为允许的 WPS 用户 ID 列表
- 用户可发送 `/whoami` 查看自己被 cc-connect 识别的 ID

### Q: 中文内容被截断得很短？

- 已修复：cc-connect 使用 `utf8.RuneCountInString` 按**字符数**（非字节数）截断，中日韩文字每个占 3 字节但只计 1 字符

### Q: 同一会话出现多张预览卡片？

- 已修复：cc-connect 在平台层按 chatID 去重，重复调用只返回已有的卡片句柄

---

## 参考链接

- [WPS 开放平台](https://open.wps.cn)
- [WPS 应用开发文档](https://open.wps.cn/documents/app-integration-dev)
- [WPS Webhook 机器人文档](https://open.wps.cn/documents/app-integration-dev/guide/robot/webhook)
- [WPS 开放平台 API 参考](https://open.wps.cn/documents/app-integration-dev/api)

---

## 下一步

- [接入飞书](./feishu.md)
- [接入钉钉](./dingtalk.md)
- [接入 Telegram](./telegram.md)
- [接入 Slack](./slack.md)
- [接入 Discord](./discord.md)
- [返回首页](../README.md)
