# cc-connect Windows 踩坑 QA

> 基于 cc-connect v1.3.2 → v1.3.3-beta.4，Windows 11 + MSYS2/bash 环境

---

## Q1: `daemon install` 报 "config.toml not found in $HOME"

**现象**
```
Warning: config.toml not found in $HOME
  Use --work-dir to specify the config directory or --config to point to the config file
```
明明 `~/.cc-connect/config.toml` 存在，`cc-connect config path` 也能正确解析。

**根因**

`daemon install` 子命令的配置查找逻辑与主程序不同：
- 主程序查找顺序：`./config.toml` → `~/.cc-connect/config.toml` ✅
- `daemon install` 只查找：`$HOME/config.toml`（直接在 HOME 下，不进子目录）❌

这是一个 bug，`daemon` 子命令没有复用主程序的配置解析逻辑。

**解决**

临时方案 — 显式指定 `--config` 参数：
```bash
cc-connect daemon install --config "$HOME\\.cc-connect\\config.toml"
```

> 注意：`--work-dir` 也可以，但需要指向 `.cc-connect` 目录而非 HOME 目录。

**已在 fork 中修复** — 分支 `feat/role-based-tool-permissions`，提交 `a4558147`：

1. `cmd/cc-connect/daemon.go`：增加与主程序一致的三级 fallback 配置解析：
   - 显式 `--config` → 使用指定路径
   - `WorkDir/config.toml` 存在 → 使用它
   - `~/.cc-connect/config.toml` 存在 → 使用它，并自动将 WorkDir 调整为 `~/.cc-connect`
2. `daemon/manager.go`：Config 新增 `ConfigFile` 字段，由 Install 传入 launcher 脚本
3. 三平台 launcher 均传递 `--config`：
   - `daemon/windows.go`：PowerShell 脚本加 `--config` 参数
   - `daemon/systemd.go`：ExecStart 加 `--config` 参数
   - `daemon/launchd.go`：ProgramArguments 加 `--config` 参数

修复后 `daemon install` 无需 `--config` 即可自动定位 `~/.cc-connect/config.toml`。

---

## Q2: 配置文件迁移到其他盘后，用 Junction 链接回来行不行？

**结论：可行**

```bash
# 1. 移动实际数据
mv ~/.cc-connect/config.toml /other-drive/path/.cc-connect/config.toml

# 2. 删除空的源目录
rmdir ~/.cc-connect

# 3. 创建 Junction（无需管理员权限）
cmd //c mklink //J "$HOME\\.cc-connect" "/other-drive/path/.cc-connect"
```

**注意事项**
- Windows Junction（`mklink /J`）不需要管理员权限，符号链接（`mklink /D`）需要
- Go 二进制和 Node.js 都能正常通过 Junction 读取文件
- `cc-connect config path` 正确返回 `$HOME/.cc-connect/config.toml`

---

## Q3: 平台 type "wps-xiezuo" 报 "unknown platform"

**现象**
```
failed to create platform type=wps-xiezuo error="unknown platform \"wps-xiezuo\", available: [dingtalk discord lark qq qqbot telegram wecom weixin feishu line weibo slack]"
```

**根因**

v1.3.2 稳定版尚未包含 wps-xiezuo 平台支持。该平台在 GitHub 主分支/beta 版中已支持。

**解决**

升级到 beta 版：
```bash
npm install -g cc-connect@beta
# 当前 beta 版为 v1.3.3-beta.4
```

---

## Q4: 环境变量 ${WPS_XIEZUO_APP_SECRET} 报 "references unset variable"

**现象**
```
WARN config: env var placeholder references unset variable var=WPS_XIEZUO_APP_SECRET
```
明明已在 `~/.bashrc` 中 `export` 了该变量。

**根因**

MSYS2/bash 环境的特殊性：
- **交互式登录 shell** 才会 source `.bashrc`
- Claude Code 的 Bash 工具每次执行新命令在**独立子 shell** 中，不一定加载 `.bashrc`
- Go 二进制（cc-connect.exe）继承的是**调用进程的环境变量**，不是 bash 的

**解决**

`daemon install` 加 `--config` 参数后，cc-connect 会自动在 `~/.cc-connect` 下生成一个 PowerShell launcher 脚本，该脚本会正确加载环境变量。Task Scheduler 任务通过该 launcher 启动，环境变量可正常传递。

手动启动时，确保先 source：
```bash
source ~/.bashrc && cc-connect
```

---

## Q5: Windows 上 `daemon install` 到底能不能用？

**结论：能，但需要绕过配置查找 bug**

v1.3.2 报 `daemon management is not supported on windows`，但这是**错误信息误导**——实际上是配置找不到导致的连锁错误。

v1.3.3-beta.4 配合 `--config` 参数可以正常安装，使用 Windows Task Scheduler（schtasks）实现开机自启：

```
cc-connect daemon installed and started.

  Platform:  schtasks
  Binary:    <path-to-cc-connect.exe>
  WorkDir:   $HOME/.cc-connect
  Log:       $HOME/.cc-connect\logs\cc-connect.log
  LogMax:    10 MB
```

---

## Q6: MSYS2 bash 中执行 `mklink` 的正确姿势

**问题**

MSYS2 bash 使用 Unix 风格路径，`mklink` 是 Windows 原生命令，路径需要 Windows 风格（反斜杠）。

**解决**

```bash
# 正确：用 //c 调用 cmd，路径用双反斜杠转义
cmd //c mklink //J "$HOME\\.cc-connect" "/other-drive/path/.cc-connect"

# 错误：单反斜杠会被 bash 转义吞掉
cmd //c mklink //J "$HOME/.cc-connect" "/other-drive/path/.cc-connect"
```

**备忘**
- `cmd //c` — MSYS2 调用 Windows cmd 的标准写法（双斜杠转义）
- `mklink //J` — Junction 类型（`/J` 在 MSYS2 中写成 `//J`）
- 路径中的 `\` — 在 bash 双引号中需要 `\\` 转义

---

## 快速参考

### 安装与启动

```bash
# 安装最新 beta 版（含 wps-xiezuo 支持）
npm install -g cc-connect@beta

# 直接前台运行（测试用）
source ~/.bashrc && cc-connect

# 安装为后台服务（开机自启）
cc-connect daemon install --config "$HOME\\.cc-connect\\config.toml"

# 管理命令
cc-connect daemon status          # 查看状态
cc-connect daemon logs -f        # 实时日志
cc-connect daemon stop           # 停止
cc-connect daemon restart        # 重启
cc-connect daemon uninstall      # 卸载服务
```

### 配置文件路径

| 场景 | 路径 |
|------|------|
| cc-connect 默认查找 | `~/.cc-connect/config.toml` |
| daemon install 查找（原 bug） | `$HOME/config.toml`（不进子目录） |
| daemon install 查找（fork 已修复） | `WorkDir/config.toml` → `~/.cc-connect/config.toml` 三级 fallback |
| 绕过方案（原版仍需） | `--config "$HOME\\.cc-connect\\config.toml"` |
| 日志目录 | `~/.cc-connect/logs/` |
| 运行时锁文件 | `~/.cc-connect/.config.toml.lock` |
| API socket | `~/.cc-connect/run/api.sock` |

---

## Fork 变更清单（feat/role-based-tool-permissions）

> 基于上游 `main` 分支，共 2 个提交 + 工作区通用性改进

### 提交 1：`42f343a` feat: role-based agent permission mode and tool whitelist

| # | 问题 | 修复 | 影响文件 | 通用性说明 |
|---|------|------|----------|-----------|
| F-1 | 权限审批无身份校验：任何用户回复"允许"即可批准工具请求 | `handlePendingPermission` 增加 `RequestingUserID` 校验，仅发起者或 admin 可批准 | `core/engine.go` | 防止低权限用户越权审批，所有多用户部署必需 |
| F-2 | `mode`/`allowed_tools` 为 agent 级别配置，所有角色共享同一权限 | 角色级 `mode` + `allowed_tools`：admin 用 yolo，member 用 dontAsk + 白名单 | `config/config.go`, `core/user_roles.go`, `core/engine.go` | 三种模式全覆盖：yolo（全通）、dontAsk（白名单外自动拒绝）、default（白名单外弹窗确认） |
| F-3 | Bridge 卡片操作硬编码 `"web-admin"` 身份 | `dispatchAsMessage` 改为参数传递 `userID`/`userName` | `core/bridge.go` | 消除身份与权限的语义脱钩 |
| F-4 | 配置无角色权限示例，用户无从下手 | `config.example.toml` 新增 admin/member/developer 三种角色示例 | `config.example.toml` | 覆盖全部三种模式，工具名可替换 |

### 提交 2：`a455814` fix: daemon install respects ~/.cc-connect/config.toml fallback path

| # | 问题 | 修复 | 影响文件 | 通用性说明 |
|---|------|------|----------|-----------|
| B-1 | `daemon install` 只查 `$HOME/config.toml`，不进 `.cc-connect` 子目录 | 三级 fallback：`--config` → `WorkDir/config.toml` → `~/.cc-connect/config.toml`，与 `resolveConfigPath` 一致 | `cmd/cc-connect/daemon.go` | 所有平台受益，Windows 尤其明显 |
| B-2 | `--config` 参数只提取 `WorkDir`，丢失完整路径 | Config 新增 `ConfigFile` 字段，保留完整配置路径 | `daemon/manager.go` | launcher 需要完整路径才能正确传递 |
| B-3 | 三平台 launcher 不传 `--config`，守护进程启动时可能找不到配置 | Windows PowerShell / systemd / launchd 均在启动命令中加 `--config` | `daemon/windows.go`, `daemon/systemd.go`, `daemon/launchd.go` | 守护进程的 CWD 可能不在配置目录，必须显式传递 |

### 工作区改进（未提交，通用性提升）

| # | 原问题 | 改进 | 影响文件 | 通用性说明 |
|---|--------|------|----------|-----------|
| G-1 | `bridgeCardAction` 用共享可变状态 `lastCardAction` 传递身份，并发竞态 | 移除 `lastCardActionMu`/`lastCardAction` 字段，改为 `dispatchAsMessage` 参数显式传递 | `core/bridge.go` | 消除并发 bug，两个用户同时操作不再互相覆盖身份 |
| G-2 | 默认身份 `"web-admin"` 暗示管理员权限 | 默认身份改为 `"bridge-user"`，不暗示任何权限 | `core/bridge.go` | 最小权限原则：未配置身份时不应自动获得语义上的 admin 身份 |
| G-3 | `AllowedTools` 注释称"only effective with dontAsk"，误导用户 | 注释改为精确描述三种模式交互：yolo 冗余、default 预授权、dontAsk 仅白名单通过 | `config/config.go`, `config.example.toml` | 注释是用户理解行为的主要入口，必须与代码一致 |
| G-4 | 示例中 `user_ids` 含个人 ID（`"your_admin_user_id"`） | 替换为 `"your_admin_user_id"` 等通用占位符 | QA 文档 Q7 | 开源项目示例不应含个人配置 |

### 各模式与 allowed_tools 交互速查

| mode | allowed_tools 为空 | allowed_tools 有值 | 不在白名单的工具 |
|------|-------------------|-------------------|----------------|
| `yolo` | 全部自动通过（默认行为） | 冗余，效果等同于空 | 无影响，仍自动通过 |
| `default` | 全部弹窗确认 | 白名单内自动通过 | 弹窗确认 |
| `dontAsk` | 全部自动拒绝 | 白名单内自动通过 | 自动拒绝（不弹窗） |

---

## Q7: 如何让 admin 拥有完整权限，其他用户只有只读权限？

**背景**

cc-connect 默认的 `mode`/`allowed_tools`/`disallowed_tools` 是 agent 级别配置，所有角色共享。源码分析发现：
- 权限审批（`handlePendingPermission`）不校验回复者身份，任何用户回复"允许"都能批准
- `admin_from` 只控制特权斜杠命令（`/shell` 等），与工具权限审批无关

**解决方案（自建 fork）**

我们 fork 了 cc-connect 并实现了三个核心改动：

1. **角色级 `mode` 和 `allowed_tools`** — 在 `[projects.users.roles]` 中新增：
   ```toml
   [projects.users.roles.admin]
   user_ids = ["your_admin_user_id"]   # 替换为你的平台用户 ID
   mode = "yolo"                       # 所有工具自动通过
   allowed_tools = []                   # yolo 模式下冗余，无需限制
   disabled_commands = []

   [projects.users.roles.readonly]
   user_ids = ["*"]                     # 匹配所有未分配的用户
   mode = "dontAsk"                     # 未预授权的工具自动拒绝
   allowed_tools = ["Read", "Grep", "Glob", "LS", "LSP"]  # 只读工具白名单（替换为你 agent 的工具名）
   disabled_commands = ["mode", "dir", "cd", "memory", "provider", "cron", "shell", "bind", "switch", "model"]
   ```

2. **权限审批身份校验** — `handlePendingPermission` 新增 `RequestingUserID` 校验：
   - 只有发起权限请求的用户或 admin 才能批准/拒绝
   - 其他人回复"允许"会被拒绝并提示

3. **Bridge 卡片身份修复** — `dispatchAsMessage` 通过参数传递真实 userID，而非共享可变状态；默认身份改为中性的 `bridge-user`（不再暗示管理员权限）

**修改的文件**

| 文件 | 改动 |
|------|------|
| `config/config.go` | RoleConfig 新增 Mode、AllowedTools 字段 |
| `core/user_roles.go` | RoleInput/UserRole 新增 Mode、AllowedTools |
| `core/engine.go` | pendingPermission 记录 RequestingUserID、handlePendingPermission 身份校验、新建 session 时注入角色级 mode 和 allowed_tools |
| `core/bridge.go` | bridgeCardAction 新增 UserID/UserName、dispatchAsMessage 传递真实身份 |
| `config.example.toml` | 新增 mode/allowed_tools 配置示例 |

分支 `feat/role-based-tool-permissions`

---

## Q8: Fork 版本编译 + 替换 npm 安装 + 重装 Daemon 完全流程

**适用场景**：你 fork 了 cc-connect 官方仓库，修改了代码，需要用自编译的二进制替换 `npm install -g cc-connect@beta` 安装的版本，并重新安装 Windows daemon 服务。

### 前置条件

| 依赖 | 最低版本 | 检查命令 |
|------|---------|---------|
| Go | 1.25+ | `go version` |
| Git | 任意 | `git --version` |
| MSYS2/bash | 任意 | `$BASH_VERSION` |
| Node.js + npm | 18+ | `node --version` |

### 第一步：编译 Fork

```bash
# 进入 fork 仓库目录
cd /d/WorkSpace/src/cc-connect

# 确认在正确的分支
git branch --show-current
# 预期输出: feat/role-based-tool-permissions

# 快速编译（跳过 Web 前端，推荐）
make build-noweb

# 或完整编译（需要 pnpm 环境）
make build
```

编译产物：项目根目录下的 `cc-connect`（MINGW 下无 `.exe` 后缀，但实际是 Windows PE 二进制）。

验证：
```bash
./cc-connect --version
# 预期输出: cc-connect v1.3.3-beta.4
#           commit:  a4558147      ← 你的 fork commit
#           built:   2026-06-07T08:18:31Z
```

### 第二步：定位 npm 安装的二进制路径

```bash
# 方法 1：通过 which/where
which cc-connect
# 输出: /d/nodejs/node_global/cc-connect  ← 这是 wrapper 脚本

# 方法 2：直接查看 npm 全局模块
ls -la "$(npm root -g)/cc-connect/bin/"
# 输出: cc-connect.exe  ← 这才是真正的二进制
```

**npm 安装结构说明**：
```
$npm_root/cc-connect/
├── bin/cc-connect.exe      ← 真正的 Go 二进制（替换目标）
├── run.js                  ← Node.js wrapper，透传命令给 bin/cc-connect.exe
├── install.js              ← 版本不匹配时自动重装
└── package.json            ← 记录版本号
```

全局 `cc-connect` 命令的调用链：
```
用户输入 cc-connect xxx
  → /d/nodejs/node_global/cc-connect (shell wrapper)
    → node run.js xxx
      → bin/cc-connect.exe xxx
```

确认当前版本：
```bash
cc-connect --version
# 官方版输出: commit: d577db28  ← 上游 commit，非 fork
```

### 第三步：停止并卸载旧 Daemon

```bash
# 停止服务
cc-connect daemon stop

# 卸载 schtasks 计划任务
cc-connect daemon uninstall

# 确认已卸载
cc-connect daemon status
# 预期: Status: Not installed
```

> 如果 daemon uninstall 提示权限不足，需在**管理员终端**执行：
> ```powershell
> schtasks /Delete /TN "cc-connect" /F
> ```

### 第四步：确认旧进程已退出

```bash
# 检查是否有残留进程
ps -W | grep -i cc-connect | grep -v grep

# 如果仍有进程残留，需在管理员终端终止：
# taskkill /PID <PID> /F
```

> **常见卡点**：旧 cc-connect.exe 进程会锁定二进制文件，导致 `cp` 报 "Device or resource busy"。必须先杀掉所有 cc-connect.exe 进程才能替换。

### 第五步：替换二进制

```bash
# 替换 npm 安装的二进制
# 注意：MINGW 下编译产物无 .exe 后缀，但 cp 到目标时需要加 .exe
cp /d/WorkSpace/src/cc-connect/cc-connect "$(npm root -g)/cc-connect/bin/cc-connect.exe"
```

验证替换成功：
```bash
"$(npm root -g)/cc-connect/bin/cc-connect.exe" --version
# 预期输出: commit: a4558147  ← 你的 fork commit
```

> **npm 更新会覆盖**：以后执行 `npm install -g cc-connect@beta` 会重新下载官方二进制覆盖你的替换。升级后需重新执行 `cp`。

### 第六步：重新安装 Daemon

```bash
# 安装并启动（fork 修复后无需 --config 参数）
cc-connect daemon install
```

预期输出：
```
cc-connect daemon installed and started.

  Platform:  schtasks
  Binary:    D:\nodejs\node_global\node_modules\cc-connect\bin\cc-connect.exe
  WorkDir:   C:\Users\KC\.cc-connect
  Log:       C:\Users\KC\.cc-connect\logs\cc-connect.log
  LogMax:    10 MB
```

### 第七步：验证

```bash
# 检查全局命令版本
cc-connect --version
# 预期: commit: a4558147 (fork)

# 检查 daemon 状态
cc-connect daemon status
# 预期: Status: Running

# 检查日志确认平台连接正常
cc-connect daemon logs -f
# 预期: platform ready + engine started + connected
```

### 完整流程速查（复制即用）

```bash
# 1. 编译
cd /d/WorkSpace/src/cc-connect && make build-noweb

# 2. 停卸旧 daemon
cc-connect daemon stop && cc-connect daemon uninstall

# 3. 确认无残留进程
ps -W | grep -i cc-connect | grep -v grep

# 4. 替换二进制
cp /d/WorkSpace/src/cc-connect/cc-connect "$(npm root -g)/cc-connect/bin/cc-connect.exe"

# 5. 重装 daemon
cc-connect daemon install

# 6. 验证
cc-connect --version && cc-connect daemon status
```

### 常见问题

| 问题 | 原因 | 解决 |
|------|------|------|
| `cp: Device or resource busy` | 旧 cc-connect.exe 进程仍运行 | `ps -W \| grep cc-connect` 找 PID，管理员 `taskkill /PID <PID> /F` |
| `daemon install` 报 config not found | 未替换二进制，用的仍是旧版 | 确认 `cc-connect --version` 显示 fork commit |
| `npm update` 后版本回退 | npm 重装了官方二进制 | 重新执行 `cp` |
| `Permission denied` (taskkill) | 需要管理员权限 | 在管理员终端执行 taskkill |
