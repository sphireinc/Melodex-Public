SHELL := /bin/bash

.DEFAULT_GOAL := help

# Melodex toolchain defaults. Override any of these at invocation time, for
# example: `make NODE_VERSION=24 frontend-test` or
# `make PLATFORM=windows/amd64 platform-build`.
APP_NAME ?= Melodex
NODE_VERSION ?= 24
NODE_MAJOR ?= 24
NVM_DIR ?= $(HOME)/.nvm
WAILS_VERSION ?= v3.0.0-beta.2
WAILS_BIN ?= $(shell command -v wails3 2>/dev/null || printf '%s/bin/wails3' "$(shell go env GOPATH 2>/dev/null)")
GO_VERSION ?= 1.26.1
GO_CACHE ?= $(CURDIR)/.melodex/go-build-cache
GO_COVERAGE ?= $(CURDIR)/.melodex/coverage.out
ARTIFACT_MANIFEST ?= $(CURDIR)/bin/artifact-manifest.txt
CHECKSUM_FILE ?= $(CURDIR)/bin/SHA256SUMS
VITE_PORT ?= 9245
VISUAL_WORKERS ?= 1
VISUAL_ARGS ?=
PERFORMANCE_ARGS ?=
VERSION ?= 0.0.0
RELEASE_CHANNEL ?= dev
BUILD_NUMBER ?= 0
SMOKE_SECONDS ?= 20
GIT_COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || printf 'dev')
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
APP_LDFLAGS ?= -X main.AppVersion=$(VERSION) -X main.BuildNumber=$(BUILD_NUMBER) -X main.GitCommit=$(GIT_COMMIT) -X main.BuildTime=$(BUILD_TIME) -X main.ReleaseChannel=$(RELEASE_CHANNEL)
ARCH ?= $(shell go env GOARCH 2>/dev/null || printf 'arm64')
PLATFORM ?= darwin/$(ARCH)
CROSS_IMAGE ?= wails-cross
CROSS_IMAGE_AMD64 ?= wails-cross-amd64
SERVER_IMAGE ?= melodex-server:latest
PACKAGE_FORMAT ?= nsis
INSTALL_SCOPE ?= machine

# Every frontend/Wails recipe selects the requested nvm version in the same
# shell that runs the command. This avoids relying on the interactive shell's
# current Node version and keeps Makefile, scripts, and CI behavior aligned.
NODE_SETUP = source "$(NVM_DIR)/nvm.sh" && nvm use "$(NODE_VERSION)" >/dev/null &&
BASE_BUILD_ENV = GOCACHE="$(GO_CACHE)" MELODEX_RELEASE_CHANNEL="$(RELEASE_CHANNEL)" MELODEX_VERSION="$(VERSION)"
# Wails Taskfiles call the CLI recursively by its conventional `wails3` name.
# Keep the resolved binary directory in PATH even when the host shell only has
# the absolute GOPATH/bin fallback available.
WAILS_PATH = $(dir $(WAILS_BIN))
BUILD_ENV = PATH="$(WAILS_PATH):$${PATH}" $(BASE_BUILD_ENV)
WAILS_ENV = PATH="$(WAILS_PATH):$${PATH}" GOCACHE="$(GO_CACHE)" APP_LDFLAGS="$(APP_LDFLAGS)"

.PHONY: help info toolchain-check node-check go-check wails-check wails-version \
  install bootstrap frontend-install frontend-format frontend-typecheck \
  frontend-ready \
  frontend-typecheck-strict frontend-test frontend-lint frontend-audit \
  frontend-build frontend-preview frontend-visual-install frontend-visual-test \
  frontend-visual-update frontend-performance \
  legal-check \
  bindings assets format gofmt go-mod-tidy backend-test backend-test-race \
  backend-coverage backend-vet backend-build backend-vuln \
  backend-audio-real backend-recovery-process backend-event-test backend-event-benchmark backend-diagnostics backend-security-portable backend-security-paths evidence-check \
  test test-race check ci doctor dev dev-frontend wails-dev launch \
  build build-dev platform-build macos-build macos-app macos-app-verify macos-smoke macos-universal macos-universal-verify \
  macos-dmg macos-dmg-verify macos-run macos-sign macos-notarize windows-build windows-app \
  macos-zip macos-universal-zip checksums artifact-manifest \
  windows-tools-check windows-installer linux-build linux-packages linux-deb linux-rpm \
  linux-appimage linux-packages-amd64 linux-deb-amd64 linux-rpm-amd64 linux-appimage-amd64 \
  linux-sign-deb linux-sign-rpm linux-sign-packages linux-package-verify \
  docker-setup-amd64 linux-build-amd64 \
  ios-build ios-package ios-package-device android-tools-check android-build android-package \
  wails-install docker-setup docker-server-build release-check release-macos \
  artifacts clean clean-frontend clean-cache clean-build require-darwin require-docker

help: ## Show the complete command catalog
	@awk 'BEGIN {FS = ":.*##"; printf "Melodex build commands (Wails %s, Node %s)\n\n", "$(WAILS_VERSION)", "$(NODE_VERSION)"} /^[a-zA-Z0-9_.-]+:.*##/ {printf "  %-28s %s\n", $$1, $$2} END {printf "\nExamples:\n  make install\n  make dev\n  make macos-app\n  make macos-universal\n  make ci\n  make PLATFORM=linux/amd64 platform-build\n\nVariable overrides:\n  NODE_VERSION=24 VITE_PORT=9245 VERSION=1.2.3 RELEASE_CHANNEL=stable\n  VISUAL_WORKERS=1 VISUAL_ARGS=\"--grep video\" SMOKE_SECONDS=30\n"}' $(MAKEFILE_LIST)

info: ## Print the resolved toolchain and build variables
	@printf 'APP_NAME=%s\n' '$(APP_NAME)'
	@printf 'NODE_VERSION=%s\n' '$(NODE_VERSION)'
	@printf 'NODE_MAJOR=%s\n' '$(NODE_MAJOR)'
	@printf 'NVM_DIR=%s\n' '$(NVM_DIR)'
	@printf 'WAILS_VERSION=%s\n' '$(WAILS_VERSION)'
	@printf 'WAILS_BIN=%s\n' '$(WAILS_BIN)'
	@printf 'GO_VERSION=%s\n' '$(GO_VERSION)'
	@printf 'GO_CACHE=%s\n' '$(GO_CACHE)'
	@printf 'GO_COVERAGE=%s\n' '$(GO_COVERAGE)'
	@printf 'ARTIFACT_MANIFEST=%s\n' '$(ARTIFACT_MANIFEST)'
	@printf 'CHECKSUM_FILE=%s\n' '$(CHECKSUM_FILE)'
	@printf 'VITE_PORT=%s\n' '$(VITE_PORT)'
	@printf 'VISUAL_WORKERS=%s\n' '$(VISUAL_WORKERS)'
	@printf 'VISUAL_ARGS=%s\n' '$(VISUAL_ARGS)'
	@printf 'PERFORMANCE_ARGS=%s\n' '$(PERFORMANCE_ARGS)'
	@printf 'VERSION=%s\n' '$(VERSION)'
	@printf 'RELEASE_CHANNEL=%s\n' '$(RELEASE_CHANNEL)'
	@printf 'BUILD_NUMBER=%s\n' '$(BUILD_NUMBER)'
	@printf 'SMOKE_SECONDS=%s\n' '$(SMOKE_SECONDS)'
	@printf 'GIT_COMMIT=%s\n' '$(GIT_COMMIT)'
	@printf 'BUILD_TIME=%s\n' '$(BUILD_TIME)'
	@printf 'PLATFORM=%s\n' '$(PLATFORM)'
	@printf 'ARCH=%s\n' '$(ARCH)'
	@printf 'CROSS_IMAGE=%s\n' '$(CROSS_IMAGE)'
	@printf 'CROSS_IMAGE_AMD64=%s\n' '$(CROSS_IMAGE_AMD64)'

toolchain-check: node-check go-check wails-check ## Verify Node, Go, Wails, npm, and required versions

node-check: ## Select Node through nvm and verify the configured Node major version
	@$(NODE_SETUP) node --version
	@$(NODE_SETUP) npm --version
	@$(NODE_SETUP) node -e 'const major = Number(process.versions.node.split(".")[0]); const expected = Number("$(NODE_MAJOR)"); if (major !== expected) { console.error(`Node $${expected}.x is required; found $${process.versions.node}`); process.exit(1); }'

go-check: ## Verify the Go toolchain declared by go.mod is available
	@command -v go >/dev/null || { echo 'Go is required' >&2; exit 1; }
	@go version
	@go version | awk '{print $$3}' | grep -Fx 'go$(GO_VERSION)' >/dev/null || { echo "Go $(GO_VERSION) is required by go.mod" >&2; exit 1; }
	@go env GOOS GOARCH GOPATH

wails-check: wails-version ## Verify the pinned Wails 3 CLI is installed

wails-version: ## Print the installed Wails CLI version
	@test -x "$(WAILS_BIN)" || command -v "$(WAILS_BIN)" >/dev/null || { echo "wails3 not found at $(WAILS_BIN); run make wails-install" >&2; exit 1; }
	@actual="$$($(WAILS_ENV) "$(WAILS_BIN)" version 2>&1)"; printf '%s\n' "$$actual"; test "$$actual" = "$(WAILS_VERSION)" || { echo "Wails $(WAILS_VERSION) is required; found $$actual" >&2; exit 1; }

wails-install: ## Install the pinned Wails 3 CLI into GOPATH/bin
	@GOBIN="$$(go env GOPATH)/bin" go install github.com/wailsapp/wails/v3/cmd/wails3@$(WAILS_VERSION)

bootstrap: ## Run the project installer and all prerequisite checks
	@$(NODE_SETUP) ./install.sh

install: bootstrap ## Alias for bootstrap

frontend-install: node-check ## Install frontend dependencies from package-lock.json
	@$(NODE_SETUP) npm ci --prefix frontend

frontend-ready: node-check ## Install frontend dependencies automatically when missing
	@test -d frontend/node_modules || { echo '[melodex] frontend dependencies are missing; running npm ci' >&2; $(NODE_SETUP) npm ci --prefix frontend; }

frontend-format: frontend-ready ## Format and autofix frontend source files
	@$(NODE_SETUP) npm run format --prefix frontend

frontend-typecheck: frontend-ready ## Run the normal TypeScript typecheck
	@$(NODE_SETUP) npm run typecheck --prefix frontend

frontend-typecheck-strict: frontend-ready ## Run the strict TypeScript module-resolution check
	@$(NODE_SETUP) npm run typecheck:strict --prefix frontend

frontend-test: frontend-ready ## Run frontend unit/helper and lifecycle tests
	@$(NODE_SETUP) npm test --prefix frontend

frontend-lint: frontend-ready ## Run the frontend ESLint check
	@$(NODE_SETUP) npm run lint --prefix frontend

frontend-audit: frontend-ready ## Audit frontend dependencies for known vulnerabilities
	@$(NODE_SETUP) npm audit --prefix frontend

frontend-build: frontend-ready ## Build the production Vite frontend
	@$(NODE_SETUP) npm run build --prefix frontend

frontend-preview: frontend-ready ## Serve the production frontend preview on VITE_PORT
	@$(NODE_SETUP) npm run preview --prefix frontend -- --port "$(VITE_PORT)" --strictPort

legal-check: node-check ## Verify project legal files and direct dependency notices
	@$(NODE_SETUP) node scripts/check-third-party-notices.mjs

frontend-visual-install: frontend-ready ## Install Playwright browsers used by visual tests
	@$(NODE_SETUP) npx --prefix frontend playwright install

frontend-visual-test: frontend-ready ## Run deterministic Playwright visual/regression checks
	@$(NODE_SETUP) npm run test:visual --prefix frontend -- --workers="$(VISUAL_WORKERS)" $(VISUAL_ARGS)

frontend-visual-update: frontend-ready ## Regenerate Playwright visual baselines intentionally
	@$(NODE_SETUP) npm run test:visual --prefix frontend -- --workers="$(VISUAL_WORKERS)" --update-snapshots $(VISUAL_ARGS)

frontend-performance: frontend-ready ## Run the sanitized 10,000-track browser performance smoke test
	@$(NODE_SETUP) npm run test:performance --prefix frontend -- $(PERFORMANCE_ARGS)

bindings: node-check wails-check ## Regenerate Wails 3 TypeScript bindings
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task common:generate:bindings

assets: node-check wails-check ## Regenerate Wails platform metadata and application icons
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task common:update:build-assets
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task common:generate:icons

format: gofmt frontend-format ## Format Go and frontend sources

gofmt: ## Format all Go packages
	@go fmt ./...

go-mod-tidy: go-check ## Reconcile go.mod and go.sum with the source tree
	@GOCACHE="$(GO_CACHE)" go mod tidy

backend-test: ## Run all Go backend tests with a repository-local cache
	@GOCACHE="$(GO_CACHE)" go test ./...

backend-test-race: ## Run all Go backend tests with the race detector
	@GOCACHE="$(GO_CACHE)" go test -race ./...

backend-coverage: ## Write a Go coverage profile and print its function summary
	@mkdir -p "$(dir $(GO_COVERAGE))"
	@GOCACHE="$(GO_CACHE)" go test -coverprofile="$(GO_COVERAGE)" ./...
	@GOCACHE="$(GO_CACHE)" go tool cover -func="$(GO_COVERAGE)"

backend-vet: ## Run go vet with a repository-local cache
	@GOCACHE="$(GO_CACHE)" go vet ./...

backend-build: ## Compile every Go package without producing a release artifact
	@GOCACHE="$(GO_CACHE)" go build ./...

backend-vuln: ## Run govulncheck when the optional Go vulnerability tool is installed
	@command -v govulncheck >/dev/null || { echo 'govulncheck is not installed; install golang.org/x/vuln/cmd/govulncheck first' >&2; exit 1; }
	@GOCACHE="$(GO_CACHE)" govulncheck ./...

backend-audio-real: ## Run audio normalization, probe metadata, and malformed-staged-audio rejection coverage
	@GOCACHE="$(GO_CACHE)" go test . -run 'TestParseAudioProbeJSON|TestProbeAudioFile|TestImportFinalizationRejectsMalformedStagedAudio|TestLocalAudioNormalizationProducesDecodableMP3$$' -count=1

backend-recovery-process: ## Run subprocess crash-boundary import recovery coverage
	@GOCACHE="$(GO_CACHE)" go test . -run '^TestImportFinalizationReplayProcessBoundary$$' -count=1

backend-event-test: ## Run deterministic parent/child event count and payload instrumentation coverage
	@GOCACHE="$(GO_CACHE)" go test . -run '^TestSyntheticParentChildJobProgressEventInstrumentation$$' -count=1

backend-event-benchmark: ## Run the machine-independent synthetic event payload benchmark once
	@GOCACHE="$(GO_CACHE)" go test -run '^$$' -bench '^BenchmarkSyntheticParentChildJobProgressEvents$$' -benchtime=1x -count=1 .

backend-diagnostics: ## Inspect a malformed-catalog diagnostics archive and verify log redaction
	@GOCACHE="$(GO_CACHE)" go test . -run '^TestDiagnosticsBundleKeepsMalformedCatalogUsableAndRedactsRecentLog$$' -count=1

backend-security-paths: ## Run traversal, allowed-root, and symlink-escape security checks
	@GOCACHE="$(GO_CACHE)" go test . -run 'TestValidatePathWithinRoots|TestMediaServer.*Symlink' -count=1

backend-security-portable: ## Run pure path/symlink checks without Wails or CGO dependencies
	@GOCACHE="$(GO_CACHE)" go test ./internal/pathsecurity -count=1

evidence-check: backend-audio-real backend-recovery-process backend-event-test backend-event-benchmark backend-diagnostics backend-security-portable backend-security-paths ## Run the local acceptance-evidence checks

test: backend-test frontend-test ## Run backend and frontend tests

test-race: backend-test-race frontend-test ## Run frontend tests and race-enabled backend tests

check: legal-check backend-test backend-vet backend-build frontend-typecheck frontend-typecheck-strict frontend-test frontend-lint frontend-build ## Run the normal migration/release quality gate

ci: check frontend-audit ## Run the CI-equivalent local quality gate

doctor: wails-check ## Run Wails diagnostics (may report host-specific warnings)
	@$(WAILS_ENV) "$(WAILS_BIN)" doctor

dev-frontend: node-check ## Start Vite on the port expected by Wails dev mode
	@$(NODE_SETUP) VITE_PORT="$(VITE_PORT)" $(WAILS_ENV) "$(WAILS_BIN)" task common:dev:frontend

wails-dev: node-check ## Start Wails dev mode using build/config.yml
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" dev -config ./build/config.yml -port $(VITE_PORT)

dev: node-check ## Run the full repository development workflow
	@$(NODE_SETUP) VITE_PORT="$(VITE_PORT)" ./dev.sh

launch: node-check ## Build and launch the available desktop artifact
	@$(NODE_SETUP) ./launch.sh

require-darwin: ## Fail clearly when a macOS-only target is run elsewhere
	@test "$$(uname -s)" = Darwin || { echo 'This target requires macOS (Darwin).' >&2; exit 1; }

require-docker: ## Verify that Docker is installed and its daemon is available
	@command -v docker >/dev/null || { echo 'Docker is required for this target.' >&2; exit 1; }
	@docker info >/dev/null 2>&1 || { echo 'Docker is installed but the daemon is not available.' >&2; exit 1; }

build: node-check ## Build the current platform's Wails production binary
	@$(NODE_SETUP) $(BUILD_ENV) "$(WAILS_BIN)" build

build-dev: node-check ## Build the current platform with Wails development flags
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task $(shell uname -s | tr '[:upper:]' '[:lower:]'):build DEV=true

platform-build: node-check ## Build PLATFORM=darwin/arm64, windows/amd64, or linux/amd64
	@$(NODE_SETUP) $(BUILD_ENV) MELODEX_BUILD_PLATFORM="$(PLATFORM)" ./build.sh

macos-build: node-check wails-check require-darwin ## Build a native macOS arm64 production binary
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task darwin:build ARCH=arm64

macos-app: node-check wails-check require-darwin ## Build a standalone signed-ad-hoc macOS .app bundle
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task darwin:package
	@test -d "bin/$(APP_NAME).app" || { echo "macOS app bundle was not created" >&2; exit 1; }
	@printf 'Standalone app: bin/%s.app\n' '$(APP_NAME)'

macos-app-verify: macos-app ## Verify the standalone app bundle and its ad-hoc signature
	@codesign --verify --deep --strict "bin/$(APP_NAME).app"
	@file "bin/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)"
	@printf 'Verified standalone app: bin/%s.app\n' '$(APP_NAME)'

macos-smoke: macos-app ## Launch the standalone app, verify process liveness, and terminate only the new instance
	@test "$$(uname -s)" = Darwin || { echo 'macos-smoke requires macOS' >&2; exit 1; }
	@set -eu; \
	app="$(CURDIR)/bin/$(APP_NAME).app"; \
	pattern="$$app/Contents/MacOS/$(APP_NAME)"; \
	before="$$(pgrep -f "$$pattern" || true)"; \
	open -n -g "$$app"; \
	new_pid=""; \
	for attempt in 1 2 3 4 5 6 7 8 9 10; do \
		after="$$(pgrep -f "$$pattern" || true)"; \
		for pid in $$after; do \
			case " $$before " in *" $$pid "*) ;; *) new_pid="$$pid"; break ;; esac; \
		done; \
		[ -n "$$new_pid" ] && break; \
		sleep 1; \
	done; \
	[ -n "$$new_pid" ] || { echo 'standalone macOS app did not produce a new process' >&2; exit 1; }; \
	echo "standalone macOS app process $$new_pid remained alive for $(SMOKE_SECONDS)s"; \
	sleep "$(SMOKE_SECONDS)"; \
	kill -TERM "$$new_pid" 2>/dev/null || true; \
	for attempt in 1 2 3 4 5; do \
		kill -0 "$$new_pid" 2>/dev/null || exit 0; \
		sleep 1; \
	done; \
	kill -KILL "$$new_pid" 2>/dev/null || true

macos-universal: node-check wails-check require-darwin ## Build a standalone universal macOS .app bundle
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task darwin:package:universal
	@printf 'Standalone universal app: bin/%s.app\n' '$(APP_NAME)'

macos-universal-verify: macos-universal ## Verify universal app bundle, signature, and both CPU architectures
	@test -d "bin/$(APP_NAME).app" || { echo "universal macOS app bundle was not created" >&2; exit 1; }
	@codesign --verify --deep --strict "bin/$(APP_NAME).app"
	@lipo -info "bin/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)"
	@lipo -info "bin/$(APP_NAME).app/Contents/MacOS/$(APP_NAME)" | grep -Eq 'arm64.*x86_64|x86_64.*arm64' || { echo "universal app does not contain arm64 and x86_64" >&2; exit 1; }
	@printf 'Verified universal standalone app: bin/%s.app\n' '$(APP_NAME)'

macos-dmg: node-check wails-check require-darwin ## Build a standalone macOS .app and styled .dmg
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task darwin:package:dmg
	@printf 'DMG output directory: bin\n'

macos-dmg-verify: macos-dmg ## Build and verify the standalone macOS DMG image
	@test -f "bin/$(APP_NAME).dmg" || { echo "macOS DMG was not created" >&2; exit 1; }
	@hdiutil imageinfo "bin/$(APP_NAME).dmg" >/dev/null
	@printf 'Verified macOS DMG: bin/%s.dmg\n' '$(APP_NAME)'

macos-zip: macos-app-verify ## Archive the native macOS app as a distributable ZIP
	@rm -f "bin/$(APP_NAME)-macos-$(ARCH).zip"
	@ditto -c -k --sequesterRsrc --keepParent "bin/$(APP_NAME).app" "bin/$(APP_NAME)-macos-$(ARCH).zip"
	@printf 'Created macOS ZIP: bin/%s-macos-%s.zip\n' '$(APP_NAME)' '$(ARCH)'

macos-universal-zip: macos-universal-verify ## Archive the universal macOS app as a distributable ZIP
	@rm -f "bin/$(APP_NAME)-macos-universal.zip"
	@ditto -c -k --sequesterRsrc --keepParent "bin/$(APP_NAME).app" "bin/$(APP_NAME)-macos-universal.zip"
	@printf 'Created universal macOS ZIP: bin/%s-macos-universal.zip\n' '$(APP_NAME)'

macos-run: node-check wails-check require-darwin ## Build/run the Wails macOS development bundle task
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task darwin:run

macos-sign: wails-check require-darwin ## Sign the macOS app using Wails signing configuration
	@$(WAILS_ENV) "$(WAILS_BIN)" task darwin:sign

macos-notarize: wails-check require-darwin ## Sign, submit, and notarize the macOS app
	@$(WAILS_ENV) "$(WAILS_BIN)" task darwin:sign:notarize

windows-build: node-check wails-check ## Build Windows amd64 executable through the Wails task
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task windows:build ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

windows-app: windows-build ## Alias for the Windows executable build

windows-tools-check: node-check ## Verify tools required by the selected Windows package format
	@if [ "$(PACKAGE_FORMAT)" = nsis ]; then command -v makensis >/dev/null || { echo 'makensis is required for NSIS packaging; run this target on Windows or install NSIS.' >&2; exit 1; }; fi
	@if [ "$(PACKAGE_FORMAT)" = msix ]; then printf '%s\n' 'MSIX packaging delegates tool checks to Wails; use make windows-installer PACKAGE_FORMAT=msix.'; fi

windows-installer: node-check wails-check windows-tools-check ## Build Windows NSIS installer; use PACKAGE_FORMAT=msix for MSIX
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task windows:package FORMAT=$(PACKAGE_FORMAT) INSTALL_SCOPE=$(INSTALL_SCOPE) ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

linux-build: node-check wails-check ## Build a Linux amd64 binary; Docker is used from macOS
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:build ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

docker-setup-amd64: require-docker ## Build an architecture-correct amd64 Wails cross-compilation image
	@docker build --platform linux/amd64 -t "$(CROSS_IMAGE_AMD64)" -f build/docker/Dockerfile.cross build/docker/

linux-build-amd64: node-check wails-check docker-setup-amd64 ## Build Linux amd64 using an amd64 Docker image
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:build ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE_AMD64)

linux-packages: node-check wails-check ## Build Linux application packages
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:package ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

linux-deb: node-check wails-check ## Build only the Linux .deb package
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:create:deb ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

linux-rpm: node-check wails-check ## Build only the Linux .rpm package
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:create:rpm ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

linux-appimage: node-check wails-check ## Build only the Linux AppImage
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:create:appimage ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE)

linux-packages-amd64: node-check wails-check docker-setup-amd64 ## Build Linux amd64 application packages with the architecture-correct image
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:package ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE_AMD64)

linux-deb-amd64: node-check wails-check docker-setup-amd64 ## Build the Linux amd64 deb package with the architecture-correct image
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:create:deb ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE_AMD64)

linux-rpm-amd64: node-check wails-check docker-setup-amd64 ## Build the Linux amd64 rpm package with the architecture-correct image
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:create:rpm ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE_AMD64)

linux-appimage-amd64: node-check wails-check docker-setup-amd64 ## Build the Linux amd64 AppImage with the architecture-correct image
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task linux:create:appimage ARCH=amd64 CROSS_IMAGE=$(CROSS_IMAGE_AMD64)

linux-sign-deb: wails-check ## Sign the generated Linux deb package using Wails signing configuration
	@$(WAILS_ENV) "$(WAILS_BIN)" task linux:sign:deb

linux-sign-rpm: wails-check ## Sign the generated Linux rpm package using Wails signing configuration
	@$(WAILS_ENV) "$(WAILS_BIN)" task linux:sign:rpm

linux-sign-packages: wails-check ## Sign generated Linux deb and rpm packages
	@$(WAILS_ENV) "$(WAILS_BIN)" task linux:sign:packages

linux-package-verify: ## Inspect generated Linux packages and binaries
	@test -d bin || { echo 'bin/ does not exist; build a Linux artifact first.' >&2; exit 1; }
	@find bin -maxdepth 1 -type f \( -name '*.deb' -o -name '*.rpm' -o -name '*.AppImage' -o -name '$(APP_NAME)' \) -print -exec file {} \; | sort

ios-build: node-check wails-check require-darwin ## Generate/build the iOS project using Wails tasks
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task ios:build

ios-package: node-check wails-check require-darwin ## Package the iOS simulator/application IPA using Wails tasks
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task ios:package:ipa IOS_PLATFORM=simulator

ios-package-device: node-check wails-check require-darwin ## Package a signed device IPA; requires identity and provisioning settings
	@test -n "$(CODESIGN_IDENTITY)" || { echo 'CODESIGN_IDENTITY is required for a device IPA.' >&2; exit 1; }
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task ios:package:ipa IOS_PLATFORM=device CODESIGN_IDENTITY="$(CODESIGN_IDENTITY)" PROVISIONING_PROFILE="$(PROVISIONING_PROFILE)"

android-tools-check: ## Verify the Android SDK, Java, and Gradle wrapper prerequisites
	@command -v java >/dev/null || [ -n "$(JAVA_HOME)" ] || { echo 'Java/JAVA_HOME is required for Android builds.' >&2; exit 1; }
	@test -x "build/android/gradlew" || { echo 'build/android/gradlew is missing or not executable.' >&2; exit 1; }
	@test -n "$(ANDROID_HOME)$(ANDROID_SDK_ROOT)" || { echo 'ANDROID_HOME or ANDROID_SDK_ROOT is required for Android builds.' >&2; exit 1; }

android-build: node-check wails-check android-tools-check ## Build the Android application
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task android:build

android-package: node-check wails-check android-tools-check ## Build the Android release package
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task android:package

docker-setup: require-docker wails-check ## Build the Wails cross-compilation Docker image
	@$(WAILS_ENV) "$(WAILS_BIN)" task setup:docker

docker-server-build: frontend-build require-docker ## Build the server-mode Docker image
	@docker build -t "$(SERVER_IMAGE)" -f build/docker/Dockerfile.server .

release-check: toolchain-check check evidence-check frontend-visual-test doctor ## Run all checks required before tagging a release
	@test -n "$$(git describe --tags --exact-match 2>/dev/null || true)" || { echo 'release-check must run from an exact Git tag' >&2; exit 1; }
	@printf 'Release checks passed for tag %s\n' "$$(git describe --tags --exact-match)"

release-macos: VERSION := $(shell git describe --tags --exact-match 2>/dev/null | sed 's/^v//')
release-macos: RELEASE_CHANNEL := stable
release-macos: node-check wails-check require-darwin ## Build the tagged standalone universal macOS app
	@test -n "$$(git describe --tags --exact-match 2>/dev/null || true)" || { echo 'release-macos requires an exact Git tag' >&2; exit 1; }
	@$(NODE_SETUP) $(WAILS_ENV) "$(WAILS_BIN)" task darwin:package:universal

artifacts: ## List generated application artifacts without deleting anything
	@find bin -maxdepth 1 \( -type f -o -type d -name '*.app' \) -print 2>/dev/null | sort || true

artifact-manifest: ## Write a deterministic manifest of generated top-level artifacts
	@mkdir -p "$(dir $(ARTIFACT_MANIFEST))"
	@{ printf '# Melodex artifacts\n'; find bin -mindepth 1 -maxdepth 1 \( -type f -o -type d -name '*.app' \) ! -name '.DS_Store' ! -name 'artifact-manifest.txt' ! -name 'SHA256SUMS' -print 2>/dev/null | sort; } > "$(ARTIFACT_MANIFEST)"
	@printf 'Artifact manifest: %s\n' '$(ARTIFACT_MANIFEST)'

checksums: artifact-manifest ## Write SHA-256 checksums for generated file artifacts
	@mkdir -p "$(dir $(CHECKSUM_FILE))"
	@{ \
		: > "$(CHECKSUM_FILE)"; \
		if command -v shasum >/dev/null 2>&1; then \
			while IFS= read -r -d '' artifact; do shasum -a 256 "$$artifact" >> "$(CHECKSUM_FILE)"; done < <(find bin -mindepth 1 -maxdepth 1 -type f ! -name '.DS_Store' ! -name 'artifact-manifest.txt' ! -name 'SHA256SUMS' -print0 | sort -z); \
		elif command -v sha256sum >/dev/null 2>&1; then \
			while IFS= read -r -d '' artifact; do sha256sum "$$artifact" >> "$(CHECKSUM_FILE)"; done < <(find bin -mindepth 1 -maxdepth 1 -type f ! -name '.DS_Store' ! -name 'artifact-manifest.txt' ! -name 'SHA256SUMS' -print0 | sort -z); \
		else echo 'shasum or sha256sum is required for checksum generation.' >&2; exit 1; fi; \
	}
	@printf 'Checksums: %s\n' '$(CHECKSUM_FILE)'

clean-frontend: ## Remove ignored Vite output while preserving tracked dist/index.html
	rm -rf frontend/dist/assets frontend/test-results frontend/playwright-report

clean-build: ## Remove Wails output while preserving build/config.yml and platform assets
	rm -rf bin build/bin build/tmp

clean-cache: ## Remove only the repository-local Go build cache
	rm -rf .melodex/go-build-cache

clean: clean-frontend clean-build clean-cache ## Remove safe generated build/test output
