APP        := cc-connect
MODULE     := github.com/chenhg5/cc-connect
CMD        := ./cmd/cc-connect
DIST       := dist

VERSION := v1.3.3
COMMIT     := $(shell git rev-parse --short HEAD 2>/dev/null || echo "none")
BUILD_TIME := $(shell date -u '+%Y-%m-%dT%H:%M:%SZ')

LDFLAGS := -s -w \
  -X main.version=$(VERSION) \
  -X main.commit=$(COMMIT) \
  -X main.buildTime=$(BUILD_TIME)

PLATFORMS := \
  linux/amd64 \
  linux/arm64 \
  darwin/amd64 \
  darwin/arm64 \
  windows/amd64 \
  windows/arm64

# ---------------------------------------------------------------------------
# Selective compilation via build tags.
#
# By default all agents and platforms are included. To build with only
# specific ones, set AGENTS and/or PLATFORMS_INCLUDE:
#
#   make build AGENTS=claudecode PLATFORMS_INCLUDE=feishu,telegram
#
# You can also exclude specific ones:
#
#   make build EXCLUDE=discord,dingtalk,qq,qqbot,line
# ---------------------------------------------------------------------------

ALL_AGENTS    := acp antigravity claudecode codex copilot cursor devin gemini iflow kimi opencode pi qoder tmux
ALL_PLATFORMS := feishu telegram discord slack dingtalk wecom weixin qq qqbot line weibo max matrix
ALL_EXTRAS    := web

COMMA := ,

# Compute exclusion tags from AGENTS / PLATFORMS_INCLUDE / EXCLUDE variables
_EXCLUDE_TAGS :=

ifdef AGENTS
  _WANTED_AGENTS := $(subst $(COMMA), ,$(AGENTS))
  _EXCLUDE_AGENTS := $(filter-out $(_WANTED_AGENTS),$(ALL_AGENTS))
  _EXCLUDE_TAGS += $(addprefix no_,$(_EXCLUDE_AGENTS))
endif

ifdef PLATFORMS_INCLUDE
  _WANTED_PLATFORMS := $(subst $(COMMA), ,$(PLATFORMS_INCLUDE))
  _EXCLUDE_PLATFORMS := $(filter-out $(_WANTED_PLATFORMS),$(ALL_PLATFORMS))
  _EXCLUDE_TAGS += $(addprefix no_,$(_EXCLUDE_PLATFORMS))
endif

ifdef EXCLUDE
  _EXCLUDE_TAGS += $(addprefix no_,$(subst $(COMMA), ,$(EXCLUDE)))
endif

ifdef NO_WEB
  _EXCLUDE_TAGS += no_web
endif

_BUILD_TAGS := $(strip $(_EXCLUDE_TAGS) goolm)
_TAGS_FLAG  := $(if $(_BUILD_TAGS),-tags '$(_BUILD_TAGS)',)

.PHONY: build run clean test test-fast test-full test-smoke test-e2e test-release test-release-local test-performance pre-test lint release release-noweb release-all web rebeta

web:
	@if [ ! -d web/node_modules ]; then cd web && npm install; fi
	cd web && npm run build

build: web
	go build $(_TAGS_FLAG) -ldflags "$(LDFLAGS)" -o $(APP) $(CMD)

build-noweb:
	go build $(_TAGS_FLAG) -tags 'no_web' -ldflags "$(LDFLAGS)" -o $(APP) $(CMD)

run: build
	./$(APP)

clean:
	rm -f $(APP)
	rm -rf $(DIST)

# ---------------------------------------------------------------------------
# Testing targets.
#
# test-fast:  Unit tests + smoke tests (< 2 min). Runs on every push.
# test-full:   Full test suite including regression (< 10 min). PR requirement.
# test-smoke:  Smoke tests only (< 1 min). Quick sanity check.
# test-e2e:    E2E and regression tests only.
# test-release: Full + performance benchmarks. Before release.
# pre-test:    Prerequisites (build + vet) before running tests.
# ---------------------------------------------------------------------------

pre-test:
	go build ./...
	go vet ./...

# Fast test: unit tests + smoke tests
test-fast: pre-test
	go test -parallel=4 -race ./...
	go test -parallel=4 -tags=smoke ./tests/e2e/...

# Full test: unit + smoke + regression (PR requirement)
test-full: pre-test
	go test -parallel=4 -race ./...
	go test -parallel=4 -tags=smoke ./tests/e2e/...
	go test -parallel=2 -tags=regression ./tests/e2e/...

# Smoke tests only
test-smoke: pre-test
	go test -v -tags=smoke ./tests/e2e/...

# E2E/regression tests only
test-e2e: pre-test
	go test -v -tags=regression ./tests/e2e/...

# Performance benchmarks only
test-performance: pre-test
	go test -bench=. -benchmem -tags=performance ./tests/performance/...

# Release test: full + performance benchmarks
test-release: pre-test
	go test -parallel=4 -race ./...
	go test -parallel=4 -tags=smoke ./tests/e2e/...
	go test -parallel=2 -tags=regression ./tests/e2e/...
	go test -bench=. -benchmem -tags=performance ./tests/performance/...

# Release-local gate: deterministic release checks that do not require real IM
# credentials, real provider accounts, or supervisor-managed services.
test-release-local:
	go test ./tests/release_local/...
	go test ./config
	go test ./core -run 'TestEngineSendToSessionWithAttachments|TestProcessInteractiveEvents_SuppressesDuplicateSideChannelText|TestCmdList_AllSessionsVisibleAfterRepeatedNew|TestCmdList_SessionVisibleDuringAgentProcessing|TestEngine_Alias|TestEngine_BannedWords|TestEngine_DisabledCommands'
	go test ./platform/feishu -run 'TestUserIDFromEventFallsBackToUserID|TestResolveUserNameSkipsInvalidLookupID|TestNew_CanDisableInteractiveCards'

# Legacy: runs unit tests only
test:
	go test -v ./...

lint:
	golangci-lint run ./...

release-all: web clean
	@mkdir -p $(DIST)
	@$(foreach platform,$(PLATFORMS), \
		$(eval GOOS   := $(word 1,$(subst /, ,$(platform)))) \
		$(eval GOARCH := $(word 2,$(subst /, ,$(platform)))) \
		$(eval EXT    := $(if $(filter windows,$(GOOS)),.exe,)) \
		$(eval OUT    := $(DIST)/$(APP)-$(VERSION)-$(GOOS)-$(GOARCH)$(EXT)) \
		echo "Building $(OUT)" && \
		GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
			go build $(_TAGS_FLAG) -ldflags "$(LDFLAGS)" -o $(OUT) $(CMD) && \
	) true
	@echo "Packaging archives..."
	@cd $(DIST) && for f in $(APP)-*; do \
		case "$$f" in \
			*.tar.gz|*.zip) continue ;; \
			*.exe) zip "$${f%.exe}.zip" "$$f" ;; \
			*)     tar czf "$$f.tar.gz" "$$f" ;; \
		esac; \
	done
	@cd $(DIST) && sha256sum * > checksums.txt
	@echo "Done. Binaries and archives in $(DIST)/"

release:
	@if [ -z "$(TARGET)" ]; then \
		echo "Usage: make release TARGET=linux/amd64"; \
		echo "Available: $(PLATFORMS)"; \
		exit 1; \
	fi
	@mkdir -p $(DIST)
	$(eval GOOS   := $(word 1,$(subst /, ,$(TARGET))))
	$(eval GOARCH := $(word 2,$(subst /, ,$(TARGET))))
	$(eval EXT    := $(if $(filter windows,$(GOOS)),.exe,))
	$(eval OUT    := $(DIST)/$(APP)-$(VERSION)-$(GOOS)-$(GOARCH)$(EXT))
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build $(_TAGS_FLAG) -ldflags "$(LDFLAGS)" -o $(OUT) $(CMD)
	@echo "Built: $(OUT)"

release-noweb:
	@if [ -z "$(TARGET)" ]; then \
		echo "Usage: make release-noweb TARGET=windows/amd64"; \
		echo "Available: $(PLATFORMS)"; \
		exit 1; \
	fi
	@mkdir -p $(DIST)
	$(eval GOOS   := $(word 1,$(subst /, ,$(TARGET))))
	$(eval GOARCH := $(word 2,$(subst /, ,$(TARGET))))
	$(eval EXT    := $(if $(filter windows,$(GOOS)),.exe,))
	$(eval OUT    := $(DIST)/$(APP)-$(VERSION)-$(GOOS)-$(GOARCH)$(EXT))
	GOOS=$(GOOS) GOARCH=$(GOARCH) CGO_ENABLED=0 \
		go build -tags 'no_web' -ldflags "$(LDFLAGS)" -o $(OUT) $(CMD)
	@echo "Built: $(OUT)"

# Re-release a beta version (delete remote tag + npm, re-push to trigger CI)
# Usage:
#   make rebeta                     # re-release v1.3.3-beta (default)
#   make rebeta REBETA_VERSION=v1.3.3-beta.4
#   make rebeta REBETA_REMOTE=origin   # override auto-detected remote
#   make rebeta DRY_RUN=1           # preview only
REBETA_VER := $(or $(REBETA_VERSION),v1.3.3-beta)
REBETA_REMOTE ?= $(shell git remote get-url fork &>/dev/null && echo fork || echo origin)
REBETA_NPM_VER := $(REBETA_VER:v%=%)
REBETA_REPO := game1991/cc-connect

rebeta:
	@echo "=== cc-connect Rebeta ==="
	@echo "  Tag:     $(REBETA_VER)"
	@echo "  NPM ver: $(REBETA_NPM_VER)"
	@echo "  Remote:  $(REBETA_REMOTE)"
	@echo ""
ifdef DRY_RUN
	@echo "[DRY RUN] The following steps would be executed:"
	@echo "  1. git push $(REBETA_REMOTE) main"
	@echo "  2. git push $(REBETA_REMOTE) :refs/tags/$(REBETA_VER)"
	@echo "  3. gh api //user/packages/npm/cc-connect/versions --jq ..."
	@echo "     (double-slash: MINGW rewrites /path to filesystem path)"
	@echo "  4. gh api --method DELETE //user/packages/npm/cc-connect/versions/<ID>"
	@echo "  5. git tag -f $(REBETA_VER) && git push $(REBETA_REMOTE) $(REBETA_VER)"
	@echo "  6. Wait for CI, then verify npm version"
	@echo ""
	@echo "=== Dry Run Complete ==="
else
	@echo "[1/6] Checking working tree..."
	@test -z "$$(git status --porcelain)" || (echo "ERROR: working tree not clean" && exit 1)
	@echo "  OK"
	@if git rev-parse $(REBETA_VER) &>/dev/null; then \
		TAG_COMMIT=$$(git rev-parse $(REBETA_VER)); \
		HEAD_COMMIT=$$(git rev-parse HEAD); \
		if [ "$$TAG_COMMIT" != "$$HEAD_COMMIT" ]; then \
			echo "  WARN: tag $(REBETA_VER) at $$TAG_COMMIT, moving to HEAD $$HEAD_COMMIT"; \
		fi; \
	else \
		echo "  WARN: local tag $(REBETA_VER) does not exist, will be created at HEAD"; \
	fi
	@echo "[2/6] Pushing main to $(REBETA_REMOTE)..."
	@git push $(REBETA_REMOTE) main
	@echo "[3/6] Deleting remote tag $(REBETA_VER)..."
	@git push $(REBETA_REMOTE) :refs/tags/$(REBETA_VER) || true
	@echo "[4/6] Deleting npm version $(REBETA_NPM_VER)..."
	@NPM_ID=$$(MSYS_NO_PATHCONV=1 gh api /user/packages/npm/cc-connect/versions --jq ".[] | select(.name==\"$(REBETA_NPM_VER)\") | .id" 2>/dev/null || echo ""); \
	if [ -n "$$NPM_ID" ]; then \
		echo "  Found npm version ID: $$NPM_ID"; \
		MSYS_NO_PATHCONV=1 gh api --method DELETE "/user/packages/npm/cc-connect/versions/$$NPM_ID"; \
	else \
		echo "  No npm version $(REBETA_NPM_VER) found. Skipping."; \
	fi
	@echo "[5/6] Re-pushing tag $(REBETA_VER)..."
	@git tag -f $(REBETA_VER)
	@git push $(REBETA_REMOTE) $(REBETA_VER)
	@echo "[6/6] Waiting for CI and verifying..."
	@HEAD_SHA=$$(git rev-parse HEAD); \
	echo "  Waiting for GitHub to create new run..."; \
	sleep 5; \
	for i in 1 2 3 4; do \
		RUN_ID=$$(gh run list --repo $(REBETA_REPO) --branch $(REBETA_VER) --limit 5 --json databaseId,headSha --jq ".[] | select(.headSha==\"$$HEAD_SHA\") | .databaseId" 2>/dev/null | head -1); \
		if [ -n "$$RUN_ID" ]; then break; fi; \
		echo "  Retry ($$i/4)..."; sleep 5; \
	done; \
	if [ -z "$$RUN_ID" ]; then \
		echo "  WARN: could not find CI run for $$HEAD_SHA. Check:"; \
		echo "    gh run list --repo $(REBETA_REPO) --limit 5"; \
	else \
		echo "  CI run:  $$RUN_ID (watching...)"; \
		gh run watch $$RUN_ID --repo $(REBETA_REPO) --exit-status > /dev/null 2>&1; \
		STATUS=$$(gh run view $$RUN_ID --repo $(REBETA_REPO) --json conclusion --jq '.conclusion' 2>/dev/null || echo "unknown"); \
		if [ "$$STATUS" = "success" ]; then \
			echo "  CI:      SUCCESS"; \
		else \
			echo "  CI:      $$STATUS"; \
			echo "  Details: gh run view $$RUN_ID --repo $(REBETA_REPO)"; \
		fi; \
	fi
	@HEAD_SHORT=$$(git rev-parse --short HEAD); \
	HEAD_FULL=$$(git rev-parse HEAD); \
	for i in 1 2 3 4 5; do \
		NPM_CREATED=$$(MSYS_NO_PATHCONV=1 gh api /user/packages/npm/cc-connect/versions --jq ".[] | select(.name==\"$(REBETA_NPM_VER)\") | .created_at" 2>/dev/null || echo ""); \
		if [ -n "$$NPM_CREATED" ]; then break; fi; \
		echo "  NPM not live yet, retry ($$i/5)..."; sleep 5; \
	done; \
	echo ""; \
	echo "=== Release Verification ==="; \
	echo "  Local HEAD:  $$HEAD_SHORT ($$HEAD_FULL)"; \
	if [ -n "$$NPM_CREATED" ]; then \
		echo "  NPM version: $(REBETA_NPM_VER)"; \
		echo "  NPM created: $$NPM_CREATED"; \
		echo "  Status:      MATCHED"; \
	else \
		echo "  NPM version: $(REBETA_NPM_VER)"; \
		echo "  Status:      NOT FOUND (may need a moment to propagate)"; \
	fi; \
	echo ""
endif
