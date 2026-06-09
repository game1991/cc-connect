# cc-connect Fork 说明书

> `@game1991/cc-connect` v1.3.3-fork.11 | 最后更新 2026-06-09

---

## 一、为什么 Fork

上游 cc-connect 是优秀的 AI agent ↔ IM 桥接工具，但在实际部署中暴露了两个系统性缺陷：

### 1. 权限模型粗糙

| 问题 | 影响 |
|------|------|
| `mode` / `allowed_tools` 是 agent 级配置，所有用户共享同一权限 | admin 和普通用户看到的弹窗确认行为完全一样 |
| `handlePendingPermission` 不校验回复者身份 | 任何人在群聊回复"允许"就能批准别人的工具请求 |
| Bridge Web 管理界面默认身份 `"web-admin"` | 暗示拥有管理员权限，违反最小权限原则 |
| 配置文件无角色权限示例 | 用户不知道如何配置差异化权限 |

**典型场景**：团队飞书群里，实习生回复"允许"就批准了 admin 发起的 `Bash rm` 请求。

### 2. Windows daemon 不可靠

| 问题 | 影响 |
|------|------|
| `daemon install` 只查 `$HOME/config.toml`，不进 `.cc-connect` 子目录 | Windows 上几乎必然报 "config not found"，需手动 `--config` 绕过 |
| 三平台 launcher 不传 `--config` | daemon 启动后找不到配置文件（CWD 不在配置目录） |
| `daemon stop` 仅依赖 `schtasks /End` | 手动启动的进程或卡死进程无法停止 |
| `KillExistingInstance` 不等待进程退出 | 旧进程还占着端口，新进程启动失败 |
| `KillExistingInstance` 不验证 PID 归属 | PID 被其他进程复用后可能误杀无关程序 |
| `.cmd` 文件关联注册表值被 Windows 更新清空 | daemon 启动时弹出"你想如何打开此文件？"对话框 |
| schtasks 未设 `-Hidden` | 每次登录闪一下 CMD 窗口 |
| AtLogOn 触发无延迟 | Explorer Shell 文件关联未就绪就启动 daemon |

**典型场景**：Windows 11 累积更新后，`.cmd` 文件关联被重置，重启时弹出对话框，daemon 未能启动。

---

## 二、Fork 改造清单

### F 系列：角色权限系统

| # | 改造 | 影响文件 | 说明 |
|---|------|----------|------|
| F-1 | 权限审批身份校验：仅发起者或 admin 可批准 | `core/engine.go` | `handlePendingPermission` 增加 `RequestingUserID` 校验 |
| F-2 | 角色级 `mode` + `allowed_tools` | `config/config.go`, `core/user_roles.go`, `core/engine.go` | 不同角色拥有独立的权限策略 |
| F-3 | Bridge 卡片身份参数化 | `core/bridge.go` | `dispatchAsMessage` 传递真实 userID，移除共享可变状态 |
| F-4 | 默认身份改为 `bridge-user` | `core/bridge.go` | 不暗示管理员权限 |
| F-5 | 配置示例补全 | `config.example.toml` | admin/developer/readonly 三种角色示例 |

### B 系列：daemon 配置路径修复

| # | 改造 | 影响文件 | 说明 |
|---|------|----------|------|
| B-1 | 三级 fallback 配置查找 | `cmd/cc-connect/daemon.go` | `--config` → `WorkDir/config.toml` → `~/.cc-connect/config.toml` |
| B-2 | Meta 新增 `ConfigFile` 字段 | `daemon/manager.go` | 保留完整配置文件路径 |
| B-3 | 三平台 launcher 传 `--config` | `daemon/windows.go`, `systemd.go`, `launchd.go` | 守护进程 CWD 不在配置目录，必须显式传递 |

### D 系列：daemon 安全加固

| # | 改造 | 影响文件 | 说明 |
|---|------|----------|------|
| D-1 | `daemon stop` 回退直杀 + 诚实报错 | `cmd/cc-connect/daemon.go` | `stopWithFallback` 依赖注入，失败 `os.Exit(1)` |
| D-2 | Unix `KillExistingInstance` 等待进程退出 | `cmd/cc-connect/instance_lock.go` | 5s 轮询，超时返回 false |
| D-3 | 进程映像名验证防误杀 | `instance_lock_windows.go`, `instance_lock.go` | Win: `QueryFullProcessImageNameW`; Unix: `/proc/<pid>/exe` |
| D-4 | `daemon.json` 权限 0600 | `daemon/manager.go` | 含路径信息，不应全局可读 |
| D-5 | 路径统一正斜杠 | `cmd/cc-connect/daemon.go` | `filepath.ToSlash` 确保跨平台可移植 |
| D-6 | `daemon restart --force` 路径修复 + 竞态消除 | `cmd/cc-connect/daemon.go` | `metaConfigPath()` + 500ms sleep |
| D-7 | `KillExistingInstance` 超时返回 false | 同 D-2/D-3 | 统一语义：调用方需正确判断 |
| D-8 | 权限拒绝 i18n 五语翻译 | `core/engine.go`, `core/i18n.go` | EN/ZH/ZH-TW/JA/ES |
| D-9 | `clean reset` 备份顺序修复 | `cmd/cc-connect/clean.go` | 先备份再清理，防止 `dir_history.json` 被删后无法备份 |
| D-10 | `reinstall` 集成为 Go 子命令 | `cmd/cc-connect/reinstall.go` | 消除旧脚本自举悖论，统一 bash/ps1 双脚本模板 |

### W 系列：Windows 专项修复

| # | 改造 | 影响文件 | 说明 |
|---|------|----------|------|
| W-1 | `OpenProcess` 加 `SYNCHRONIZE` 权限 | `instance_lock_windows.go` | `WaitForSingleObject` 不再返回 `WAIT_FAILED` |
| W-2 | ~~`.cmd` 文件关联修复~~ 已删除 | `daemon/windows.go` | 旧版 `ensureCmdFileAssociation` 创建空 `OpenWithList` key，反成弹框根因，v1.3.3-fork.11 彻底删除 |
| W-3 | schtasks 设 `-Hidden` | `daemon/windows.go` | `New-ScheduledTaskSettingsSet -Hidden` 防止 CMD 闪现 |
| W-4 | AtLogOn 触发延迟 30s | `daemon/windows.go` | `$trigger.Delay = 'PT30S'` 等 Shell 初始化完成 |

### G 系列：跨平台通用修复

| # | 改造 | 影响文件 | 说明 |
|---|------|----------|------|
| G-1 | `normalizeWorkspacePath` 统一正斜杠 | `core/workspace_state.go` | workspace key 跨平台一致 |
| G-2 | `replyFooterHomeRelativePath` 正斜杠比较 | `core/engine.go` | 修复 `~` 缩写 Windows 上不生效 |
| G-3 | `AllowedTools` 注释精确化 | `config/config.go` | 描述三种模式交互而非仅 dontAsk |
| G-4 | `doctor_runas_test.go` 补 `!windows` build tag | `cmd/cc-connect/` | 修复 Windows 编译失败 |

---

## 三、核心功能总览

cc-connect 是 AI 编码 Agent 与即时通讯平台的桥接工具。用户在飞书、Telegram、Slack 等聊天窗口中即可操控 Claude Code、Codex、Gemini CLI 等 Agent，完成代码编写、文件读写、命令执行等任务。

### Agent ↔ IM 桥接

| Agent | 类型 | Platform | 类型 |
|-------|------|----------|------|
| Claude Code | 进程流式 | 飞书/Lark | 长连接事件 |
| Codex | 进程流式 | Telegram | Long Polling |
| Cursor | tmux 屏幕抓取 | Discord | Gateway WS |
| Gemini CLI | 进程流式 | Slack | Socket Mode |
| ACP | 协议层 | 钉钉 | HTTP 回调 |
| iFlow / OpenCode / Qoder | 进程流式 | 企业微信 / QQ / LINE | 各自协议 |

### Web 管理界面

内置 React SPA 前端，编译后通过 `//go:embed` 嵌入 Go 二进制，无需额外部署。

| 页面 | 功能 |
|------|------|
| Dashboard | 系统概览：版本、运行时间、已连接平台/适配器 |
| Projects | 项目 CRUD、Agent 配置、平台绑定 |
| Chat | 实时聊天：消息发送、会话管理、命令面板 |
| Cron | 定时任务管理：创建/执行/暂停/恢复 |
| Providers | 全局 AI Provider 切换 + 远程预设 |
| Skills | 技能发现与市场 |
| Bridge | 外部适配器状态与连接管理 |
| System | 全局设置：语言、日志级别、流式预览、超时 |

启用方式（在 `config.toml` 中添加）：

```toml
[bridge]
enabled = true
port = 9810
token = "a-strong-random-secret"

[management]
enabled = true
port = 9820
token = "mgmt-secret"
```

或通过 CLI 一键启用：

```bash
cc-connect setup-web
```

启动后访问 `http://localhost:9820`，使用 management token 登录。

### Bridge 协议

Bridge 允许外部适配器通过 WebSocket 连接到 cc-connect，无需编写 Go 代码即可接入新平台。

- WebSocket 端点：`ws://<host>:9810/bridge/ws`
- 认证方式：查询参数 `?token=` / Header `Authorization: Bearer` / Header `X-Bridge-Token`
- 能力声明：适配器注册时声明支持 `text`（必需）、`card`、`buttons`、`typing`、`image`、`file` 等
- 自动降级：适配器缺少某能力时，自动回退到纯文本

### 默认端口分配

| 服务 | 端口 | 协议 |
|------|------|------|
| Bridge (WebSocket) | 9810 | WS + HTTP REST |
| Management API + Web UI | 9820 | HTTP |
| Vite Dev Server | 9821 | HTTP（仅开发） |

---

## 四、核心功能：角色权限系统

### 设计思路

```
用户发消息 → resolveUserRole() → 获取角色级 mode + allowed_tools
                                    ↓
                    ┌── yolo ──→ 全部自动通过
                    ├── default → allowed_tools 自动通过，其余弹窗确认
                    └── dontAsk → allowed_tools 自动通过，其余自动拒绝
                                    ↓
              权限请求回调 → 身份校验（仅发起者或 admin 可批准）
```

### 配置方式

```toml
[[projects]]
name = "my-project"

[projects.agent]
type = "claudecode"

[projects.agent.options]
work_dir = "D:/WorkSpace/src/my-project"
mode = "default"               # agent 级默认值

[projects.users]
default_role = "readonly"      # 未匹配任何角色的用户默认角色

[projects.users.roles.admin]
user_ids = ["ou_admin_xxx"]    # 发送 /whoami 获取你的平台用户 ID
mode = "yolo"                  # 所有工具自动通过
allowed_tools = []             # yolo 下冗余
disabled_commands = []

[projects.users.roles.developer]
user_ids = ["ou_dev_aaa", "ou_dev_bbb"]
mode = "default"               # allowed_tools 自动通过，其余弹窗确认
allowed_tools = ["Read", "Grep", "Glob", "Bash", "Edit", "Write"]
disabled_commands = ["shell"]   # 只禁用危险命令

[projects.users.roles.readonly]
user_ids = ["*"]                # * 匹配所有未分配角色的用户
mode = "dontAsk"                # allowed_tools 自动通过，其余自动拒绝
allowed_tools = ["Read", "Grep", "Glob", "LS", "LSP"]
disabled_commands = ["*"]       # 禁用所有内置命令
```

### 模式 × allowed_tools 交互速查

| | allowed_tools 为空 | allowed_tools 有值 | 工具不在白名单 |
|---|---|---|---|
| **yolo** | 全部自动通过 | 冗余，等同空 | 仍自动通过 |
| **default** | 全部弹窗确认 | 白名单自动通过 | 弹窗确认 |
| **dontAsk** | 全部自动拒绝 | 白名单自动通过 | 自动拒绝 |

### 权限审批身份校验

当 agent 请求工具权限时：
- **发起者本人**可以批准/拒绝
- **admin 角色**可以批准/拒绝任何人的请求
- **其他人**回复"允许"会被拒绝，并收到提示："只有请求者或管理员可以批准此操作"

---

## 五、安装与部署

### 安装方式

唯一推荐路径：**GitHub Packages npm 安装**。

> ⚠️ 手动 `cp` 编译产物替换 `npm install` 安装的二进制 **不可行**：
> `run.js` 每次执行时会对比版本，不匹配则从 GitHub Release 重新下载覆盖；
> Windows 上运行中的 daemon 会锁定 `.exe` 文件导致 `cp` 失败。

### 前置条件

| 依赖 | 最低版本 | 检查命令 |
|------|---------|---------|
| Node.js + npm | 18+ | `node --version` |
| Git | 任意 | `git --version` |

### 第一步：配置 npm 作用域

编辑 `~/.npmrc`（Windows: `C:\Users\<用户名>\.npmrc`）：

```ini
@game1991:registry=https://npm.pkg.github.com
//npm.pkg.github.com/:_authToken=<YOUR_GITHUB_PAT>
```

获取 PAT：GitHub → Settings → Developer settings → Personal access tokens → Fine-grained tokens → 勾选 `read:packages`。

### 第二步：安装

```bash
npm install -g @game1991/cc-connect
```

验证：

```bash
cc-connect --version
# 预期: cc-connect v1.3.3-fork.11
```

### 第三步：创建配置

```bash
# 运行一次自动生成模板
cc-connect

# 或手动创建
mkdir -p ~/.cc-connect
cp config.example.toml ~/.cc-connect/config.toml
```

最小配置（飞书示例）：

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

### 第四步：启动

**前台调试：**

```bash
cc-connect
```

**后台 daemon（推荐）：**

```bash
# 安装并启动（自动定位 ~/.cc-connect/config.toml，无需 --config）
cc-connect daemon install

# 管理
cc-connect daemon status
cc-connect daemon logs -f
cc-connect daemon stop
cc-connect daemon restart
cc-connect daemon restart --force  # 杀进程 → 500ms → 重启
cc-connect daemon uninstall
```

### 清理与重装

**运行时清理**（停止 daemon、删除 lock/sock/log，保留 config 和 crons）：

```bash
cc-connect clean
```

**彻底重置**（删除所有用户数据，config/crons 先备份到 `~/.cc-connect-backup/<时间戳>/`）：

```bash
cc-connect clean reset
```

**一键重装**（清理 + npm 重装 + 恢复配置 + daemon install）：

```bash
cc-connect reinstall --yes
```

`cc-connect reinstall` 是两阶段命令：
- **Phase 1**（Go 代码）：备份配置 → 清理运行时 → 卸载 daemon → 生成补全脚本
- **Phase 2**（补全脚本）：npm uninstall → npm install → 恢复配置 → daemon install

在 MINGW/Git Bash 环境中，Phase 1 完成后自动 exec 进入 Phase 2 bash 脚本，实现一条命令完成。
在纯 Windows 环境（PowerShell/CMD）中因文件锁限制需两步：先 `cc-connect reinstall`，再运行打印的 PowerShell 补全脚本命令。

> **注意**：备份在清理操作之前创建（v1.3.3-fork.11+），确保 `dir_history.json` 等文件不会被清理步骤删除后再备份。脚本中的 `BACKUP_DIR` 和 `CC_DIR` 路径由 Go 代码计算后直接嵌入，不依赖 `$HOME` 或 `$env:USERPROFILE` 重建，避免 MINGW 环境下 `$HOME` 指向 MSYS2 家目录而非 Windows `%USERPROFILE%` 的路径不一致问题。

---

## 六、开发者编译指南

### 前置条件

| 依赖 | 最低版本 | 用途 |
|------|---------|------|
| Go | 1.23+ | 后端编译 |
| Node.js + npm/pnpm | 18+ | Web 前端构建 |
| pnpm | 9+ | CI/前端锁定（可选） |

### 全量编译（含 Web UI）

```bash
make build
```

等价于：先 `cd web && npm install && npm run build` 生成 `web/dist/`，再 `go build` 嵌入前端资源。产物包含完整 Web 管理界面。

### 无 Web 编译

```bash
make build-noweb
```

跳过前端构建，使用 `-tags 'no_web'` 编译。二进制更小，不提供 Web 界面。

### 前端独立开发

```bash
cd web
npm install
npm run dev        # Vite 开发服务器，端口 9821
```

Vite 自动将 `/api` 代理到 `localhost:9820`（Management API），`/bridge` 代理到 `localhost:9810`（Bridge WebSocket）。

### 选择性编译

排除不需要的 Agent 或 Platform 以减小二进制体积：

```bash
make build EXCLUDE=discord,dingtalk,qq,qqbot,line,weibo
make build AGENTS=claudecode PLATFORMS_INCLUDE=feishu,telegram
```

可用标签：`no_acp no_claudecode no_codex no_cursor no_gemini no_iflow no_opencode no_qoder`（Agent）和 `no_feishu no_telegram no_discord no_slack no_dingtalk no_wecom no_weixin no_qq no_qqbot no_line no_weibo`（Platform）。

### 测试

```bash
make test-fast       # 单元 + 冒烟测试（< 2 分钟）
make test-full       # 完整测试套件（PR 要求）
make test-smoke      # 仅冒烟测试
make test-release-local  # 本地发布测试（不需要真实凭证）
```

### 交叉编译发布

```bash
make release TARGET=linux/amd64           # 单平台带 Web
make release-noweb TARGET=windows/amd64   # 单平台无 Web
make release-all                          # 全平台带 Web
```

---

## 七、升级

```bash
cc-connect daemon stop
npm update -g @game1991/cc-connect
cc-connect --version        # 确认新版本
cc-connect daemon restart
```

---

## 八、故障排查

### daemon install 后弹出 "你想如何打开此文件？"（可能弹两次）

**根因**：旧版 cc-connect (v1.3.3-fork.7 ~ fork.10) 的 `ensureCmdFileAssociation()` 在 `daemon install` 时创建了 `HKCU\...\FileExts\.cmd\OpenWithList` 空 key。在干净 Windows 系统上此键**不应存在**，Windows 会回退到系统默认关联 `HKCR\.cmd → cmdfile → cmd.exe`。但空 key 存在时，Windows 认为"用户已配置此类型但清除了所有程序"，不走系统默认 → 弹框。系统内置 `Monitoring` 计划任务（热补丁监控）在登录时和每日触发 `.cmd` 文件，短时间多次触发 → **弹两次**。

**已修复**：v1.3.3-fork.11+ 已彻底删除 `ensureCmdFileAssociation`，cc-connect 不再触碰注册表。如果之前版本已创建空 key，需手动清理：

```cmd
reg delete "HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.cmd" /f
```

然后注销重新登录，让 Windows 恢复系统默认关联。

### daemon stop 停不掉

**优先级递增方案：**

```bash
# 1. 正常停止（schtasks /End）
cc-connect daemon stop

# 2. 强制重启（杀进程 → 500ms → 重启）
cc-connect daemon restart --force

# 3. 手动杀进程（需管理员）
ps -W | grep -i cc-connect | grep -v grep
taskkill /PID <PID> /F
```

Fork v1.3.3-fork.4+ 的 `daemon stop` 已含 PID 直杀回退：`schtasks /End` 失败后自动读取实例锁 PID → 验证进程映像名 → `TerminateProcess`。

### daemon install 找不到配置文件

Fork 修复了三级 fallback 查找逻辑：

1. 显式 `--config /path/to/config.toml` → 使用指定路径
2. `WorkDir/config.toml` 存在 → 使用它
3. `~/.cc-connect/config.toml` 存在 → 使用它，WorkDir 自动调整为 `~/.cc-connect`

如仍需指定，显式传 `--config` 即可。

### 日志查看

```bash
cc-connect daemon logs          # 末尾日志
cc-connect daemon logs -f       # 实时跟踪
cc-connect daemon logs -n 100   # 最近 100 行
```

日志自动轮转（默认 10MB），保留一份备份。

---

## 九、配置文件路径速查

| 路径 | 说明 |
|------|------|
| `~/.cc-connect/config.toml` | 默认配置文件 |
| `~/.cc-connect/daemon.json` | daemon 元数据（0600 权限，路径统一正斜杠） |
| `~/.cc-connect/logs/cc-connect.log` | 运行日志 |
| `~/.cc-connect/.config.toml.lock` | 实例锁（含 PID） |
| `~/.cc-connect/run/api.sock` | API Unix socket |
| `~/.cc-connect/cc-connect-daemon.ps1` | Windows daemon launcher 脚本 |

---

## 十、发布流程（面向维护者）

### 版本号规则

格式：`v<major>.<minor>.<patch>-fork.<N>`

- `fork.N` 独立于上游 `-beta.N` 递增
- **必须满足** `fork.N` ≥ 上游 npm beta 版的 pre-release 排序值
  - 当前上游 npm：`1.3.3-beta.4`
  - 当前 fork：`1.3.3-fork.11`
  - semver 排序：`fork.8 > beta.4` ✅（否则 `run.js` 会覆盖你的二进制）

### 发版命令

```bash
# 1. 编辑 npm/package.json 中的 version 字段

# 2. 提交
git add npm/package.json
git commit -m "chore: bump version to vX.Y.Z-fork.N"

# 3. 打 tag（触发 CI）
git tag vX.Y.Z-fork.N

# 4. 推送
git push origin feat/role-based-tool-permissions --tags
```

CI（`fork-release.yml`）自动执行：
- 5 平台交叉编译（linux-amd64/arm64, darwin-amd64/arm64, windows-amd64）
- 打包 `.tar.gz` / `.zip` + SHA256 校验
- 创建 GitHub Release
- 发布 npm 包到 `npm.pkg.github.com`

### 从上游同步

```bash
git remote add upstream https://github.com/chenhg5/cc-connect.git  # 首次
git fetch upstream
git merge upstream/main
# 解决冲突后：
make build && ./cc-connect --version
go test ./...
```

合并后检查：
- `npm/install.js` 中 `GITHUB_REPO` / `GITEE_REPO` 是否被上游重置
- `daemon/manager.go` 中 Meta 结构体是否有新增字段
- 版本号是否需要更新以保持 `fork.N > beta.N`
