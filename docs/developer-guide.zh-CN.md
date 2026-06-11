# cc-connect 开发者指南

> 面向维护者 & 贡献者 | v1.3.3 | 最后更新 2026-06-11

---

## 1. 架构与代码结构

### 目录布局

```
cc-connect/
├── cmd/cc-connect/        ← 入口：CLI、daemon、补全脚本生成
├── config/                ← TOML 配置解析、环境变量替换、手术式保存
├── core/                  ← 核心：Engine、接口定义、i18n、卡片、会话、注册表
│   ├── engine.go          ← 消息路由中心（446KB）
│   ├── interfaces.go      ← Platform/Agent/AgentSession 等接口
│   ├── registry.go        ← RegisterAgent/RegisterPlatform 插件注册
│   └── *_test.go          ← 每个模块均有对应测试
├── agent/                 ← 14 个 Agent 适配器
│   ├── claudecode/        ← 主力 Agent，6 种权限模式
│   ├── codex/             ← OpenAI Codex，4 种模式
│   ├── gemini/            ← Google Gemini CLI
│   ├── cursor/            ← tmux 屏幕抓取
│   ├── iflow/             ← 进程流式，含工具超时
│   ├── kimi/              ← 4 种模式
│   ├── pi/                ← 6 级推理强度，会话文件管理
│   ├── acp/               ← JSON-RPC over stdio 协议层
│   ├── devin/             ← ACP 封装
│   ├── copilot/           ← Content-Length 帧，2 种模式
│   ├── antigravity/       ← 3 种模式
│   ├── opencode/          ← 2 种模式，持久模型缓存
│   ├── qoder/             ← 2 种模式
│   └── tmux/              ← 自动创建窗格，自定义 Shell
├── platform/              ← 13+1 个 Platform 适配器
│   ├── feishu/            ← 长连接事件，交互式卡片
│   ├── telegram/          ← Long Polling
│   ├── discord/           ← Gateway WS
│   ├── slack/             ← Socket Mode
│   ├── dingtalk/          ← HTTP 回调，AI 卡片降级
│   ├── wecom/             ← HTTP 回调
│   ├── weixin/            ← ilink 协议
│   ├── qq/  qqbot/        ← QQ 协议
│   ├── line/  weibo/      ← HTTP 协议
│   ├── max/               ← Long-poll/Webhook
│   ├── wps-xiezuo/        ← WebSocket，AES-256-CBC + HMAC-SHA256
│   └── bridge/            ← 内置 Bridge Platform（由 core/bridge.go 协调）
├── daemon/                ← systemd/launchd/schtasks 服务管理
├── web/                   ← React SPA（Vite + pnpm）
│   ├── src/               ← 前端源码
│   └── dist/              ← 构建产物 → embed.go 嵌入二进制
├── npm/                   ← npm 包分发
│   ├── package.json       ← @game1991/cc-connect
│   ├── install.js          ← 下载二进制到 bin/
│   └── run.js              ← 版本检查 + 代理执行
├── tests/                 ← 测试套件
│   ├── e2e/               ← 端到端测试
│   ├── blackbox/          ← 黑盒测试（p0/p1/p2 分级）
│   ├── integration/       ← 集成测试
│   ├── performance/       ← 性能基准
│   ├── mocks/             ← Mock 实现
│   └── release_local/     ← 本地发布测试
├── scripts/               ← 辅助脚本
├── docs/                  ← 文档
└── .github/workflows/     ← CI/CD
```

### 依赖方向规则

```
cmd/ → config/, core/, agent/*, platform/*
agent/*   → core/   （绝不引用其他 agent 或 platform）
platform/* → core/  （绝不引用其他 platform 或 agent）
core/     → 仅标准库（绝不引用 agent/ 或 platform/）
```

**`core/` 是核心枢纽**，定义所有接口并包含 Engine。core 包必须保持平台无关性——永远不要在 core 中写 `if p.Name() == "feishu"` 或 `CreateAgent("claudecode", ...)`。

### 插件架构

Agent 和 Platform 通过 `init()` 函数中的 `core.RegisterAgent()` / `core.RegisterPlatform()` 自注册。Engine 通过配置中的字符串名称调用 `core.CreateAgent()` / `core.CreatePlatform()` 创建实例。

每个插件对应一个 `cmd/cc-connect/plugin_*.go` 文件，包含 build tag（如 `//go:build !no_feishu`）。默认编译全部插件。

### 核心接口

| 接口 | 职责 | 必需 |
|------|------|------|
| `Platform` | IM 平台适配（Start, Reply, Send, Stop） | 是 |
| `Agent` | AI Agent 适配（StartSession, ListSessions, Stop） | 是 |
| `AgentSession` | 运行中的双向会话（Send, RespondPermission, Events） | 是 |
| `Engine` | 中央编排器，路由消息 | 是 |
| `CardSender` | 富卡片消息 | 否 |
| `InlineButtonSender` | 内联键盘按钮 | 否 |
| `ProviderSwitcher` | 多模型切换 | 否 |
| `DoctorChecker` | Agent 特定健康检查 | 否 |
| `AgentDoctorInfo` | CLI 二进制元数据 | 否 |

---

## 2. 构建系统

### Makefile targets

| Target | 说明 | 时间估算 |
|--------|------|----------|
| `make build` | 全量编译（含 Web UI） | ~30s |
| `make build-noweb` | 无 Web UI | ~10s |
| `make web` | 仅前端构建 | ~15s |
| `make run` | 编译并运行 | ~30s |
| `make clean` | 清理构建产物 | 即时 |
| `make test-fast` | 单元 + 冒烟测试 | < 2 min |
| `make test-full` | 完整套件含回归测试 | < 10 min |
| `make test-smoke` | 仅冒烟测试 | < 1 min |
| `make test-e2e` | E2E + 回归测试 | ~5 min |
| `make test-performance` | 性能基准测试 | ~3 min |
| `make test-release` | 完整 + 性能基准 | ~12 min |
| `make test-release-local` | 本地发布验证 | ~2 min |
| `make lint` | golangci-lint | ~1 min |
| `make release-all` | 全平台交叉编译+打包 | ~5 min |

### ldflags 注入

版本信息通过链接标志注入：

```makefile
LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.buildTime=$(BUILD_TIME)
```

运行 `cc-connect --version` 输出三行：版本号、commit hash、构建时间。

### 选择性编译

通过 build tag 控制编译范围：

```bash
# 只编译特定 Agent 和 Platform
make build AGENTS=claudecode PLATFORMS_INCLUDE=feishu,telegram

# 排除不需要的组件
make build EXCLUDE=discord,dingtalk,qq,qqbot,line,weibo

# 直接使用 build tag
go build -tags 'no_discord no_dingtalk no_qq no_qqbot' ./cmd/cc-connect
```

全部可用 Agent tag：`no_acp no_antigravity no_claudecode no_codex no_copilot no_cursor no_devin no_gemini no_iflow no_kimi no_opencode no_pi no_qoder no_tmux`

全部可用 Platform tag：`no_feishu no_telegram no_discord no_slack no_dingtalk no_wecom no_weixin no_qq no_qqbot no_line no_weibo no_max no_wps_xiezuo`

Web UI：`no_web`

### 前端构建

```bash
cd web && npm install && npm run dev    # 开发模式，端口 9821
cd web && pnpm install && pnpm build   # 生产构建
```

构建产物输出到 `web/dist/`，通过 `embed.go` 嵌入 Go 二进制。`embed_stub.go`（build tag: `no_web`）提供空嵌入。

### 交叉编译与发布

6 平台构建矩阵：

| GOOS | GOARCH |
|------|--------|
| linux | amd64 |
| linux | arm64 |
| darwin | amd64 |
| darwin | arm64 |
| windows | amd64 |
| windows | arm64 |

```bash
# 单平台
make release TARGET=linux/amd64
make release-noweb TARGET=windows/amd64

# 全平台
make release-all
```

产出 `.tar.gz`（Linux/macOS）或 `.zip`（Windows），附带 SHA256 校验和。

---

## 3. 测试策略

### 测试层级

| 层级 | 命令 | 覆盖范围 | 时间 | 触发时机 |
|------|------|----------|------|----------|
| 冒烟测试 | `make test-smoke` | 基础启动/关闭/消息发送 | < 1 min | 每次推送 |
| 快速测试 | `make test-fast` | 单元 + 冒烟 | < 2 min | 每次推送 |
| 完整测试 | `make test-full` | 含回归测试 | < 10 min | PR 合入 |
| E2E | `make test-e2e` | 完整 Agent-Platform 集成 | ~5 min | PR 合入 |
| 性能基准 | `make test-performance` | 吞吐量/延迟基准 | ~3 min | 发布前 |
| 发布验证 | `make test-release` | 完整 + 性能 | ~12 min | 打 tag 前 |
| 本地发布 | `make test-release-local` | 精选核心场景 | ~2 min | 打 tag 前 |

### 测试目录结构

```
tests/
├── e2e/                   ← 端到端测试
│   ├── smoke_test.go      ← build tag: smoke
│   ├── regression_test.go ← build tag: regression
│   └── *_test.go          ← E2E 场景测试
├── blackbox/              ← 黑盒测试
│   ├── p0/ p1/ p2/       ← 优先级分级
│   └── platform/          ← 平台特定测试
├── integration/           ← 集成测试
│   ├── config_matrix/     ← 配置组合矩阵
│   ├── engine_matrix/     ← Engine 多场景
│   ├── media_pipeline/    ← 媒体处理管道
│   └── turn_contract/     ← 轮次契约
├── performance/           ← 性能基准
│   └── bench_test.go      ← build tag: performance
├── mocks/                 ← Mock 实现
│   ├── mock_agent.go      ← Agent 桩
│   ├── mock_platform.go   ← Platform 桩
│   └── fake/              ← 轻量假实现
└── release_local/         ← 本地发布验证
```

### 测试模式

- **stub 模式**：不启动真实 Agent 进程，使用 mock Agent/Platform
- **channel 模拟**：通过 Go channel 模拟事件流，验证 Engine 消息处理逻辑
- **Race detector**：CI 和 `make test-fast`/`test-full` 均启用 `-race`

### 本地发布测试

```bash
make test-release-local
```

执行 `tests/release_local/` 中的精选核心测试，验证配置解析、会话管理、关键 Engine 场景，确保打包前的基本可用性。

---

## 4. 版本管理

### 格式

```
v<major>.<minor>.<patch>-fork.<N>
```

两处存储须同步：

| 位置 | 字段 | 当前值 |
|------|------|--------|
| `Makefile` | `VERSION` | `v1.3.3` |
| `npm/package.json` | `version` | `1.3.3` |

同步校验：

```bash
grep '^VERSION' Makefile | awk -F= '{print $2}'
node -e "console.log(require('./npm/package.json').version)"
```

### fork.N 递增规则

- 每次发布新 fork 版本时递增 N
- `fork.N` 独立于上游的 `-beta.N` / `-rc.N` 递增
- 示例演进：`v1.3.3-fork.1` → `v1.3.3-fork.2` → `v1.4.0-fork.1`

### fork.N 排序陷阱详解

npm 包的 `run.js` 在每次执行时调用 `isNewerOrEqual()` 判断本地二进制是否需要更新。其逻辑为：

```
1. 比较 major.minor.patch 数字 → 大者胜出
2. 数字相同则比较预发布标签：
   a. 无预发布 > 有预发布（1.2.3 >= 1.2.3-beta.1）
   b. 都有预发布 → 比较 preTag 字典序 → 比较 preNum
```

**关键陷阱**：`preTag` 按字典序比较。`fork` > `beta` > `alpha`（f > b > a）。

如果上游发布 `v1.3.3-beta.5`，而本地是 `v1.3.3-fork.4`：

```
数字比较：1.3.3 == 1.3.3 → 相等
预发布比较：fork vs beta → "fork" > "beta"（字典序）
→ isNewerOrEqual 返回 true → 不重新下载 ✓
```

**如果 fork.N 排序低于上游 beta**：

假设误用 `v1.3.3-alpha.4`（alpha <beta），则：
```
数字比较：1.3.3 == 1.3.3 → 相等
预发布比较：alpha vs beta → "alpha" < "beta"
→ isNewerOrEqual 返回 false → run.js 触发自动下载 → 本地二进制被覆盖 ✗
```

**违反后果**：`run.js` 自动下载上游版本覆盖本地二进制，导致 fork 功能丢失。

**必须确保**：`preTag` 的字典序始终大于上游可能使用的标签。`fork` 满足此条件（f > b）。

### 版本修改检查清单

1. 编辑 `Makefile` 中的 `VERSION`
2. 编辑 `npm/package.json` 中的 `version`
3. 两处版本号须一致（不含 `v` 前缀的差异）
4. `preTag` 必须使用 `fork`，不可改为 `alpha`/`beta`/`rc`
5. 提交信息：`chore: bump version to vX.Y.Z-fork.N`
6. 打 tag：`git tag vX.Y.Z-fork.N`

---

## 5. 发布流程

### 完整步骤

```
1. 确认版本号同步（Makefile + npm/package.json）
2. 运行完整测试 + 性能基准：make test-release
3. 运行本地发布验证：make test-release-local
4. 提交版本修改：git commit -m "chore: bump version to vX.Y.Z-fork.N"
5. 打 tag：git tag vX.Y.Z-fork.N
6. 推送：git push origin <branch> --tags
```

### CI 流程

tag `v*-fork.*` 推送后，`fork-release.yml` 自动触发：

```
git tag v*-fork.* → 5 平台编译 → GitHub Release → npm.pkg.github.com
```

三阶段 Job：

1. **build-and-release**：5 平台矩阵编译（Go 1.24 + Node 20 + pnpm 9），构建前端 → 构建二进制 → 打包归档 → 上传 artifact
2. **github-release**：下载所有 artifact → 生成 SHA256 校验和 → 创建 GitHub Release（自动生成 release notes）
3. **publish-npm**：下载 artifact → 解压 Windows 二进制到 `npm/bin/` → 同步 `package.json` version 为 tag 版本 → `npm publish` 到 GitHub Packages

### 双源分发

| 源 | 地址 | 面向 | 优先级 |
|------|------|------|--------|
| GitHub | github.com/game1991/cc-connect | 全球 | 主 |
| Gitee | gitee.com/game1991/cc-connect | 中国 | 备 |

自更新命令：`cc-connect update`（默认从 GitHub 获取），`cc-connect update --pre`（预发布版）。配置 `prefer_gitee = true` 切换优先级。

### npm 包结构

```
@game1991/cc-connect
├── package.json       ← bin.cc-connect → run.js
├── install.js         ← postinstall 钩子，下载二进制到 bin/
├── run.js             ← 版本检查 + 代理执行
├── README.md
└── bin/
    ├── cc-connect         ← Linux/macOS 二进制
    └── cc-connect.exe     ← Windows 二进制
```

**执行流程**：

```
用户运行 cc-connect
  → npm 解析 bin → run.js
  → run.js 调 parseVersion() 解析 EXPECTED_VER
  → run.js 检查 bin/ 下二进制是否存在
  → 如存在：执行二进制 --version 获取已安装版本
  → 调 isNewerOrEqual(已安装版本, EXPECTED_VER)
  → 如 isNewerOrEqual 返回 false：触发 install.js 重新下载
  → execFileSync 代理执行 bin/cc-connect
```

---

## 6. 上游同步工作流

### 添加 upstream remote

```bash
git remote add upstream https://github.com/chenhg5/cc-connect.git
```

### fetch + merge 流程

```bash
git fetch upstream
git merge upstream/main
```

### 合并后必查项

| 检查项 | 说明 |
|--------|------|
| `npm/install.js` | 确认下载源指向 fork 仓库 |
| `npm/run.js` | 确认 `isNewerOrEqual` 逻辑未被覆盖 |
| `daemon/manager.go` | 确认 Meta 结构体未被修改 |
| `Makefile VERSION` | 确认版本号未被覆盖 |
| `package.json` | 确认 name/repository/publishConfig 指向 fork |
| `.github/workflows/` | 确认 CI 仍为 fork 版本 |

### 冲突解决策略

- 版本号冲突：保留 fork 版本号
- CI 工作流冲突：保留 fork 的 tag pattern（`v*-fork.*`）和 npm 发布配置
- 功能代码冲突：手动合并，优先保留上游修复，叠回 fork 定制

---

## 7. 平台特定开发笔记

### Windows

- **无 pty**：iFlow Agent 使用 `creack/pty`，Windows 不支持，跳过 pty 相关测试
- **schtasks daemon**：使用 Windows 任务计划程序而非 systemd
- **`.old` 备份**：自更新时当前二进制重命名为 `.old`
- **无 `syscall.Exec`**：Windows 不支持进程替换，使用 `execFileSync` 代理
- **`.cmd` 弹框问题**：`ensureCmdFileAssociation` 函数会创建空的 `OpenWithList` 注册表键，导致 `.cmd` 文件关联弹框。**已移除该函数**。如需诊断可检查注册表 `HKEY_CURRENT_USER\SOFTWARE\Microsoft\Windows\CurrentVersion\Explorer\FileExts\.cmd`

### macOS

- **quarantine**：下载的二进制可能被 Gatekeeper 隔离，需 `xattr -cr cc-connect` 解除
- **launchd**：daemon 使用 `~/Library/LaunchAgents/com.game1991.cc-connect.plist`

### Linux

- **systemd**：daemon 使用 `~/.config/systemd/user/cc-connect.service`
- **run_as_user**：通过 sudo 以指定用户身份运行 Agent，含审计日志和预检探针
- **Unix socket**：API Server 使用 `~/.cc-connect/run/api.sock`

---

## 8. 开发环境搭建

### 前置依赖

| 依赖 | 最低版本 | 用途 |
|------|---------|------|
| Go | 1.23+ | 后端编译 |
| Node.js + npm | 18+ | 前端构建 + npm 分发 |
| pnpm | 9+ | 前端依赖锁定（可选） |
| Git | 任意 | 版本控制 |
| ffmpeg | 任意 | 语音消息处理（可选） |

### 首次构建

```bash
# 克隆
git clone https://github.com/game1991/cc-connect.git
cd cc-connect

# 编译（含 Web UI）
make build

# 或无 Web UI
make build-noweb

# 运行
./cc-connect --version
```

### 前端开发模式

```bash
cd web && pnpm install && pnpm dev
# 浏览器访问 http://localhost:9821
# 同时启动 Go 后端连接 Vite 代理
```

### IDE 建议

- **VSCode**：Go 扩展 + ESLint
- **GoLand**：原生支持，推荐用于 Engine 调试
- **调试**：`dlv debug ./cmd/cc-connect -- --config path/to/config.toml`

---

## 9. 代码风格与提交规范

### Go 风格规则

- 遵循 `gofmt` / `go vet` / `golangci-lint`
- 使用 `strings.EqualFold` 进行大小写不敏感比较
- 避免在 `init()` 中做除插件注册以外的操作
- 函数超过 ~80 行时提取辅助函数
- 命名：`New()` 构造、`Get/Set` 访问、避免重复（`feishu.FeishuPlatform` → `feishu.Platform`）

### 错误处理

- 始终使用 `fmt.Errorf("context: %w", err)` 包装错误
- 绝不静默吞掉错误；至少用 `slog.Error` / `slog.Warn` 记录
- 使用 `core.RedactToken()` 在错误消息中遮蔽 token/密钥
- 统一使用 `slog` 结构化日志；禁止 `log.Printf` / `fmt.Printf`

### 并发安全

- Agent session 从多个 goroutine 访问；用 `sync.Mutex` 或 `atomic` 保护共享状态
- 使用 `context.Context` 传播取消
- Channel 须明确所有权；文档化谁关闭
- 一次性清理用 `sync.Once`（如 `pendingPermission.resolve()`）

### i18n

所有用户可见字符串必须通过 `core/i18n.go`：

1. 定义 `MsgKey` 常量
2. 为所有支持语言添加翻译（EN, ZH, ZH-TW, JA, ES）
3. 使用 `e.i18n.T(MsgKey)` 或 `e.i18n.Tf(MsgKey, args...)`

### 提交信息格式

```
<type>(<scope>): <description>

[可选 body]
```

type：`feat` / `fix` / `refactor` / `docs` / `chore` / `test` / `ci`

scope：`engine` / `feishu` / `claudecode` / `daemon` / `config` / 等

示例：
```
feat(engine): add multi-workspace channel binding
fix(dingtalk): handle AI card degradation to plain text
docs: restructure fork-guide into product + developer guides
chore: bump version to v1.3.3-fork.11
```

### Pre-Commit 清单

1. **构建通过**：`go build ./...`
2. **测试通过**：`go test ./...`
3. **Core 无硬编码**：`grep -r 'Name() == "feishu"' core/`（应无结果）
4. **i18n 完整**：所有新字符串有 5 语言翻译
5. **无密钥泄露**：源码中不含 API Key / Token / 密码
