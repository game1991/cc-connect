# cc-connect 产品指南

> AI 编码 Agent × 即时通讯，一句话写代码 | v1.3.3-beta | 最后更新 2026-06-14

---

## 1. cc-connect 是什么

cc-connect 是 AI 编码 Agent 与即时通讯平台的桥接工具。用户在飞书、Telegram、Slack 等聊天窗口中即可操控 Claude Code、Codex、Gemini CLI 等 Agent，完成代码编写、文件读写、命令执行等任务。

**核心价值**：把 Agent 从终端搬到聊天窗口 — 不需要 SSH、不需要 IDE，在手机上也能让 AI 改代码。

### 架构全景

```
┌──────────┐     ┌─────────────────────────────┐     ┌──────────────┐
│ 飞书/TG/  │     │          Engine              │     │  Agent 进程   │
│ Slack/... │◄───►│  消息路由 / 会话管理 / 权限  │◄───►│  Claude Code │
│ IM 平台   │     │  卡片渲染 / 流式 / i18n      │     │  Codex       │
└──────────┘     ├─────────────────────────────┤     │  Gemini CLI  │
                 │ Bridge WS (9810)             │     │  ...         │
                 │ Management API + Web UI (9820)│     └──────────────┘
                 │ Webhook (9111)               │
                 └─────────────────────────────┘
```

- **Engine** 是核心枢纽，路由消息、管理会话、执行权限策略
- **Bridge** 允许外部 WebSocket 适配器接入，无需写 Go 代码即可扩展新平台
- **Web UI** 是嵌入二进制的 React SPA，零额外部署
- **Webhook** 提供 HTTP 触发端点，可用于 Git hooks、CI/CD 等

### 适用场景

| 场景 | 说明 |
|------|------|
| 手机改代码 | 在通勤途中通过飞书/Telegram 发送指令，Agent 直接修改仓库 |
| 团队审查 | 多人在群聊中查看 Agent 输出、审批工具权限 |
| 定时巡检 | 配置 Cron 任务，每天自动执行 lint、测试、部署检查 |
| 多 Agent 编排 | 一个项目可配置多个 Agent（如 Claude Code + Codex），通过 Provider 切换 |

---

## 2. 功能全览

### Agent（14 个）

| Agent | 交互方式 | 说明 |
|-------|----------|------|
| Claude Code | 进程流式 | 主力支持，6 种权限模式 |
| Codex | 进程流式 | OpenAI，4 种模式 |
| Gemini CLI | 进程流式 | Google，4 种模式 |
| Cursor | tmux 屏幕抓取 | 需 tmux 环境 |
| ACP | JSON-RPC over stdio | Agent Control Protocol |
| Devin | ACP 封装 | Devin 默认配置 |
| Copilot | Content-Length 帧 | 2 种模式 |
| Antigravity | 进程流式 | 3 种模式 |
| iFlow | 进程流式 | 4 种模式，含工具超时 |
| Kimi | 进程流式 | 4 种模式 |
| OpenCode | 进程流式 | 2 种模式，持久模型缓存 |
| Qoder | 进程流式 | 2 种模式 |
| Pi | 进程流式 | 2 种模式，6 级推理强度，会话文件管理 |
| tmux | tmux 屏幕抓取 | 自动创建窗格，自定义 Shell |

### IM 平台（13 个 + Bridge 服务）

| 平台 | 注册名 | 连接方式 | 特色能力 | 需公网 IP |
|------|--------|----------|----------|-----------|
| 飞书/Lark | `feishu` | 长连接事件 | 交互式卡片、按钮回调、QR 设置 | 否 |
| Telegram | `telegram` | Long Polling | HTML Markdown 渲染 | 否 |
| Discord | `discord` | Gateway WS | Embed、按钮交互 | 否 |
| Slack | `slack` | Socket Mode | Block Kit、斜杠命令、typing emoji | 否 |
| 钉钉 | `dingtalk` | Stream SDK（长连接） | AI 流式卡片（降级为文本） | 否 |
| 企业微信 | `wecom` | HTTP 回调（默认）/ WebSocket | 文本 + 图片 | 是 |
| 微信个人号 | `weixin` | ilink 协议 | QR 码登录 | 否 |
| QQ | `qq` | WebSocket（OneBot v11） | 文本 + 图片 | 视配置 |
| QQ Bot | `qqbot` | WebSocket（QQ Bot Gateway v2） | 文本 + 图片 | 视配置 |
| LINE | `line` | HTTP Webhook | 文本 + 图片 | 是 |
| 微博 | `weibo` | WebSocket（微博 Open IM） | 文本 + 图片 | 是 |
| MAX | `max` | Long Poll（默认）/ Webhook | 三种部署拓扑 | 视配置 |
| WPS 协作 | `wps-xiezuo` | WebSocket（HMAC-SHA256 签名） | 文本 + 图片 | 否 |

> **Bridge** 不是 platform 适配器，而是内置的 WebSocket 服务（端口 9810），允许外部适配器声明式接入，无需写 Go 代码即可扩展新平台。

### Web 管理界面

内置 React SPA，编译后嵌入 Go 二进制，零额外部署。

| 页面 | 功能 |
|------|------|
| Dashboard | 版本、运行时间、连接状态 |
| Projects | 项目 CRUD、3 步添加向导 |
| Chat | 实时聊天、会话管理、40+ 斜杠命令 |
| Sessions | 列表、聊天、批量删除（/delete 1,2,3 或 /delete 3-7） |
| Cron | 定时任务 CRUD、执行历史 |
| Providers | 多 Provider 管理、模型切换 |
| Skills | 技能发现与管理 |
| Bridge | 外部适配器管理 |
| System | 全局设置（语言、附件、日志、流式、限速）、配置查看 |

### Bridge 扩展协议

WebSocket 接入，声明式能力，自动降级。无需写 Go 代码即可接入新平台。

- 端点：`ws://<host>:9810/bridge/ws`
- 认证：`?token=` / `Authorization: Bearer` / `X-Bridge-Token`
- 能力：`text`（必需）、`card`、`buttons`、`typing`、`image`、`file`、`audio`、`update_message`、`preview`、`reconstruct_reply`、`delete_message`

### 默认端口

| 服务 | 端口 | 协议 |
|------|------|------|
| Bridge (WebSocket) | 9810 | WS + HTTP REST |
| Management API + Web UI | 9820 | HTTP |
| Vite Dev Server | 9821 | HTTP（仅开发） |
| Webhook | 9111 | HTTP |

---

## 3. 安装与快速开始

### 前置条件

| 依赖 | 最低版本 | 检查 |
|------|---------|------|
| Node.js + npm | 18+ | `node --version` |
| Git | 任意 | `git --version` |

### 配置 npm 作用域

编辑 `~/.npmrc`（Windows: `C:\Users\<用户名>\.npmrc`）：

```ini
@game1991:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=<YOUR_GITHUB_PAT>
```

PAT 获取：GitHub → Settings → Developer settings → Personal access tokens → Fine-grained → `read:packages`

### 安装

```bash
npm install -g @game1991/cc-connect
cc-connect --version
```

### 创建配置

```bash
cc-connect                  # 首次运行自动生成模板
# 或手动创建
mkdir -p ~/.cc-connect
cp config.example.toml ~/.cc-connect/config.toml
```

最小飞书配置：

```toml
language = "zh"

[log]
level = "info"

[[projects]]
name = "my-project"

[projects.agent]
type = "claudecode"

[projects.agent.options]
work_dir = "D:/WorkSpace/src/my-project"
mode = "default"

[[projects.platforms]]
type = "feishu"

[projects.platforms.options]
app_id = "cli_xxxxxxxxxxxx"
app_secret = "xxxxxxxxxxxxxxxxxxxxxxxx"
```

### 启动

```bash
# 前台调试
cc-connect

# 后台 daemon
cc-connect daemon install
cc-connect daemon status
cc-connect daemon logs -f
```

### 启用 Web 管理界面

```bash
cc-connect setup-web
```

或手动在 `config.toml` 添加：

```toml
[bridge]
enabled = true
port = 9810
token = "<YOUR_BRIDGE_SECRET>"

[management]
enabled = true
port = 9820
token = "<YOUR_MANAGEMENT_SECRET>"
```

访问 `http://localhost:9820`，使用 management token 登录。

---

## 4. 斜杠命令速查

所有命令在聊天窗口或 Web UI 中输入，以 `/` 开头。需管理员权限的命令标记为 **[admin]**，仅 `admin_from` 列表中的用户可执行。

### 会话管理

| 命令 | 别名 | 说明 |
|------|------|------|
| `/new [名称]` | | 创建新会话 |
| `/list` | `/sessions` | 列出当前项目的会话 |
| `/switch <id>` | | 切换到指定会话 |
| `/name <名称>` | `/rename` | 重命名当前会话 |
| `/current` | | 查看当前会话 |
| `/history [n]` | | 查看最近 n 条消息 |
| `/usage` | `/quota` | 查看账号/模型限额使用情况 |
| `/compress` | `/compact` | 压缩当前会话上下文 |
| `/delete <id>` | `/del`, `/rm` | 删除指定会话（支持 `/delete 1,2,3` 或 `/delete 3-7`） |
| `/search <关键词>` | `/find` | 搜索会话 |
| `/cancel` | | 取消当前正在进行的 Agent 调用 |

### 权限与模式

| 命令 | 说明 |
|------|------|
| `/mode` | 查看可用模式 |
| `/mode <模式名>` | 切换模式（如 `yolo`、`default`） |
| `/allow [工具名]` | 无参数列出已授权工具；带参数预授权指定工具 |
| `/quiet` | 切换输出模式：full → compact → quiet（循环） |

### Provider 与模型

| 命令 | 说明 |
|------|------|
| `/provider` | 列出 Provider 及交互式切换/添加 |
| `/model` | 列出可用模型，点击卡片切换 |
| `/reasoning [等级]` | 查看或切换推理强度（需 Agent 支持） |

### 工作目录与引用

| 命令 | 别名 | 说明 |
|------|------|------|
| `/dir [路径\|reset]` | `/cd`, `/chdir`, `/workdir` | 查看、切换或重置工作目录 |
| `/dir <序号>` | | 按历史序号切换 |
| `/dir -` | | 返回上一个目录 |
| `/show <引用>` | | 按引用查看文件、目录或代码片段 |
| `/bind` | | 附件回传绑定管理 |
| `/workspace` | `/ws` | 工作区绑定管理（多工作区模式） |

### 运维

| 命令 | 别名 | 说明 |
|------|------|------|
| `/shell` | `/sh`, `/exec`, `/run` | **[admin]** 打开交互式 Shell |
| `/show` | | **[admin]** 按引用查看文件/目录/代码片段 |
| `/dir` | `/cd` | **[admin]** 切换工作目录 |
| `/restart` | | **[admin]** 重启引擎 |
| `/web [setup\|status]` | | **[admin]** Web 管理界面控制 |
| `/diff` | | **[admin]** 对比差异 |
| `/cron` | | 定时任务管理（卡片交互式 CRUD） |
| `/stop` | | 停止当前执行 |
| `/whoami` | `/myid` | 显示当前用户平台 ID（用于角色配置） |
| `/version` | | 查看版本信息 |
| `/status` | | 查看连接状态 |
| `/lang` | | 切换界面语言 |
| `/config` | | 查看当前配置 |
| `/doctor` | | 运行诊断检查 |
| `/upgrade` | `/update` | 升级到最新版本 |
| `/commands` | `/command`, `/cmd` | 管理自定义命令 |
| `/alias` | | 管理命令别名 |
| `/skills` | `/skill` | 技能发现与管理 |
| `/memory` | | 管理引擎级记忆 |
| `/heartbeat` | `/hb` | 心跳/定时任务管理 |
| `/tts` | | 语音合成控制 |
| `/ps` | `/btw` | 向正在进行的 Agent 会话注入补充消息 |
| `/start` | | 显示欢迎消息 |
| `/help` | | 显示可用命令 |

---

## 5. 配置参考

### 配置文件路径

| 路径 | 说明 |
|------|------|
| `~/.cc-connect/config.toml` | 默认配置 |
| `~/.cc-connect/daemon.json` | daemon 元数据（0600） |
| `~/.cc-connect/logs/cc-connect.log` | 运行日志 |
| `~/.cc-connect/.config.toml.lock` | 实例锁（含 PID） |
| `~/.cc-connect/run/api.sock` | API Unix socket |
| `~/.cc-connect/cc-connect-daemon.ps1` | Windows launcher 脚本 |

三级 fallback：`--config` 参数 → `WorkDir/config.toml` → `~/.cc-connect/config.toml`

### 全局设置

```toml
language = "zh"             # en, zh, zh-TW, ja, es
data_dir = ""               # 默认 ~/.cc-connect
attachment_send = "off"     # "on" 或 "off"，Agent 生成的图片/文件是否自动回传

[log]
level = "info"              # debug, info, warn, error
```

### 消息展示设置

```toml
[display]
mode = "full"                       # full（默认）、compact、quiet
thinking_messages = true            # 是否显示 thinking 消息
tool_messages = true                # 是否显示工具调用消息
show_context_indicator = true       # 是否显示 [ctx: ~N%] 上下文指示
reply_footer = true                # 是否显示回复页脚
card_mode = "legacy"                # "legacy"（默认）或 "rich"（飞书 Card 2.0）
```

`mode` 说明：
- `full`：完整显示 thinking + 工具消息（默认）
- `compact`：隐藏 thinking/工具，每个文本段独立卡片
- `quiet`：隐藏 thinking/工具，所有文本追加到同一卡片

### 全局 Provider

```toml
[[providers]]
name = "deepseek"
base_url = "https://api.deepseek.com/v1"
api_key = "sk-xxx"
model = "deepseek-chat"

provider_presets_url = "https://cdn.example.com/presets.json"
```

Provider 可被项目级 `[projects.agent.options]` 中的环境变量覆盖。

### 项目配置

```toml
[[projects]]
name = "my-project"
work_dir = "D:/WorkSpace/src/my-project"
run_as_user = ""             # Linux/macOS 用户隔离
reset_on_idle_mins = 0       # 空闲后自动切换新会话（0=关闭）
max_turn_time_mins = 0       # 单次 Agent 执行的最长分钟数（0=不限）
admin_from = ""              # 特权用户 ID 列表（逗号分隔）

[projects.agent]
type = "claudecode"

[projects.agent.options]
# 通用选项
mode = "default"
model = ""
# Claude Code 专属
router_url = ""
api_key = ""
system_prompt = ""
# 环境变量（map[string]string 或 map[string]any）
env = { ANTHROPIC_BASE_URL = "https://..." }
```

### 平台配置

飞书：

```toml
[[projects.platforms]]
type = "feishu"

[projects.platforms.options]
app_id = "cli_xxx"
app_secret = "xxx"
require_mention = true       # 群聊中需要 @机器人
group_reply_all = false      # 群聊中回复全部
```

Telegram：

```toml
[[projects.platforms]]
type = "telegram"

[projects.platforms.options]
token = "123456:ABC-DEF"
```

Discord：

```toml
[[projects.platforms]]
type = "discord"

[projects.platforms.options]
token = "xxx"
application_id = "123456"
```

Slack：

```toml
[[projects.platforms]]
type = "slack"

[projects.platforms.options]
bot_token = "xoxb-xxx"
app_token = "xapp-xxx"
```

钉钉：

```toml
[[projects.platforms]]
type = "dingtalk"

[projects.platforms.options]
client_id = "xxx"
client_secret = "xxx"
```

企业微信：

```toml
[[projects.platforms]]
type = "wecom"

[projects.platforms.options]
corp_id = "xxx"
agent_id = "1000002"
secret = "xxx"
```

Bridge：

```toml
[bridge]
enabled = true
port = 9810
token = "<YOUR_BRIDGE_SECRET>"
allowed_origins = ["*"]
```

### 角色权限配置

详见 [第 6 节](#6-角色权限系统rbac)。

### 语音配置

```toml
[speech]
stt_endpoint = ""            # Whisper 兼容端点（空=禁用）
stt_model = "whisper-1"
stt_language = "zh"

tts_provider = ""            # minimaxi, alibaba, volcengine, fish, openai, edge-tts
tts_voice = ""
tts_model = ""
tts_mode = ""                # voice_only（仅语音）, always（语音+文字）
```

### 定时任务

```toml
[[crons]]
name = "daily-lint"
project = "my-project"
expression = "0 6 * * 1-5"  # robfig/cron 表达式
mode = "prompt"              # prompt（发消息给 Agent）或 exec（Shell 命令）
prompt = "帮我运行 lint 并总结"
session_mode = "new_per_run" # new_per_run 或 reuse
timeout_mins = 10
mute = false                  # 静默模式：不向 IM 发送输出
```

### 生命周期钩子

```toml
[[hooks]]
event = "session.started"    # message.received, message.sent, session.started,
                             # session.ended, cron.triggered, permission.requested, error
handler = "command"          # command（Shell）或 http（Webhook）
command = "echo 'Session started'"  # handler=command 时
url = ""                     # handler=http 时
```

### 管理 API

```toml
[management]
enabled = true
port = 9820
token = "<YOUR_MANAGEMENT_SECRET>"
```

### Webhook

```toml
[webhook]
enabled = true
port = 9111
token = "<YOUR_WEBHOOK_SECRET>"
```

---

## 6. 角色权限系统（RBAC）

> **安全提示**：未配置角色时，所有用户共享 agent 级权限策略，群聊中任何人回复"允许"即可批准工具请求。强烈建议为生产环境配置角色。

### 工作流程

```
用户发消息 → resolveUserRole() → 获取角色级 mode + allowed_tools
                                    ↓
                    ┌── yolo ──→ 全部自动通过
                    ├── default → allowed_tools 自动通过，其余弹窗确认
                    └── dontAsk → allowed_tools 自动通过，其余自动拒绝
                                    ↓
              权限请求回调 → 身份校验（仅发起者或 admin 可批准）
```

### 三级角色

| 角色 | 推荐模式 | 说明 |
|------|----------|------|
| admin | yolo | 完全控制，可批准任何人的权限请求 |
| developer | default | 可读写代码，工具需按白名单控制 |
| readonly | dontAsk | 只读，只允许安全工具自动通过 |

### 配置示例

```toml
[[projects]]
name = "my-project"

[projects.users]
default_role = "readonly"

[projects.users.roles.admin]
user_ids = ["ou_admin_xxx"]
mode = "yolo"
allowed_tools = []

[projects.users.roles.developer]
user_ids = ["ou_dev_aaa", "ou_dev_bbb"]
mode = "default"
allowed_tools = ["Read", "Grep", "Glob", "Bash", "Edit", "Write"]
disabled_commands = ["shell"]

[projects.users.roles.readonly]
user_ids = ["*"]               # * 匹配所有未分配角色的用户
mode = "dontAsk"
allowed_tools = ["Read", "Grep", "Glob", "LS", "LSP"]
disabled_commands = ["*"]
```

获取用户 ID：在聊天中发送 `/whoami`。

### 模式 × allowed_tools 交互

| | allowed_tools 为空 | allowed_tools 有值 | 工具不在白名单 |
|---|---|---|---|
| **yolo** | 全部自动通过 | 冗余 | 仍自动通过 |
| **default** | 全部弹窗确认 | 白名单自动通过 | 弹窗确认 |
| **dontAsk** | 全部自动拒绝 | 白名单自动通过 | 自动拒绝 |

### 审批身份校验

- 发起者本人可以批准/拒绝
- admin 角色可以批准/拒绝任何人的请求
- 其他人回复"允许"会被拒绝

### disabled_commands

控制哪些斜杠命令对该角色禁用。`"*"` 表示禁用所有命令。

### admin_from 与特权命令

在项目配置中设置 `admin_from = "alice,bob"` 后，只有这些用户 ID 能执行 `/shell`、`/show`、`/dir`、`/restart`、`/web`、`/diff` 等特权命令。配置语法详见 [第 10 节 安全机制](#10-安全机制)。

---

## 7. 高级功能

### 流式预览

Agent 执行过程中实时预览输出，按平台差异分两种策略：

- **卡片流式**（飞书、钉钉）：200ms/20 字符节流
- **降级流式**（其余平台）：1500ms/30 字符节流

可在 `[stream_preview]` 块下按平台禁用：`disabled_platforms = ["qq"]`

### 语音消息

**语音转文字 (STT)**：兼容 OpenAI Whisper API，支持 OpenAI、Groq、Qwen、Gemini 后端。需要 ffmpeg。

**文字转语音 (TTS)**：7 个 Provider — Qwen/DashScope、OpenAI、MiniMax、Mimo、espeak、pico、Edge TTS。需要 ffmpeg。

模式：`voice_only`（仅语音回复）、`always`（语音 + 文字）。

### 定时任务 (Cron)

基于 robfig/cron/v3，支持人类可读表达式。两种类型：

- `prompt`：发送文本消息给 Agent（如"帮我运行 lint"）
- `exec`：直接执行 Shell 命令

会话模式：`reuse`（复用现有会话）或 `new_per_run`（每次新建）。支持超时和静默模式。

可通过 `/cron` 卡片交互式添加（支持自然语言表达式）。

### 多机器人中继 (Relay)

多个 cc-connect 实例之间的 Bot 消息互通，由 RelayManager 协调。

可见性模式：
- `full`：转发完整对话
- `summary`：转发摘要
- `none`：隐藏

### 生命周期钩子 (Hooks)

支持 7 种事件：`message.received`、`message.sent`、`session.started`、`session.ended`、`cron.triggered`、`permission.requested`、`error`

两种 Handler：
- `command`：执行 Shell 命令
- `http`：发送 HTTP POST

异步执行，不阻塞主流程。

### Bridge WebSocket 协议

允许外部平台适配器通过 WebSocket 接入。适配器声明能力集，Engine 自动降级不支持的特性。

- 心跳：ping/pong 保活
- 重连：指数退避
- REST API：会话列表、发消息、创建/删除会话

### Web 管理界面

内置 React SPA（端口 9820），Bearer token 认证。

功能页：Dashboard、Projects（3 步向导）、Chat（40+ 斜杠命令、命令面板）、Sessions、Cron、Providers、Skills、Bridge、System（全局设置、i18n、主题切换）。

### 多工作区模式

通过 channel-to-workspace 绑定，将不同 IM 频道映射到不同的工作目录。绑定持久化到 `workspace_bindings.json`，支持约定式自动绑定和克隆初始化。

### 会话管理

- **弹性恢复**：路径标准化 + resume 失败自动降级为全新会话
- **自动压缩**：上下文超限时自动压缩，可在项目配置中调整阈值

```toml
[projects.auto_compress]
max_tokens = 0       # 触发压缩的预估 token 阈值（0=使用默认值）
min_gap_mins = 30    # 两次压缩之间的最短间隔（分钟）
```
- **空闲重置**：长时间无消息自动新建会话
- **最大执行时间**：单次 Agent 执行的绝对时间上限，含两阶段关停（优雅 → 强杀）
- **批量删除**：`/delete 1,2,3` 和 `/delete 3-7` 语法

### 附件回传

Agent 生成的图片/文件可自动回传到聊天。需要 `/bind setup` 初始化（飞书/钉钉），也可通过 `cc-connect send --image/--file` 手动发送。

---

## 8. 运维命令手册

### Daemon 安装模式

Windows 上 daemon 有两种安装模式，取决于安装时的权限：

| 权限 | 模式 | 管理 | 特点 |
|------|------|------|------|
| 管理员 | Windows Service | `services.msc` 或 `sc.exe` | 无弹窗，开机自启，最稳定 |
| 普通用户 | 任务计划（schtasks） | `schtasks.exe` | 可能有 PowerShell 弹窗 |

> **建议**：Windows 用户优先以管理员权限安装，选择 Windows Service 模式。

### Daemon 管理

```bash
cc-connect daemon install       # 安装并启动
cc-connect daemon status
cc-connect daemon logs -f       # 实时跟踪
cc-connect daemon logs -n 100   # 最近 100 行
cc-connect daemon stop
cc-connect daemon restart
cc-connect daemon restart --force   # 进程卡死时：杀进程 → 500ms → 重启
cc-connect daemon uninstall
```

日志自动轮转（10MB），保留一份备份。

### daemon install --force 的使用场景

`daemon install --force` 用于**已安装 daemon 时覆盖重装**。普通 `install` 检测到已注册的服务/任务会拒绝并提示加 `--force`。常见场景：

- **更换配置路径**：如从 `--config A` 改为 `--config B`
- **更换工作目录**：如从 `--work-dir /old` 改为 `--work-dir /new`
- **修复损坏的注册**：服务/任务注册信息异常时强制覆盖

> **注意**：正常版本升级**不需要** `--force`，使用 `daemon restart` 即可。`--force` 会先删除再重建服务/任务，产生短暂中断。

### 升级

**常规升级**（daemon 已运行，保留配置）：

```bash
cc-connect daemon stop
npm install -g @game1991/cc-connect
cc-connect --version            # 确认新版本
cc-connect daemon restart
```

> **重要**：必须先 `daemon stop` 再 `npm install`。`daemon stop` 会在 SCM 停止失败时自动强杀残留进程，确保二进制文件可被替换。直接 `npm install` 可能因旧进程锁定文件而失败，或导致新 daemon 启动时加载旧二进制。

**跨模式重装**（从 schtasks 切换到 Windows Service，或反之）：

```bash
cc-connect daemon stop          # 先停止，确保进程退出
cc-connect daemon uninstall     # 再卸载服务注册
# 以目标权限重新安装：
cc-connect daemon install       # 普通用户 → schtasks
# 或以管理员身份运行：
cc-connect daemon install       # 管理员 → Windows Service
```

> **注意**：`daemon uninstall` 已内置残留进程强杀机制，会自动清理 SCM 停止后的孤儿进程。旧版本需先手动 `daemon stop` 再 `uninstall`。

**彻底清理后重装**（配置损坏、二进制丢失等疑难场景）：

```bash
cc-connect daemon stop          # 确保进程完全退出
# Windows: 删除旧 service（需管理员终端）
sc.exe stop cc-connect
sc.exe delete cc-connect
# 然后重装
npm uninstall -g @game1991/cc-connect
npm install -g @game1991/cc-connect
cc-connect daemon install
```

> **注意**：必须使用 `npm install` 而非 `npm update`。此包发布在 GitHub Packages，`npm update` 对 scoped package 的 registry 路由行为不稳定。同时确保 `~/.npmrc` 中 `@game1991:registry=https://npm.pkg.github.com` 配置有效且 PAT 未过期。

### 清理与重装

```bash
cc-connect clean              # 清运行时，保留 config 和 crons
cc-connect clean reset        # 全部删除，用户数据先备份到 ~/.cc-connect-backup/<时间戳>/
cc-connect reinstall --yes    # 清理 + npm 重装 + 恢复配置 + daemon install
```

`clean` 实际删除项：实例锁、运行时目录（sock）、日志目录、daemon 元数据（daemon.json）、Windows 启动器脚本（cc-connect-daemon.ps1）、目录历史（dir_history.json）。保留 config.toml 和 crons/。

`clean reset` 在执行 `clean` 之前备份：config.toml、crons/jobs.json、dir_history.json。

`reinstall` 是两阶段命令：
- **Phase 1**（Go）：备份（config + crons/jobs.json + dir_history.json + daemon-meta.json）→ 清理 → 卸载 daemon → 生成补全脚本
- **Phase 2**（补全脚本）：npm uninstall → npm install → 恢复配置 → daemon install（使用备份的 daemon-meta.json 恢复原始 install 参数）

MINGW bash 中一条命令完成；PowerShell/CMD 中因文件锁需两步执行。

### 诊断

```bash
cc-connect doctor            # 检查 agent 二进制可用性、认证、用户隔离预检
```

### 版本查询

```bash
cc-connect --version         # cc-connect v1.3.3
                            # commit:  2eeaac4
                            # built:   2026-06-11T...
```

### 自更新

```bash
cc-connect update            # 从 GitHub/Gitee 获取最新稳定版
cc-connect update --pre      # 获取最新预发布版
```

双源策略：GitHub 为主，Gitee 为备（面向中国用户）。可在配置中设置 `prefer_gitee = true` 切换优先级。

---

## 9. 故障排查

### daemon stop 停不掉

`daemon stop` 已含 PID 直杀回退。如果仍不生效：

```bash
cc-connect daemon restart --force   # 杀进程 → 重启
ps -W | grep -i cc-connect          # 手动查 PID
taskkill /PID <PID> /F              # 强制终止
```

### 配置文件找不到

三级 fallback 自动查找：`--config` → `WorkDir/config.toml` → `~/.cc-connect/config.toml`。也可显式传 `--config <路径>`。

### npm 安装失败

1. 检查 PAT 是否有 `read:packages` 权限
2. 检查 `~/.npmrc` 格式（注意 `@game1991:` 前缀冒号不能漏）
3. 检查网络是否可访问 `npm.pkg.github.com`

### 飞书连接断开

飞书使用长连接，断线后会自动重连。检查 `app_id`/`app_secret` 是否正确，以及网络是否可访问飞书 API。

### 二进制被覆盖

如果 `cc-connect --version` 报告的版本与预期不符，可能是 npm 的 `run.js` 自动更新覆盖了本地二进制。这与版本号排序有关 — 详见[开发者指南 §4 版本管理](developer-guide.zh-CN.md#4-版本管理)中的 fork.N 陷阱详解。
### Windows 特殊问题

- **`.old` 备份**：自更新时当前二进制重命名为 `.old`
- **无 pty**：iFlow Agent 在 Windows 上跳过

---

## 10. 安全机制

| 机制 | 说明 | 配置位置 |
|------|------|----------|
| 实例锁 | PID 文件锁，防止重复进程，`--force` 可杀旧进程 | 自动 |
| run-as-user | Linux/macOS 下以指定 OS 用户运行 Agent 进程 | `run_as_user = "coder"` |
| allow_from | 限制特定 IP 或用户 ID 可发送消息 | 平台 `options` 中 `allow_from` |
| admin_from | 限制可执行特权命令（`/shell`、`/show`、`/dir`、`/restart`、`/web`、`/diff`）的用户 | `admin_from = "alice,bob"` |
| 请求去重 | TTL 去重，防止重复消息触发重复执行 | 自动 |
| 敏感信息脱敏 | 环境变量值、API Key 参数在日志中自动遮蔽 | 自动 |
| 路径穿越保护 | `SaveFilesToDisk` 清理 `..` 和绝对路径前缀 | 自动 |
| 原子写入 | 临时文件 + sync + rename，防止崩溃导致文件损坏 | 自动 |

---

## 11. Fork 版本说明

### 当前版本：v1.3.3-fork

本仓库是 [cc-connect 官方版](https://github.com/chenhg5/cc-connect) 的 fork 版本，基于官方 `v1.3.3` tag 衍生。fork 的目的是在官方版本基础上解决 Windows 生产环境中的实际痛点，这些修复尚未被上游合并。

**发布渠道**：`@game1991/cc-connect@beta`（GitHub Packages）

### 快速导航

- [下载与安装](#下载与安装) — 从零开始安装 fork 版本
- [从官方版切换](#从官方版切换到-fork-版本) — 卸载旧版 + 安装新版
- [就地升级](#就地升级fork-版本内升级) — fork 版本之间的升级
- [降级回官方版](#降级回官方版本)
- [彻底清理重装](#彻底清理后重装)
- [兼容性说明](#兼容性说明)
- [Fork 变更内容](#fork-相对于官方-v133-的变更)
- [版本选择建议](#版本选择建议)

---

### 下载与安装

#### 前置条件

| 依赖 | 要求 | 检查命令 |
|------|------|----------|
| Node.js + npm | 18+ | `node --version` |
| Git | 任意 | `git --version` |
| GitHub PAT | `read:packages` 权限 | — |

#### 第一步：配置 npm 作用域

fork 版本发布在 GitHub Packages，需要配置 npm 作用域才能下载。编辑 `~/.npmrc`（Windows: `C:\Users\<用户名>\.npmrc`）：

```ini
@game1991:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=<YOUR_GITHUB_PAT>
```

PAT 获取方式：GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens → 勾选 `read:packages`

> 如果已有官方版的 `.npmrc` 配置，两行新增内容直接追加即可，不影响其他 scope。

#### 第二步：安装

```bash
npm install -g @game1991/cc-connect@beta
```

> `@beta` 是 npm dist-tag，始终指向最新的 fork 预发布版本。无需记忆具体版本号。

#### 第三步：验证

```bash
cc-connect --version
# 预期输出: cc-connect v1.3.3-beta  或  v1.3.3-beta.N
```

如果提示 `command not found`，检查 npm 全局 bin 目录是否在 PATH 中：

```bash
npm prefix -g          # 查看 npm 全局目录
npm bin -g             # 查看 bin 目录（npm < 9）
ls "$(npm prefix -g)/bin/cc-connect"   # 确认二进制存在
```

#### 第四步：初始化配置

```bash
cc-connect                  # 首次运行自动生成 ~/.cc-connect/config.toml 模板
```

或手动创建：

```bash
mkdir -p ~/.cc-connect
cp config.example.toml ~/.cc-connect/config.toml
```

#### 第五步：安装 daemon

```bash
# 管理员终端 → Windows Service（推荐，无弹窗、开机自启）
cc-connect daemon install

# 普通终端 → schtasks（兼容模式，可能有 PowerShell 弹窗）
cc-connect daemon install
```

安装时自动检测权限：
- **管理员**：注册为 Windows Service，通过 `services.msc` 管理，无需登录即可运行
- **普通用户**：注册为任务计划，需用户登录后才会启动，会提示建议用管理员重装

#### 第六步：确认运行

```bash
cc-connect daemon status
cc-connect daemon logs -f
```

---

### 从官方版切换到 Fork 版本

如果你当前运行的是官方版（`@anthropic-ai/cc-connect` 或 npm 上的 `cc-connect`），切换步骤如下：

#### 场景 A：官方版以 daemon 方式运行

```bash
# 1. 停止旧 daemon
cc-connect daemon stop

# 2. 卸载旧版
npm uninstall -g cc-connect
# 或（如果通过 scoped 包安装的官方版）：
npm uninstall -g @anthropic-ai/cc-connect

# 3. 安装 fork 版本（确保 ~/.npmrc 已配置 @game1991 scope）
npm install -g @game1991/cc-connect@beta

# 4. 验证
cc-connect --version

# 5. 安装 daemon（管理员终端推荐）
cc-connect daemon install

# 6. 确认
cc-connect daemon status
```

#### 场景 B：官方版以前台方式运行

```bash
# 1. 停止运行中的 cc-connect（Ctrl+C）

# 2. 卸载旧版
npm uninstall -g cc-connect

# 3. 安装 fork 版本
npm install -g @game1991/cc-connect@beta

# 4. 验证 + 启动
cc-connect --version
cc-connect
```

#### 配置文件兼容吗？

**完全兼容**。fork 版本与官方 v1.3.3 使用相同的 `config.toml` 格式，无需任何修改。现有的 `~/.cc-connect/config.toml`、`crons/jobs.json`、会话数据均可直接沿用。

---

### 就地升级（Fork 版本内升级）

如果你已经在使用 fork 版本，获取最新修复和新功能：

#### 方式一：npm 覆盖安装（推荐）

```bash
# 1. 停止 daemon
cc-connect daemon stop

# 2. 覆盖安装到最新 beta
npm install -g @game1991/cc-connect@beta

# 3. 确认版本已更新
cc-connect --version

# 4. 重启 daemon
cc-connect daemon restart
```

> **为什么要先 stop？** daemon 进程会持有二进制文件的文件句柄。Windows 上文件被打开时无法被覆盖。`daemon stop` 会在 SCM 停止失败时自动强杀残留进程，确保旧文件可被替换。Linux/macOS 同理（虽无文件锁问题，但旧进程继续运行会使用旧二进制）。

#### 方式二：自更新命令

cc-connect 内置了自更新能力，支持从聊天窗口直接升级：

```
/upgrade --pre
```

- `--pre`：获取预发布版本（即 fork beta）
- 不加 `--pre`：获取最新稳定版（会跳过 fork 版本）

更新后 fork 版本会自动重新生成 daemon 启动脚本并重启 daemon（`postUpdateDaemonRestart`）。

双源策略：默认从 GitHub Releases 下载，中国大陆用户可在 config.toml 中设置 `prefer_gitee = true` 切换到 Gitee 镜像源。

#### 方式三：一条命令重装（适用于疑难场景）

```bash
cc-connect reinstall --yes
```

此命令自动执行：备份配置 → 停止 daemon → 清理运行时 → 卸载 npm → 重新安装 npm → 恢复配置 → 重装 daemon。详见[彻底清理后重装](#彻底清理后重装)。

---

### 降级回官方版本

如果试用后决定回退：

```bash
# 1. 停止 fork daemon
cc-connect daemon stop

# 2. 卸载 daemon 注册
cc-connect daemon uninstall

# 3. 卸载 fork 版本
npm uninstall -g @game1991/cc-connect

# 4. 安装官方稳定版
npm install -g cc-connect@latest

# 5. 验证
cc-connect --version          # 应无 fork / beta 标识

# 6. 重装 daemon
cc-connect daemon install
```

> 配置文件无需任何改动，官方版可直接读取 `~/.cc-connect/config.toml`。

---

### 彻底清理后重装

适用于以下场景：
- daemon 重复安装导致 Service 注册混乱
- 进程残留杀不掉，二进制文件被锁定
- 运行时状态损坏（实例锁残留、日志异常膨胀）
- npm 安装反复失败，需要从头来

#### 方式一：`reinstall` 命令（推荐）

```bash
cc-connect reinstall --yes
```

自动两阶段流程：
1. **Go 阶段**（当前进程）：备份 config.toml + crons + dir_history + daemon.json → 停止 daemon → 卸载 daemon → 清理运行时 → 生成补全脚本
2. **补全脚本**（bash/PowerShell）：`npm uninstall` → `npm install` → 恢复配置 → `daemon install`（使用备份的 daemon.json 恢复原始安装参数）

MINGW bash 中一条命令完成；PowerShell/CMD 因文件锁需分两步执行（脚本会打印提示）。

备份文件自动保存到 `~/.cc-connect-backup/<时间戳>/`，包含 `config.toml`、`crons-jobs.json`、`dir_history.json`、`daemon-meta.json`。

#### 方式二：手动逐步清理

```bash
# 1. 停止 daemon
cc-connect daemon stop

# 2. Windows: 手动删除残留 Service（需管理员终端）
#    仅当 daemon uninstall 报错或 services.msc 中仍存在 cc-connect 时执行
sc.exe stop cc-connect
sc.exe delete cc-connect

# 3. 卸载 npm 包
npm uninstall -g @game1991/cc-connect

# 4. 清理运行时数据
#    保留 config.toml 和 crons/，删除其他所有运行时文件
cc-connect clean
#    或完全重置（自动备份后删除全部，包括 config.toml 和 crons/）
cc-connect clean reset

# 5. 重新安装
npm install -g @game1991/cc-connect@beta

# 6. 验证
cc-connect --version
cc-connect doctor

# 7. 重装 daemon
cc-connect daemon install
```

> `cc-connect clean` 实际删除项：实例锁（`.config.toml.lock`）、运行时目录（`run/sock`）、日志（`logs/`）、daemon 元数据（`daemon.json`）、Windows 启动脚本（`cc-connect-daemon.ps1`）、目录历史（`dir_history.json`）。**保留** `config.toml` 和 `crons/`。

---

### 兼容性说明

| 项目 | 官方 v1.3.3 | Fork 版本 | 说明 |
|------|-------------|-----------|------|
| config.toml 格式 | ✅ | ✅ | 完全兼容，无需修改 |
| crons/jobs.json | ✅ | ✅ | 完全兼容 |
| 会话数据 | ✅ | ✅ | 共享 `~/.cc-connect/`，可双向切换 |
| npm scope | `cc-connect` 或 `@anthropic-ai/cc-connect` | `@game1991/cc-connect` | **不同 scope**，需手动切换 |
| daemon 平台 | systemd / launchd / schtasks | systemd / launchd / schtasks / **svc** | fork 新增 Windows Service 模式 |
| CLI 命令 | ✅ | ✅ | 完全一致，`cc-connect` 入口不变 |
| `--version` 输出 | `v1.3.3` | `v1.3.3-beta` 或 `v1.3.3-beta.N` | 含 beta 标识可区分 |
| Web UI | ✅ | ✅ | 内嵌二进制，无需额外部署 |

**关键结论**：
- **配置文件 100% 兼容**，两个版本可无缝切换，无需改任何一行配置
- **数据目录共享**（`~/.cc-connect/`），会话、定时任务等全部保留
- **唯一的区别是 npm scope**：安装/卸载时注意包名不同
- **就地升级安全**：覆盖安装即可，无需卸载再装

---

### Fork 相对于官方 v1.3.3 的变更

共 20 个 commits，按功能分组：

#### 核心新增：Windows Service 原生支持

官方版 Windows daemon 仅支持 schtasks（任务计划），存在 PowerShell 弹窗、登录才启动等问题。本 fork 新增 Windows Service 模式：

| 特性 | 官方版 | Fork 版 |
|------|--------|---------|
| 安装模式 | 仅 schtasks | **管理员→Windows Service / 普通用户→schtasks** |
| 开机自启 | 需用户登录 | **Service 模式无需登录** |
| 弹窗问题 | 可能出现 PowerShell 窗口 | **Service 模式无弹窗** |
| 管理 | `schtasks.exe` | **`services.msc` / `sc.exe`** |
| 环境变量 | SYSTEM 账户无用户 PATH | **安装时捕获用户 PATH 和 HOME，Service 启动时恢复** |
| SCM 生命周期 | 不支持 | **支持 Stop/Shutdown 信号，25s 优雅关停** |

实现文件：
- `daemon/svcmanager_windows.go`：sc.exe 管理 Service（安装/启动/停止/卸载/状态查询），含 `IsAdmin()` 权限检测
- `cmd/cc-connect/svc_run_windows.go`：Service 入口，对接 SCM，恢复用户环境变量，优雅关停
- `cmd/cc-connect/update_daemon.go`：自更新后自动重启 daemon + 重写启动脚本

#### 核心新增：插件技能发现

- 自动扫描 `~/.cc-connect/plugins/cache/<plugin>/skills/` 和 `<plugin>/.claude/skills/`
- 符号链接安全检查：跳过指向缓存目录外部的符号链接
- `filepath.WalkDir` 返回值检查修复

#### Daemon 稳健性增强（6 项修复）

| 问题 | 修复 |
|------|------|
| `daemon stop` 后残留实例锁 | stop / uninstall / restart --force 后自动清理 `.config.toml.lock`，新实例不再被误挡 |
| npm 更新后 daemon 加载旧二进制 | 启动脚本改为 PATH 解析：先检查 `BinaryPath`，找不到则 `Get-Command cc-connect` fallback |
| daemon 更新后启动脚本过期 | `cc-connect update` 和 `daemon restart --force` 自动调用 `RewriteLauncherScript()` |
| uninstall 后孤儿进程占文件 | uninstall 在 SCM 停止后自动强杀残留进程（通过实例锁 PID） |
| `cc-connect clean` 顺序不当 | clean 先快照 `daemon.json` → 用快照杀进程 → 最后才 uninstall（避免 meta 过早丢失） |
| Windows Service 启动错误不可见 | Service 模式下启动错误写入 `slog.Error` 日志，不再静默退出 |

#### 发布流程：`make rebeta`

```bash
make rebeta                                  # 默认重推 v1.3.3-beta
make rebeta REBETA_VERSION=v1.3.3-beta.5     # 指定版本
make rebeta DRY_RUN=1                       # 仅预览，不执行
```

流程：推送 main → 删除远端 tag → 删除 npm 旧版本 → 重新打 tag → 推送 tag → 等 CI → 验证 npm 版本号。

---

### 版本选择建议

| 场景 | 推荐版本 |
|------|----------|
| Linux/macOS 生产环境 | **官方稳定版** — fork 改动主要针对 Windows |
| Windows 服务器/虚拟机 | **Fork 版本** — Windows Service 无弹窗、开机自启 |
| Windows 开发机（每天手动启动） | **官方稳定版** — schtasks 够用 |
| 使用 Claude Code 插件技能 | **Fork 版本** — 自动发现插件技能目录 |
| daemon 经常卡死/残留进程 | **Fork 版本** — 实例锁/孤儿进程清理机制 |
| 仅基础功能、运行正常 | **官方稳定版** — 无需切换 |
