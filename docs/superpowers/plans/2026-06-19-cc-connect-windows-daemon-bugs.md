# cc-connect Windows Daemon Bug 修复 实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 Windows svc 模式下 daemon stop 权限不足误报成功、日志不可见、EnvExtra 丢失三个 Bug

**Architecture:** 扩展 daemon.json 的 Meta 结构以持久化 EnvExtra，在 fixSvcEnvironment 启动时恢复；增加权限预检和错误传播让 stop 失败原因可见；增加诊断文件让 fixSvcEnvironment 的行为可观测

**Tech Stack:** Go 1.23, Windows syscall (PROCESS_TERMINATE), encoding/json

**Spec:** `docs/superpowers/specs/2026-06-19-cc-connect-windows-daemon-bugs-design.md`

---

## Task 1: Meta 结构体新增 EnvExtra 字段

**Files:**
- Modify: `daemon/manager.go:77-86` (Meta struct)
- Modify: `daemon/manager_test.go` (existing serialization tests)

- [ ] **Step 1: Write the failing test**

在 `daemon/manager_test.go` 中添加 Meta EnvExtra 序列化/反序列化测试：

```go
func TestMetaEnvExtraRoundTrip(t *testing.T) {
	original := &Meta{
		LogFile:  "C:/Users/test/.cc-connect/logs/cc-connect.log",
		WorkDir:  "C:/Users/test/.cc-connect",
		EnvPATH:  "/usr/bin",
		HomeDir:  "C:/Users/test",
		EnvExtra: map[string]string{
			"ANTHROPIC_API_KEY": "sk-test-123",
			"HTTPS_PROXY":       "http://proxy:8080",
		},
		InstalledAt: "2026-06-19T12:00:00Z",
	}

	data, err := json.MarshalIndent(original, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var restored Meta
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(restored.EnvExtra) != 2 {
		t.Errorf("EnvExtra len = %d, want 2", len(restored.EnvExtra))
	}
	if restored.EnvExtra["ANTHROPIC_API_KEY"] != "sk-test-123" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", restored.EnvExtra["ANTHROPIC_API_KEY"], "sk-test-123")
	}
	if restored.EnvExtra["HTTPS_PROXY"] != "http://proxy:8080" {
		t.Errorf("HTTPS_PROXY = %q, want %q", restored.EnvExtra["HTTPS_PROXY"], "http://proxy:8080")
	}
}

func TestMetaEnvExtraOmitEmpty(t *testing.T) {
	m := &Meta{
		LogFile:     "test.log",
		InstalledAt: "2026-01-01T00:00:00Z",
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// omitempty 应保证无 env_extra 字段
	if strings.Contains(string(data), "env_extra") {
		t.Errorf("env_extra should be omitted when nil, got:\n%s", data)
	}

	// 反序列化不含 env_extra 的 JSON 时，EnvExtra 应为 nil
	var restored Meta
	if err := json.Unmarshal(data, &restored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if restored.EnvExtra != nil {
		t.Errorf("EnvExtra = %v, want nil when omitted", restored.EnvExtra)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/ -run "TestMetaEnvExtra" -v`
Expected: FAIL — Meta struct 没有 EnvExtra 字段，编译错误

- [ ] **Step 3: Write minimal implementation**

修改 `daemon/manager.go` 第 77-86 行的 Meta 结构体，新增 EnvExtra 字段：

```go
type Meta struct {
	LogFile     string            `json:"log_file"`
	LogMaxSize  int64             `json:"log_max_size"`
	WorkDir     string            `json:"work_dir"`
	BinaryPath  string            `json:"binary_path"`  // informational only; daemon resolves cc-connect via PATH at runtime
	ConfigFile  string            `json:"config_file,omitempty"`
	EnvPATH     string            `json:"env_path,omitempty"`     // captured user PATH so service can find agent binaries
	HomeDir     string            `json:"home_dir,omitempty"`      // captured user home directory for SYSTEM service
	EnvExtra    map[string]string `json:"env_extra,omitempty"`    // captured env vars (API keys, proxies, etc.)
	InstalledAt string            `json:"installed_at"`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./daemon/ -run "TestMetaEnvExtra" -v`
Expected: PASS

- [ ] **Step 5: Run full daemon package tests**

Run: `go test ./daemon/ -v 2>&1 | tail -30`
Expected: 仅预先存在的 `TestSchtasksInstall_TightensExistingScriptFrom0644` 失败（MINGW 权限问题），其他全部通过

- [ ] **Step 6: Commit**

```bash
git add daemon/manager.go daemon/manager_test.go
git commit -m "feat(daemon): add EnvExtra field to Meta struct for persisting captured env vars"
```

---

## Task 2: SaveMeta 写入 cfg.EnvExtra

**Files:**
- Modify: `cmd/cc-connect/daemon.go:102-112` (SaveMeta call in daemonInstall)

- [ ] **Step 1: Write the failing test**

在 `daemon/manager_test.go` 中添加集成级测试，验证 SaveMeta/LoadMeta 往返包含 EnvExtra：

```go
func TestSaveLoadMetaWithEnvExtra(t *testing.T) {
	dir := t.TempDir()
	// 覆盖 metaPath 指向临时目录
	origMetaPath := metaPath
	metaPath = func() string { return filepath.Join(dir, "daemon.json") }
	defer func() { metaPath = origMetaPath }()

	m := &Meta{
		LogFile:  "test.log",
		WorkDir:  "/tmp/work",
		EnvPATH:  "/usr/bin",
		HomeDir:  "/home/test",
		EnvExtra: map[string]string{"FOO": "bar", "BAZ": "qux"},
		InstalledAt: NowISO(),
	}

	if err := SaveMeta(m); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	loaded, err := LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}

	if len(loaded.EnvExtra) != 2 {
		t.Errorf("EnvExtra len = %d, want 2", len(loaded.EnvExtra))
	}
	if loaded.EnvExtra["FOO"] != "bar" {
		t.Errorf("FOO = %q, want %q", loaded.EnvExtra["FOO"], "bar")
	}
	if loaded.EnvExtra["BAZ"] != "qux" {
		t.Errorf("BAZ = %q, want %q", loaded.EnvExtra["BAZ"], "qux")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./daemon/ -run "TestSaveLoadMetaWithEnvExtra" -v`
Expected: PASS（因为 Task 1 已加了字段，但 cmd 层还没传值）

注意：此测试验证的是 Meta 序列化层，应已通过。真正验证 cmd 层传值需要 Task 3 的端到端测试。

- [ ] **Step 3: Modify SaveMeta call in daemonInstall**

修改 `cmd/cc-connect/daemon.go` 第 102-112 行：

```go
homeDir, _ := os.UserHomeDir()
if err := daemon.SaveMeta(&daemon.Meta{
	LogFile:     filepath.ToSlash(cfg.LogFile),
	LogMaxSize:  cfg.LogMaxSize,
	WorkDir:     filepath.ToSlash(cfg.WorkDir),
	BinaryPath:  filepath.ToSlash(cfg.BinaryPath),
	ConfigFile:  filepath.ToSlash(cfg.ConfigFile),
	EnvPATH:     cfg.EnvPATH,
	HomeDir:     filepath.ToSlash(homeDir),
	EnvExtra:    cfg.EnvExtra,
	InstalledAt: daemon.NowISO(),
}); err != nil {
	fmt.Fprintf(os.Stderr, "Warning: failed to save metadata: %v\n", err)
}
```

- [ ] **Step 4: Run build to verify compilation**

Run: `go build ./cmd/cc-connect`
Expected: 编译成功，无错误

- [ ] **Step 5: Commit**

```bash
git add cmd/cc-connect/daemon.go
git commit -m "feat(daemon): persist cfg.EnvExtra in daemon.json during install"
```

---

## Task 3: stopWithFallback 错误传播修复

**Files:**
- Modify: `cmd/cc-connect/daemon.go:315-343` (stopWithFallback function)
- Test: `cmd/cc-connect/daemon_test.go` (new test file or extend existing)

- [ ] **Step 1: Write the failing test**

在 `cmd/cc-connect/daemon_test.go` 中添加 stopWithFallback 测试：

```go
package main

import (
	"bytes"
	"errors"
	"testing"

	"github.com/chenhg5/cc-connect/daemon"
)

type stubManager struct {
	installed bool
	running   bool
	platform  string
	stopErr   error
}

func (s *stubManager) Platform() string             { return s.platform }
func (s *stubManager) Install(daemon.Config) error  { return nil }
func (s *stubManager) Uninstall() error              { return nil }
func (s *stubManager) Start() error                 { return nil }
func (s *stubManager) Stop() error                  { return s.stopErr }
func (s *stubManager) Restart() error               { return nil }
func (s *stubManager) Status() (*daemon.Status, error) {
	return &daemon.Status{Installed: s.installed, Running: s.running, Platform: s.platform}, nil
}

func TestStopWithFallback_PropagatesPlatformStopError(t *testing.T) {
	stopErr := errors.New("access denied")
	newMgr := func() (daemon.Manager, error) {
		return &stubManager{installed: true, platform: "svc", stopErr: stopErr}, nil
	}
	loadMeta := func() (*daemon.Meta, error) {
		return &daemon.Meta{WorkDir: "/tmp"}, nil
	}
	kill := func(string) bool { return false }

	var stderr bytes.Buffer
	err := stopWithFallback(newMgr, loadMeta, kill, &stderr)
	if err != nil {
		t.Errorf("stopWithFallback returned error: %v", err)
	}
	// stderr 应该包含平台错误信息
	if !bytes.Contains(stderr.Bytes(), []byte("Platform stop reported")) {
		t.Errorf("stderr should contain platform error, got: %q", stderr.String())
	}
}

func TestStopWithFallback_NotInstalled(t *testing.T) {
	newMgr := func() (daemon.Manager, error) {
		return &stubManager{installed: false}, nil
	}
	loadMeta := func() (*daemon.Meta, error) { return nil, nil }
	kill := func(string) bool { return false }

	var stderr bytes.Buffer
	err := stopWithFallback(newMgr, loadMeta, kill, &stderr)
	if err == nil {
		t.Fatal("expected error for not installed")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("not installed")) {
		t.Errorf("error should mention not installed, got: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cc-connect/ -run "TestStopWithFallback" -v`
Expected: FAIL — `stopWithFallback` 吞掉 `mgr.Stop()` 错误，stderr 不含 "Platform stop reported"

- [ ] **Step 3: Modify stopWithFallback**

修改 `cmd/cc-connect/daemon.go` 中的 `stopWithFallback` 函数（第 315-343 行）：

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

	// Report platform stop errors instead of silently discarding them.
	if stopErr := mgr.Stop(); stopErr != nil {
		slog.Warn("daemon stop: platform stop reported error", "platform", st.Platform, "error", stopErr)
		fmt.Fprintf(stderr, "Platform stop reported: %v\n", stopErr)
	}

	meta, merr := loadMeta()
	if merr != nil {
		slog.Warn("daemon stop: could not load metadata, skipping kill verification", "error", merr)
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

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/cc-connect/ -run "TestStopWithFallback" -v`
Expected: PASS

- [ ] **Step 5: Run full cmd package tests**

Run: `go test ./cmd/cc-connect/ -v 2>&1 | tail -20`
Expected: 全部通过

- [ ] **Step 6: Commit**

```bash
git add cmd/cc-connect/daemon.go cmd/cc-connect/daemon_test.go
git commit -m "fix(daemon): propagate mgr.Stop() errors instead of silently discarding them"
```

---

## Task 4: daemonStop 增加 Windows svc 权限预检

**Files:**
- Modify: `cmd/cc-connect/daemon.go:304-313` (daemonStop function)
- Modify: `cmd/cc-connect/daemon_test.go` (add permission check test)

- [ ] **Step 1: Write the failing test**

```go
func TestDaemonStop_SvcRequiresAdmin(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	// 保存原始函数并恢复
	origNewManager := daemonNewManager
	daemonNewManager = func() (daemon.Manager, error) {
		return &stubManager{installed: true, running: true, platform: "svc"}, nil
	}
	origIsAdmin := daemonIsAdmin
	daemonIsAdmin = func() bool { return false }
	defer func() {
		daemonNewManager = origNewManager
		daemonIsAdmin = origIsAdmin
	}()

	// 捕获 os.Exit
	oldExit := osExit
	exitCode := 0
	osExit = func(code int) { exitCode = code }
	defer func() { osExit = oldExit }()

	var stderr bytes.Buffer
	// 重定向 stderr
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = origStderr
		w.Close()
	}()

	daemonStop()

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)

	if exitCode != 1 {
		t.Errorf("exit code = %d, want 1", exitCode)
	}
	if !bytes.Contains(buf.Bytes(), []byte("administrator privileges")) {
		t.Errorf("stderr should mention admin, got: %q", buf.String())
	}
}
```

注意：为使 `daemonStop` 可测试，需要将 `daemon.NewManager` 和 `daemon.IsAdmin` 提取为包级变量：

在 `cmd/cc-connect/daemon.go` 顶部添加：

```go
var (
	daemonNewManager = daemon.NewManager
	daemonIsAdmin    = daemon.IsAdmin
	osExit           = os.Exit
)
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cc-connect/ -run "TestDaemonStop_SvcRequiresAdmin" -v`
Expected: FAIL — daemonStop 不检查权限，exitCode != 1

- [ ] **Step 3: Modify daemonStop**

修改 `cmd/cc-connect/daemon.go` 中的 `daemonStop` 函数（第 304-313 行）：

```go
func daemonStop() {
	if runtime.GOOS == "windows" {
		mgr, err := daemonNewManager()
		if err == nil {
			st, _ := mgr.Status()
			if st != nil && st.Installed && st.Platform == "svc" && !daemonIsAdmin() {
				fmt.Fprintln(os.Stderr, "Error: stopping a Windows Service (svc mode) requires administrator privileges.")
				fmt.Fprintln(os.Stderr, "Run from an elevated terminal (Run as Administrator).")
				osExit(1)
				return
			}
		}
	}
	if err := stopWithFallback(daemonNewManager, daemon.LoadMeta, KillExistingInstance, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		osExit(1)
	}
	fmt.Println("cc-connect daemon stopped.")
	if runtime.GOOS == "windows" {
		fmt.Println("Verify:  tasklist /FI \"IMAGENAME eq cc-connect.exe\"")
	}
}
```

同时将 `daemonStop` 中直接引用的 `daemon.NewManager` 替换为 `daemonNewManager` 变量（为测试可注入性），`daemon.IsAdmin` 替换为 `daemonIsAdmin`，`os.Exit` 替换为 `osExit`。

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/cc-connect/ -run "TestDaemonStop_SvcRequiresAdmin" -v`
Expected: PASS

- [ ] **Step 5: Run full cmd package tests**

Run: `go test ./cmd/cc-connect/ -v 2>&1 | tail -20`
Expected: 全部通过

- [ ] **Step 6: Commit**

```bash
git add cmd/cc-connect/daemon.go cmd/cc-connect/daemon_test.go
git commit -m "fix(daemon): add admin privilege check for stopping Windows Service"
```

---

## Task 5: fixSvcEnvironment 增加诊断文件写入

**Files:**
- Modify: `cmd/cc-connect/svc_run_windows.go:107-188` (fixSvcEnvironment function)
- Test: `cmd/cc-connect/svc_run_windows_test.go` (new test)

- [ ] **Step 1: Write the failing test**

```go
//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFixSvcEnvironmentWritesDiagFile(t *testing.T) {
	// 设置 SYSTEM 账户标志
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", `C:\Windows\System32\config\systemprofile`)
	defer os.Setenv("USERPROFILE", origUserProfile)

	// 创建临时目录模拟 dataDir
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.toml")

	// 写入一个最小的 daemon.json
	metaContent := `{"log_file":"","work_dir":"` + filepath.ToSlash(dir) + `","home_dir":"C:/Users/TestUser","env_path":"C:/bin","installed_at":"2026-01-01T00:00:00Z"}`
	metaPath := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// 修改 os.Args 使 fixSvcEnvironment 能解析 --config
	origArgs := os.Args
	os.Args = []string{"cc-connect.exe", "--config", configFile}
	defer func() { os.Args = origArgs }()

	fixSvcEnvironment()

	// 验证诊断文件已生成
	diagPath := filepath.Join(dir, "svc-env-fix.log")
	data, err := os.ReadFile(diagPath)
	if err != nil {
		t.Fatalf("diagnostic file not found at %s: %v", diagPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "fixSvcEnvironment started") {
		t.Errorf("diagnostic file should contain 'fixSvcEnvironment started', got:\n%s", content)
	}
	if !strings.Contains(content, "fixSvcEnvironment completed") {
		t.Errorf("diagnostic file should contain 'fixSvcEnvironment completed', got:\n%s", content)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/cc-connect/ -run "TestFixSvcEnvironmentWritesDiagFile" -v`
Expected: FAIL — 诊断文件未被创建

- [ ] **Step 3: Modify fixSvcEnvironment**

修改 `cmd/cc-connect/svc_run_windows.go` 中的 `fixSvcEnvironment` 函数。在 SYSTEM 账户检测之后、读取 daemon.json 之前，加入诊断文件初始化；在所有关键步骤使用 `writeDiag` 记录；在函数末尾记录完成状态。

完整替换的函数：

```go
func fixSvcEnvironment() {
	// Only fix when running as SYSTEM (USERPROFILE points to systemprofile).
	up := os.Getenv("USERPROFILE")
	if !strings.Contains(strings.ToLower(up), "system32") &&
		!strings.Contains(strings.ToLower(up), "systemprofile") {
		return
	}

	// Locate daemon.json via --config path or default data dir.
	var configFile string
	for i := 0; i < len(os.Args); i++ {
		if (os.Args[i] == "--config" || os.Args[i] == "-config") && i+1 < len(os.Args) {
			configFile = os.Args[i+1]
			break
		}
		if prefix, ok := strings.CutPrefix(os.Args[i], "--config="); ok {
			configFile = prefix
			break
		}
	}

	var dataDir string
	if configFile != "" {
		dataDir = filepath.Dir(filepath.FromSlash(configFile))
	} else {
		dataDir = filepath.Join(os.Getenv("SystemDrive"), "Users", ".cc-connect")
	}

	// Diagnostic file — visible even when stderr is redirected by SCM.
	diagPath := filepath.Join(dataDir, "svc-env-fix.log")
	diagFile, diagErr := os.OpenFile(diagPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	writeDiag := func(format string, args ...any) {
		if diagFile != nil {
			fmt.Fprintf(diagFile, "[%s] "+format+"\n",
				append([]any{time.Now().Format(time.RFC3339)}, args...)...)
		}
	}
	defer func() {
		if diagFile != nil {
			diagFile.Close()
		}
	}()

	writeDiag("fixSvcEnvironment started")
	writeDiag("USERPROFILE=%s", up)
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

	// Restore home directory from daemon.json (takes precedence over --config derivation).
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

	// Restore log file location from daemon.json.
	logFile := meta.LogFile
	if logFile == "" {
		logFile = filepath.ToSlash(filepath.Join(dataDir, "logs", "cc-connect.log"))
	}
	logFile = filepath.FromSlash(logFile)
	os.Setenv("CC_LOG_FILE", logFile)
	writeDiag("CC_LOG_FILE=%s", logFile)

	// Ensure the log directory exists (SYSTEM has full filesystem access).
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		writeDiag("FAILED to create log dir %s: %v", filepath.Dir(logFile), err)
		fmt.Fprintf(os.Stderr, "svc: failed to create log dir %s: %v\n", filepath.Dir(logFile), err)
	}

	// Restore PATH so agent binaries (claude, codex, etc.) are findable.
	if meta.EnvPATH != "" {
		os.Setenv("PATH", meta.EnvPATH)
		writeDiag("restored PATH, len=%d", len(meta.EnvPATH))
	}

	// Restore captured env vars from install time (API keys, proxy settings, etc.).
	if len(meta.EnvExtra) > 0 {
		for k, v := range meta.EnvExtra {
			os.Setenv(k, v)
		}
		writeDiag("restored EnvExtra, count=%d", len(meta.EnvExtra))
	} else {
		writeDiag("no EnvExtra in daemon.json")
	}

	writeDiag("fixSvcEnvironment completed")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./cmd/cc-connect/ -run "TestFixSvcEnvironmentWritesDiagFile" -v`
Expected: PASS

- [ ] **Step 5: Run full build**

Run: `go build ./...`
Expected: 编译成功

- [ ] **Step 6: Commit**

```bash
git add cmd/cc-connect/svc_run_windows.go cmd/cc-connect/svc_run_windows_test.go
git commit -m "fix(daemon): add diagnostic file output to fixSvcEnvironment for svc mode"
```

---

## Task 6: 集成验证 — 构建和全量测试

**Files:** 无新修改，仅验证

- [ ] **Step 1: Full build**

Run: `go build ./...`
Expected: 编译成功，0 errors

- [ ] **Step 2: Run all tests (excluding known MINGW failure)**

Run: `go test $(go list ./... | grep -v "TestSchtasksInstall_TightensExistingScriptFrom0644") 2>&1 | tail -40`
Expected: 仅已知 MINGW 权限测试失败，其他全部 PASS

- [ ] **Step 3: Verify daemon.json backward compatibility**

手动验证：不含 `env_extra` 字段的旧版 `daemon.json` 仍可被 `LoadMeta()` 正常解析（`omitempty` 保证 nil 而非报错）。

Run: `echo '{"log_file":"test","installed_at":"2026-01-01"}' | go run ./cmd/cc-connect/ daemon status 2>&1 || true`
Expected: 不报 JSON 解析错误（`env_extra` 缺失时 EnvExtra 为 nil，不影响运行）

- [ ] **Step 4: Commit (if any fix needed)**

仅在修复发现的问题时提交。

---

## Task 7: 文档和 spec 状态更新

**Files:**
- Modify: `docs/superpowers/specs/2026-06-19-cc-connect-windows-daemon-bugs-design.md` (status: Draft → Approved)

- [ ] **Step 1: Update spec status**

将 spec 首行状态从 `Draft (v2 — 基于代码审查修正)` 改为 `Approved`

- [ ] **Step 2: Commit**

```bash
git add docs/superpowers/specs/2026-06-19-cc-connect-windows-daemon-bugs-design.md
git commit -m "docs: approve Windows daemon bugs fix spec"
```

---

## 自查清单

### 1. Spec 覆盖率

| Spec 要求 | 对应 Task |
|-----------|----------|
| Meta 新增 EnvExtra 字段 (omitempty) | Task 1 |
| SaveMeta 写入 cfg.EnvExtra | Task 2 |
| stopWithFallback 不再吞错 | Task 3 |
| daemonStop 权限预检 | Task 4 |
| fixSvcEnvironment 诊断文件 | Task 5 |
| fixSvcEnvironment 恢复 EnvExtra | Task 5 |
| 不修改 daemon/windows.go | 全部（PS1 已有渲染） |
| 向后兼容旧版 daemon.json | Task 1 (omitempty) |

### 2. Placeholder 扫描

无 TBD/TODO/未完成步骤。每个 Step 都包含完整代码或命令。

### 3. 类型一致性

- `Meta.EnvExtra` 类型：`map[string]string` — Task 1 定义，Task 2 写入，Task 5 读取，全部一致
- `daemon.IsAdmin()` 返回 `bool` — Task 4 引用，与 `svcmanager_windows.go:221` 定义一致
- `daemon.Manager` 接口方法：`Status() (*Status, error)` / `Stop() error` — Task 3 stub 实现与接口一致
- `writeDiag` 辅助函数签名：`func(format string, args ...any)` — 内部使用 `fmt.Fprintf`，与 Go 惯例一致
