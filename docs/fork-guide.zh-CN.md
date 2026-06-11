# cc-connect

> AI 编码 Agent × 即时通讯，一句话写代码 | 最后更新 2026-06-11

---

## cc-connect 是什么

cc-connect 是 AI 编码 Agent 与即时通讯平台的桥接工具。用户在飞书、Telegram、Slack 等聊天窗口中即可操控 Claude Code、Codex、Gemini CLI 等 Agent，完成代码编写、文件读写、命令执行等任务。

**核心价值**：把 Agent 从终端搬到聊天窗口——不需要 SSH、不需要 IDE，在手机上也能让 AI 改代码。

---

## 支持的 Agent 与平台

### Agent

| Agent | 交互方式 | 说明 |
|-------|----------|------|
| Claude Code | 进程流式 | 主力支持 |
| Codex | 进程流式 | OpenAI |
| Gemini CLI | 进程流式 | Google |
| Cursor | tmux 屏幕抓取 | 需 tmux 环境 |
| ACP | 协议层 | Agent Control Protocol |
| iFlow / OpenCode / Qoder | 进程流式 | 社区 Agent |

### IM 平台

| 平台 | 连接方式 | 特色能力 |
|------|----------|----------|
| 飞书/Lark | 长连接事件 | 交互式卡片、按钮回调 |
| Telegram | Long Polling | Markdown 渲染 |
| Discord | Gateway WS | 按钮交互 |
| Slack | Socket Mode | Block Kit |
| 钉钉 | HTTP 回调 | AI 卡片 |
| 企业微信 / QQ / LINE | 各自协议 | 基础文本 + 图片 |

### Web 管理界面

内置 React SPA，编译后嵌入 Go 二进制，零额外部署。

| 页面 | 功能 |
|------|------|
| Dashboard | 版本、运行时间、连接状态 |
| Projects | 项目 CRUD、Agent/平台配置 |
| Chat | 实时聊天、会话管理 |
| Cron | 定时任务管理 |
| Providers | AI Provider 切换 |
| Skills | 技能发现 |
| Bridge | 外部适配器管理 |
| System | 全局设置 |

### Bridge 扩展协议

WebSocket 接入，声明式能力，自动降级。无需写 Go 代码即可接入新平台。

- 端点：`ws://<host>:9810/bridge/ws`
- 认证：`?token=` / `Authorization: Bearer` / `X-Bridge-Token`
- 能力：`text`（必需）、`card`、`buttons`、`typing`、`image`、`file`

---

## 角色权限系统

> **安全提示**：未配置角色时，所有用户共享 agent 级权限策略，群聊中任何人回复"允许"即可批准工具请求。强烈建议为生产环境配置 admin/developer/readonly 三级角色。

不同用户拥有独立的权限策略，避免"实习生批准 admin 的 `Bash rm`"。

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

### 配置示例

```toml
[[projects]]
name = "my-project"

[projects.agent]
type = "claudecode"

[projects.agent.options]
work_dir = "D:/WorkSpace/src/my-project"
mode = "default"               # agent 级默认值

[projects.users]
default_role = "readonly"      # 未匹配角色的用户默认 readonly

[projects.users.roles.admin]
user_ids = ["ou_admin_xxx"]    # 发送 /whoami 获取平台用户 ID
mode = "yolo"
allowed_tools = []
disabled_commands = []

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

---

## 安装与部署

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
cc-connect daemon stop
cc-connect daemon restart
cc-connect daemon restart --force   # 杀进程 → 500ms → 重启
cc-connect daemon uninstall
```

### Web 管理界面

```bash
cc-connect setup-web          # 一键启用 Bridge + Management
```

或手动配置：

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

### 默认端口

| 服务 | 端口 | 协议 |
|------|------|------|
| Bridge (WebSocket) | 9810 | WS + HTTP REST |
| Management API + Web UI | 9820 | HTTP |
| Vite Dev Server | 9821 | HTTP（仅开发） |

---

## 运维命令

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

### 日志

```bash
cc-connect daemon logs           # 末尾日志
cc-connect daemon logs -f        # 实时跟踪
cc-connect daemon logs -n 100    # 最近 100 行
```

日志自动轮转（10MB），保留一份备份。

### 故障排查

**daemon stop 停不掉**：

```bash
cc-connect daemon restart --force   # 杀进程 → 500ms → 重启
ps -W | grep -i cc-connect          # 手动查 PID
taskkill /PID <PID> /F              # 强制终止
```

**daemon install 找不到配置文件**：三级 fallback 自动查找，也可显式 `--config`。

---

## 配置文件路径

| 路径 | 说明 |
|------|------|
| `~/.cc-connect/config.toml` | 默认配置 |
| `~/.cc-connect/daemon.json` | daemon 元数据（0600） |
| `~/.cc-connect/logs/cc-connect.log` | 运行日志 |
| `~/.cc-connect/.config.toml.lock` | 实例锁（含 PID） |
| `~/.cc-connect/run/api.sock` | API Unix socket |
| `~/.cc-connect/cc-connect-daemon.ps1` | Windows launcher 脚本 |

---

## 开发者指南

### 前置条件

| 依赖 | 最低版本 | 用途 |
|------|---------|------|
| Go | 1.23+ | 后端 |
| Node.js + npm | 18+ | 前端构建 |
| pnpm | 9+ | 前端锁定（可选） |

### 编译

```bash
make build              # 全量（含 Web UI）
make build-noweb        # 无 Web UI（-tags no_web）
```

### 前端开发

```bash
cd web && npm install && npm run dev    # 端口 9821
```

### 选择性编译

```bash
make build EXCLUDE=discord,dingtalk,qq,qqbot,line,weibo
make build AGENTS=claudecode PLATFORMS_INCLUDE=feishu,telegram
```

Agent 标签：`no_acp no_claudecode no_codex no_cursor no_gemini no_iflow no_opencode no_qoder`
Platform 标签：`no_feishu no_telegram no_discord no_slack no_dingtalk no_wecom no_weixin no_qq no_qqbot no_line no_weibo`

### 测试

```bash
make test-fast              # 单元 + 冒烟
make test-full              # 完整套件
make test-smoke             # 仅冒烟
make test-release-local     # 本地发布测试
```

### 交叉编译

```bash
make release TARGET=linux/amd64
make release-noweb TARGET=windows/amd64
make release-all
```

### 从上游同步

```bash
git remote add upstream https://github.com/chenhg5/cc-connect.git
git fetch upstream && git merge upstream/main
make build && go test ./...
```

合并后检查：
- `npm/install.js` 中 `GITHUB_REPO` / `GITEE_REPO` 是否被上游重置
- `daemon/manager.go` 中 Meta 结构体是否有新增字段
- 版本号排序是否仍满足 `fork.N > beta.N`

---

## 版本与发布

### 版本号规则

格式：`v<major>.<minor>.<patch>-fork.<N>`

- `fork.N` 独立于上游 `-beta.N` 递增
- 必须满足 `fork.N` ≥ 上游 npm beta 的排序值（否则 `run.js` 会覆盖本地二进制）

### 发版流程

```bash
# 1. 编辑 npm/package.json 中的 version 字段
# 2. 提交
git add npm/package.json && git commit -m "chore: bump version"
# 3. 打 tag（触发 CI）—— 最终定版后执行
git tag vX.Y.Z-fork.N
# 4. 推送
git push origin <branch> --tags
```

CI 自动执行：5 平台编译 → 打包 + SHA256 → GitHub Release → npm.pkg.github.com
