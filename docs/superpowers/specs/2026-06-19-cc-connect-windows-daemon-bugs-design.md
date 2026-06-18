# cc-connect Windows Daemon Bug 修复设计

日期: 2026-06-19
状态: Approved

## 概述

修复 cc-connect 在 Windows 服务 (svc) 模式下的三个关联 Bug：
1. `daemon stop` 因权限不足而失败（非锁死锁）
2. svc 模式下日志文件不生成（fixSvcEnvironment 诊断不可见）
3. svc 模式下 EnvExtra 丢失导致 agent 子进程无法启动

## Phase 1: 根因调查结论

### Bug 1: daemon stop — 非管理员权限不足

**Bug 报告中的根因描述有误**：报告称 `daemon stop` 因获取实例锁而死锁，但代码审查证明 `stopWithFallback()` 全程不获取实例锁。

**真实根因**：非管理员用户对 SYSTEM 进程执行停止操作时权限不足。

执行路径追踪：
```
用户: cc-connect daemon stop
  → main.go:190 子命令路由（在锁获取 main.go:275 之前）
  → daemonStop() → stopWithFallback()
  → newMgr() → svcManager{} (无权限问题)
  → mgr.Status() → sc.exe query (只读操作，SYSTEM 允许)
  → mgr.Stop() → sc.exe stop → ERROR_ACCESS_DENIED
  → _ = mgr.Stop() ← 错误被静默吞掉！
  → loadMeta() → 从用户目录读取 daemon.json ✓
  → kill(configPath) → OpenProcess(PROCESS_TERMINATE)
                      → ERROR_ACCESS_DENIED ← 非管理员无权终止 SYSTEM 进程
  → kill 返回 false → stopWithFallback 返回 nil
  → 用户看到 "cc-connect daemon stopped." 但进程实际仍在运行
```

关键问题：
- `_ = mgr.Stop()` 吞掉了 sc.exe stop 的 ACCESS_DENIED 错误
- `KillExistingInstance` 对 SYSTEM 进程无 PROCESS_TERMINATE 权限，返回 false（不是错误，只是"没找到可杀的进程"）
- 函数返回 nil，外层打印 "daemon stopped." — **误报成功**

### Bug 2: svc 模式日志不生成

`fixSvcEnvironment()` 机制存在且时序正确（在 `runMain()` 之前调用），但存在两个问题：

1. **诊断信息丢失**：`fixSvcEnvironment` 内所有 `slog.Warn` / `slog.Info` 输出到 stderr。Windows 服务 (SCM) 的 stderr 重定向到不可见位置。若该函数静默失败，用户完全无法获知原因。

2. **fallback 路径不可达**：若 daemon install 未传 `--config`（极端情况），`fixSvcEnvironment` 的 dataDir 会 fallback 到 `SystemDrive/Users/.cc-connect`，该路径不存在，`ReadFile` 失败，环境不恢复。

时序分析（正常 install + 有 --config）：
```
svcHandler.Execute():
  1. filterArgs(os.Args, "--service")    → 移除 --service 标志
  2. fixSvcEnvironment()                → 从 --config 推导 dataDir
     ├─ 读取 C:\Users\KC\.cc-connect\daemon.json ✓
     ├─ 恢复 USERPROFILE=KC, HOME=KC, PATH ✓
     ├─ 设置 CC_LOG_FILE ✓
     └─ 创建 logs/ 目录 ✓
  3. status <- Running                    → 向 SCM 报告
  4. runMain()                            →
     └─ os.Getenv("CC_LOG_FILE") 有值 ✓  → 打开日志文件 ✓
```

**结论**：在正常安装场景下，fixSvcEnvironment 应该能工作。Bug 2 可能是因为 fixSvcEnvironment 遇到了非预期路径而静默失败，且用户无法看到失败信息。

### Bug 3: svc 模式 EnvExtra 丢失

**影响范围限定**：仅 svc 模式，schtasks 模式已正确实现。

对比各平台 EnvExtra 渲染机制：

| 平台/模式 | EnvExtra 渲染位置 | 运行时恢复 |
|-----------|------------------|-----------|
| Linux (systemd) | 单元文件 `Environment="KEY=VALUE"` | 不需要 |
| macOS (launchd) | plist `<EnvironmentVariables>` dict | 不需要 |
| Windows (schtasks) | PS1 脚本 `$env:KEY = 'VALUE'` (**已实现**, windows.go:212-230) | 不需要 |
| Windows (svc) | **无！** sc.exe 不支持环境变量 | 需要，但 Meta 缺少 EnvExtra 字段 |

根因确认：
- `SaveMeta()` 不写 `cfg.EnvExtra`（daemon.go:103-112）
- `Meta` 结构体没有 `EnvExtra` 字段（manager.go:77-86）
- `fixSvcEnvironment()` 无法恢复未持久化的数据

真实 `daemon.json` 确认无 `env_extra` 字段：
```json
{
  "log_file": "C:/Users/KC/.cc-connect/logs/cc-connect.log",
  "log_max_size": 10485760,
  "work_dir": "C:/Users/KC/.cc-connect",
  "binary_path": "...",
  "config_file": "C:/Users/KC/.cc-connect/config.toml",
  "env_path": "C:\\Python314\\...",
  "home_dir": "C:/Users/KC",
  "installed_at": "2026-06-19T00:35:34+08:00"
}
```

## 修复方案

### Bug 1: 权限不足的清晰反馈

**目标**：让用户知道为什么 stop 失败，而不是误报成功。

#### 修改文件：`cmd/cc-connect/daemon.go`

**`stopWithFallback()` 修改**：

```go
func stopWithFallback(newMgr func() (daemon.Manager, error), loadMeta func() (*daemon.Meta, error), kill func(string) bool, stderr io.Writer) error {
    mgr, err := newMgr()
    if err != nil {
        return fmt.Errorf("error: %v", err)
    }
    st, _ := mgr.Status()
    if st == nil || !st.Installed {
        return fmt.Errorf("service is not installed. Run first:\n  cc-connect daemon install --work-dir /path/to/config-dir")
    }

    // Log the platform stop result instead of silently discarding
    if stopErr := mgr.Stop(); stopErr != nil {
        slog.Warn("daemon stop: platform stop reported error", "platform", st.Platform, "error", stopErr)
        fmt.Fprintf(stderr, "Platform stop reported: %v\n", stopErr)
    }

    meta, merr := loadMeta()
    if merr != nil {
        slog.Warn("daemon stop: could not load metadata, skipping kill verification", "error", merr)
        // If platform stop succeeded, this is fine.
        // If platform stop failed, we can't verify — report the platform error.
        return nil
    }
    configPath := metaConfigPath(meta)
    if kill(configPath) {
        fmt.Fprintln(stderr, "Warning: process was still running after platform stop; killed via instance lock PID")
        if err := RemoveInstanceLock(configPath); err != nil {
            slog.Warn("failed to remove stale lock file", "error", err)
        }
    }
    return nil
}
```

**`daemonStop()` 修改**：增加 Windows svc 模式权限预检。

```go
func daemonStop() {
    if runtime.GOOS == "windows" {
        mgr, _ := daemon.NewManager()
        if mgr != nil {
            st, _ := mgr.Status()
            if st != nil && st.Installed && st.Platform == "svc" && !daemon.IsAdmin() {
                fmt.Fprintln(os.Stderr, "Error: stopping a Windows Service (svc mode) requires administrator privileges.")
                fmt.Fprintln(os.Stderr, "Run from an elevated terminal (Run as Administrator).")
                os.Exit(1)
            }
        }
    }
    if err := stopWithFallback(daemon.NewManager, daemon.LoadMeta, KillExistingInstance, os.Stderr); err != nil {
        fmt.Fprintln(os.Stderr, err.Error())
        os.Exit(1)
    }
    fmt.Println("cc-connect daemon stopped.")
    if runtime.GOOS == "windows" {
        fmt.Println("Verify:  tasklist /FI \"IMAGENAME eq cc-connect.exe\"")
    }
}
```

设计决策说明：
- 权限检查只在 `daemonStop()` 中做（调用者），`stopWithFallback()` 不包含退出逻辑（保持可测试性）
- 如果是 schtasks 模式，不检查管理员权限（schtasks 以用户身份运行，不需要额外权限）
- `stopWithFallback()` 不再吞掉 `mgr.Stop()` 的错误，而是输出到 stderr 并用 slog 记录

### Bug 2: fixSvcEnvironment 诊断可见性

**目标**：让 fixSvcEnvironment 的失败/成功信息可见，而不是静默失败。

#### 修改文件：`cmd/cc-connect/svc_run_windows.go`

在 `fixSvcEnvironment()` 中增加诊断文件写入。由于此时 slog 还没有设置日志文件 handler（`runMain()` 还没被调用），**所有关键诊断信息必须直接写文件，不能依赖 slog**。

```go
func fixSvcEnvironment() {
    // ...existing SYSTEM account detection code...

    var configFile string
    for i := 0; i < len(os.Args); i++ {
        // ...existing --config parsing...
    }

    var dataDir string
    if configFile != "" {
        dataDir = filepath.Dir(filepath.FromSlash(configFile))
    } else {
        dataDir = filepath.Join(os.Getenv("SystemDrive"), "Users", ".cc-connect")
    }

    // Diagnostic file — visible even when stderr is redirected by SCM.
    // Written BEFORE attempting to read daemon.json so that all steps are logged.
    diagPath := filepath.Join(dataDir, "svc-env-fix.log")
    diagFile, diagErr := os.OpenFile(diagPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
    writeDiag := func(format string, args ...any) {
        if diagFile != nil {
            fmt.Fprintf(diagFile, "[%s] "+format+"\n", append([]any{time.Now().Format(time.RFC3339)}, args...)...)
        }
    }
    defer func() {
        if diagFile != nil {
            diagFile.Close()
        }
    }()

    writeDiag("fixSvcEnvironment started")
    writeDiag("USERPROFILE=%s (SYSTEM=%v)", up, strings.Contains(strings.ToLower(up), "system32") || strings.Contains(strings.ToLower(up), "systemprofile"))
    writeDiag("configFile=%s dataDir=%s", configFile, dataDir)

    metaPath := filepath.Join(dataDir, "daemon.json")
    data, err := os.ReadFile(metaPath)
    if err != nil {
        writeDiag("FAILED to read daemon.json: %v", err)
        slog.Warn("svc: could not read daemon.json, environment not restored", "path", metaPath, "error", err)
        return
    }
    writeDiag("read daemon.json OK, %d bytes", len(data))

    var meta daemon.Meta
    if err := json.Unmarshal(data, &meta); err != nil {
        writeDiag("FAILED to parse daemon.json: %v", err)
        slog.Warn("svc: could not parse daemon.json", "error", err)
        return
    }

    // ...existing HOME/USERPROFILE restore code...
    if meta.HomeDir != "" {
        hd := filepath.FromSlash(meta.HomeDir)
        os.Setenv("USERPROFILE", hd)
        os.Setenv("HOME", hd)
        drive := filepath.VolumeName(hd)
        os.Setenv("HOMEDRIVE", drive)
        os.Setenv("HOMEPATH", strings.TrimPrefix(hd, drive))
        writeDiag("restored home dir: USERPROFILE=%s", hd)
    } else if configFile != "" {
        homeDir := filepath.Dir(dataDir)
        os.Setenv("USERPROFILE", homeDir)
        os.Setenv("HOME", homeDir)
        drive := filepath.VolumeName(homeDir)
        os.Setenv("HOMEDRIVE", drive)
        os.Setenv("HOMEPATH", strings.TrimPrefix(homeDir, drive))
        writeDiag("derived home dir from config: %s", homeDir)
    }

    logFile := meta.LogFile
    if logFile == "" {
        logFile = filepath.ToSlash(filepath.Join(dataDir, "logs", "cc-connect.log"))
    }
    logFile = filepath.FromSlash(logFile)
    os.Setenv("CC_LOG_FILE", logFile)
    writeDiag("CC_LOG_FILE=%s", logFile)

    if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
        writeDiag("FAILED to create log dir: %v", err)
        fmt.Fprintf(os.Stderr, "svc: failed to create log dir %s: %v\n", filepath.Dir(logFile), err)
    }

    if meta.EnvPATH != "" {
        os.Setenv("PATH", meta.EnvPATH)
        writeDiag("restored PATH, len=%d", len(meta.EnvPATH))
    }

    // Restore EnvExtra (new — see Bug 3)
    if len(meta.EnvExtra) > 0 {
        for k, v := range meta.EnvExtra {
            os.Setenv(k, v)
        }
        writeDiag("restored EnvExtra, count=%d", len(meta.EnvExtra))
    }

    writeDiag("fixSvcEnvironment completed")
}
```

注意：
- 所有 `slog.Warn/Info` 保留（未来 runMain 设置日志 handler 后也许能捕获），但不依赖它们
- `writeDiag` 辅助函数确保关键信息写入可访问的文件
- `writeDiag` 的 defer Close 保证即使 panic 也能写入

### Bug 3: Meta.EnvExtra 持久化 + 恢复

**影响范围**：仅 svc 模式。schtasks 模式的 PS1 脚本已正确渲染 EnvExtra (windows.go:212-230)，无需修改。

#### 1. `daemon/manager.go` — Meta 结构体扩展

```go
type Meta struct {
    LogFile     string            `json:"log_file"`
    LogMaxSize  int64             `json:"log_max_size"`
    WorkDir     string            `json:"work_dir"`
    BinaryPath  string            `json:"binary_path"`
    ConfigFile  string            `json:"config_file,omitempty"`
    EnvPATH     string            `json:"env_path,omitempty"`
    HomeDir     string            `json:"home_dir,omitempty"`
    EnvExtra    map[string]string `json:"env_extra,omitempty"` // 新增
    InstalledAt string            `json:"installed_at"`
}
```

`omitempty` 保证旧版 daemon.json 不含 `env_extra` 字段时不会报错，`LoadMeta` 反序列化时 EnvExtra 为 nil（空 map），不影响已有安装。

#### 2. `cmd/cc-connect/daemon.go` — SaveMeta 写入 EnvExtra

```go
daemon.SaveMeta(&daemon.Meta{
    LogFile:     filepath.ToSlash(cfg.LogFile),
    LogMaxSize:  cfg.LogMaxSize,
    WorkDir:     filepath.ToSlash(cfg.WorkDir),
    BinaryPath:  filepath.ToSlash(cfg.BinaryPath),
    ConfigFile:  filepath.ToSlash(cfg.ConfigFile),
    EnvPATH:     cfg.EnvPATH,
    HomeDir:     filepath.ToSlash(homeDir),
    EnvExtra:    cfg.EnvExtra, // 新增
    InstalledAt: daemon.NowISO(),
})
```

#### 3. `cmd/cc-connect/svc_run_windows.go` — fixSvcEnvironment 恢复 EnvExtra

已在 Bug 2 的修改中集成（见上方 `writeDiag("restored EnvExtra, count=%d", len(meta.EnvExtra))` 之前的代码块）。

关键代码：
```go
// Restore captured env vars from install time (API keys, proxy settings, etc.)
if len(meta.EnvExtra) > 0 {
    for k, v := range meta.EnvExtra {
        os.Setenv(k, v)
    }
    slog.Info("svc: restored env vars from daemon.json", "count", len(meta.EnvExtra))
}
```

#### 4. `daemon/windows.go` — 无需修改

`buildWindowsTaskScript()` 已在第 212-230 行实现了 EnvExtra 的 PS1 渲染，包含：
- 按 key 排序遍历
- `isValidEnvName` 校验
- 空值过滤
- `writePowerShellEnv` 辅助函数（含 `protectedEnvKeys` 防护）
- 单引号转义

不需要任何新增或修改。

## 影响范围

| 文件 | 变更类型 | 风险 |
|------|---------|------|
| `daemon/manager.go` | Meta 新增 EnvExtra 字段 | 低 — `omitempty` 保证向后兼容 |
| `cmd/cc-connect/daemon.go` | SaveMeta 传入 cfg.EnvExtra | 低 — 新增字段 |
| `cmd/cc-connect/daemon.go` | daemonStop 增加权限预检 | 低 — 仅增加前置检查 |
| `cmd/cc-connect/daemon.go` | stopWithFallback 不再吞错 | 低 — 改变日志行为，不改变控制流 |
| `cmd/cc-connect/svc_run_windows.go` | fixSvcEnvironment 增加诊断文件 + EnvExtra 恢复 | 低 — 增加文件写入和 os.Setenv 调用 |

不修改的文件：
- `daemon/windows.go` — PS1 脚本的 EnvExtra 渲染已存在，无需修改
- `daemon/svcmanager_windows.go` — sc.exe 本身不支持环境变量，使用 daemon.json 替代

## 验证标准

### Bug 1

1. 非管理员运行 `cc-connect daemon stop`（svc 模式）时，应输出 "Error: stopping a Windows Service requires administrator privileges" 并退出
2. 管理员运行 `cc-connect daemon stop` 时，若 sc.exe stop 失败，应输出平台错误信息
3. `stopWithFallback` 的每个步骤都有 slog + stderr 输出，不再静默吞错
4. schtasks 模式不检查管理员权限（schtasks 以用户身份运行）

### Bug 2

1. `cc-connect daemon install`（svc 模式）后，`C:\Users\KC\.cc-connect\logs\cc-connect.log` 存在且有内容
2. `cc-connect daemon logs -f` 能正常跟踪日志
3. `C:\Users\KC\.cc-connect\svc-env-fix.log` 中能看到环境恢复记录
4. 若 fixSvcEnvironment 遇到错误，`svc-env-fix.log` 中能看到 FAILED 记录

### Bug 3（仅 svc 模式）

1. `daemon.json` 中包含 `env_extra` 字段，值与安装时 `cfg.EnvExtra` 一致
2. `cc-connect daemon install`（svc 模式）后，WPS 发消息应出现 kscc.exe 子进程
3. 管理 API 中 session 的 `live` 为 `true`，`agent_session_id` 非空
4. 机器人正常回复消息
5. `svc-env-fix.log` 中能看到 "restored EnvExtra, count=N" 记录
6. schtasks 模式不受影响（PS1 脚本 EnvExtra 渲染已存在）

## 后续工作

- Bug 1 确认权限不足是唯一根因后，考虑增加 `--force` 标志到 `daemon stop` 命令，以管理员身份运行时使用 TerminateProcess 强制终止
- 评估 svc install 时是否通过注册表 `HKLM\SYSTEM\CurrentControlSet\Services\<name>\Parameters\Environment` 注入 EnvExtra，作为 daemon.json 的补充/替代方案
- 评估是否将 schtasks 设为 Windows 默认 daemon 模式（避免 SYSTEM 账户问题），但需解决用户登出后任务停止的局限
