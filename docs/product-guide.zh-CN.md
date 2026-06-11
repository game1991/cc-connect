# cc-connect 产品指南

> AI 编码 Agent × 即时通讯，一句话写代码 | v1.3.3 | 最后更新 2026-06-11

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

### IM 平台（13 个 + Bridge）

| 平台 | 连接方式 | 特色能力 | 需公网 IP |
|------|----------|----------|-----------|
| 飞书/Lark | 长连接事件 | 交互式卡片、按钮回调、QR 设置 | 否 |
| Telegram | Long Polling | HTML Markdown 渲染 | 否 |
| Discord | Gateway WS | Embed、按钮交互 | 否 |
| Slack | Socket Mode | Block Kit、斜杠命令、typing emoji | 否 |
| 钉钉 | HTTP 回调 | AI 流式卡片（降级为文本） | 是 |
| 企业微信 | HTTP 回调 | 文本 + 图片 | 是 |
| 微信个人号 | ilink 协议 | QR 码登录 | 否 |
| QQ | 协议特定 | 文本 + 图片 | 视配置 |
| QQ Bot | 协议特定 | 文本 + 图片 | 视配置 |
| LINE | 协议特定 | 文本 + 图片 | 是 |
| 微博 | 协议特定 | 文本 + 图片 | 是 |
| MAX | Long-poll/Webhook | 三种部署拓扑 | 视配置 |
| WPS 协作 | WebSocket | AES-256-CBC + HMAC-SHA256 | 否 |
| Bridge | WebSocket | 通用外部适配器协议 | 否 |

### Web 管理界面

内置 React SPA，编译后嵌入 Go 二进制，零额外部署。

| 页面 | 功能 |
|------|------|
| Dashboard | 版本、运行时间、连接状态 |
| Projects | 项目 CRUD、3 步添加向导 |
| Chat | 实时聊天、会话管理、25 个斜杠命令 |
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

所有命令在聊天窗口或 Web UI 中输入，以 `/` 开头。

### 会话管理

| 命令 | 说明 |
|------|------|
| `/new [名称]` | 创建新会话 |
| `/list` | 列出当前项目的会话 |
| `/switch <id>` | 切换到指定会话 |
| `/current` | 查看当前会话 |
| `/history [n]` | 查看最近 n 条消息 |
| `/usage` | 查看账号/模型限额使用情况 |
| `/compact` | 压缩当前会话上下文 |
| `/reset` | 重置为全新会话 |
| `/delete <id>` | 删除指定会话（支持 `/delete 1,2,3` 或 `/delete 3-7`） |
| `/close` | 关闭当前会话 |

### 权限与模式

| 命令 | 说明 |
|------|------|
| `/mode` | 查看可用模式 |
| `/mode yolo` | 切换到 YOLO 模式（自动批准所有工具） |
| `/mode default` | 切换到默认模式（每次工具调用确认） |
| `/allow <工具名>` | 预授权工具 |

### Provider 与模型

| 命令 | 说明 |
|------|------|
| `/provider list` | 列出 Provider |
| `/provider switch <名称>` | 切换 API Provider |
| `/model` | 列出可用模型 |
| `/model switch <别名>` | 按别名切换模型 |
| `/reasoning [等级]` | 查看或切换推理强度（Codex） |

### 工作目录与引用

| 命令 | 说明 |
|------|------|
| `/dir [路径\|reset]` | 查看、切换或重置工作目录 |
| `/dir <序号>` | 按历史序号切换 |
| `/dir -` | 返回上一个目录 |
| `/cd <路径>` | `/dir` 的兼容别名 |
| `/show <引用>` | 按引用查看文件、目录或代码片段 |
| `/bind setup` | 初始化附件回传（飞书/钉钉） |

### 运维

| 命令 | 说明 |
|------|------|
| `/shell` | 打开交互式 Shell |
| `/cron add <表达式> <指令>` | 添加定时任务（自然语言） |
| `/stop` | 停止当前执行 |
| `/whoami` | 显示当前用户平台 ID（用于角色配置） |
| `/version` | 查看版本信息 |
| `/status` | 查看连接状态 |
| `/lang` | 切换界面语言 |
| `/config` | 查看当前配置 |
| `/doctor` | 运行诊断检查 |
| `/upgrade` | 升级到最新版本 |
| `/help` | 显示可用命令 |

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

在项目配置中设置 `admin_from = "alice,bob"` 后，只有这些用户 ID 能执行 `/shell`、`/show`、`/dir`、`/restart`、`/upgrade`、`/web`、`/diff` 等特权命令。配置语法详见 [第 10 节 安全机制](#10-安全机制)。

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

可通过 `/cron add <表达式> <指令>` 自然语言添加。

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

功能页：Dashboard、Projects（3 步向导）、Chat（25 个斜杠命令、命令面板）、Sessions、Cron、Providers、Skills、Bridge、System（全局设置、i18n、主题切换）。

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

### Daemon 管理

```bash
cc-connect daemon install       # 安装并启动
cc-connect daemon status
cc-connect daemon logs -f       # 实时跟踪
cc-connect daemon logs -n 100   # 最近 100 行
cc-connect daemon stop
cc-connect daemon restart
cc-connect daemon restart --force   # 杀进程 → 500ms → 重启
cc-connect daemon uninstall
```

日志自动轮转（10MB），保留一份备份。

### 清理与重装

```bash
cc-connect clean              # 清运行时（lock/sock/log），保留 config 和 crons
cc-connect clean reset        # 全部删除，config/crons 先备份到 ~/.cc-connect-backup/<时间戳>/
cc-connect reinstall --yes    # 清理 + npm 重装 + 恢复配置 + daemon install
```

`reinstall` 是两阶段命令：
- **Phase 1**（Go）：备份 → 清理 → 卸载 daemon → 生成补全脚本
- **Phase 2**（补全脚本）：npm uninstall → npm install → 恢复配置 → daemon install

MINGW bash 中一条命令完成；PowerShell/CMD 中因文件锁需两步执行。

### 升级

```bash
cc-connect daemon stop
npm update -g @game1991/cc-connect
cc-connect --version
cc-connect daemon restart
```

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

- **schtasks daemon**：Windows 使用任务计划程序而非 systemd
- **`.old` 备份**：自更新时当前二进制重命名为 `.old`
- **无 pty**：iFlow Agent 在 Windows 上跳过
- **`.cmd` 弹框**：已移除导致弹框的 `ensureCmdFileAssociation` 函数

---

## 10. 安全机制

| 机制 | 说明 | 配置位置 |
|------|------|----------|
| 实例锁 | PID 文件锁，防止重复进程，`--force` 可杀旧进程 | 自动 |
| run-as-user | Linux/macOS 下以指定 OS 用户运行 Agent 进程 | `run_as_user = "coder"` |
| allow_from | 限制特定 IP 或用户 ID 可发送消息 | 平台 `options` 中 `allow_from` |
| admin_from | 限制可执行特权命令（`/shell`、`/show`、`/dir`、`/restart`、`/upgrade`、`/web`、`/diff`）的用户 | `admin_from = "alice,bob"` |
| 请求去重 | TTL 去重，防止重复消息触发重复执行 | 自动 |
| 敏感信息脱敏 | 环境变量值、API Key 参数在日志中自动遮蔽 | 自动 |
| 路径穿越保护 | `SaveFilesToDisk` 清理 `..` 和绝对路径前缀 | 自动 |
| 原子写入 | 临时文件 + sync + rename，防止崩溃导致文件损坏 | 自动 |
