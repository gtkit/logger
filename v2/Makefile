.PHONY: lint fmt check tag gittag fix-tags

LINT_TARGETS ?= ./...

lint: ## 只读静态检查（不修改代码，格式化用 make fmt）
	@ echo "▶️ golangci-lint run"
	golangci-lint run $(LINT_TARGETS)
	@ unformatted=$$(gofumpt -l .); \
	if [ -n "$$unformatted" ]; then \
	  echo "✗ 以下文件未按 gofumpt 格式化（运行 make fmt 修复）:"; echo "$$unformatted"; exit 1; \
	fi
	@ echo "✅ golangci-lint run"

fmt: ## 按 gofumpt 格式化代码（唯一允许写文件的格式化入口）
	gofumpt -l -w .

check: ## Check vulnerabilities
	govulncheck ./...
	gosec ./...

# 发版语义级别：patch（默认）/ minor。新增导出 API 必须 BUMP=minor。
# major（/v3）需要 module path 变更与目录/分支重组，不由本脚本承载。
BUMP ?= patch
COVERAGE_MIN ?= 80
tag: ## 发布（BUMP=patch|minor）：干净树 + vet/lint/race + CHANGELOG 版本检查 + bump + tag
	@set -e; \
	if [ -n "$$(git status --porcelain)" ]; then \
	  echo "✗ 工作区不干净，发版前请先提交或清理（防止给不含本次功能的提交打 tag）："; \
	  git status --short; exit 1; \
	fi; \
	echo "▶️ go mod tidy -diff"; go mod tidy -diff; \
	echo "▶️ go vet"; go vet ./...; \
	echo "▶️ lint（含 gofumpt 只读检查）"; $(MAKE) lint; \
	echo "▶️ 漏洞与安全扫描"; govulncheck ./...; gosec ./...; \
	echo "▶️ 测试 (race)"; go test -race -count=1 -timeout=5m ./...; \
	echo "▶️ 覆盖率 ≥ $(COVERAGE_MIN)%"; \
	go test -count=1 -coverprofile=coverage.out ./... >/dev/null; \
	cov=$$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}'); \
	rm -f coverage.out; \
	awk -v c="$$cov" -v m="$(COVERAGE_MIN)" 'BEGIN{exit !(c>=m)}' || { echo "✗ 覆盖率 $$cov% 低于 $(COVERAGE_MIN)%"; exit 1; }; \
	echo "▶️ benchmark（-benchmem -count=3）"; go test -run '^$$' -bench . -benchmem -count=3 ./...; \
	current=$$(grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' version.go | head -n1 | tr -d 'v'); \
	if [ -z "$$current" ]; then echo "version not found in version.go"; exit 1; fi; \
	maj=$$(echo $$current | cut -d. -f1); \
	min=$$(echo $$current | cut -d. -f2); \
	patch=$$(echo $$current | cut -d. -f3); \
	case "$(BUMP)" in \
	  minor) min=$$(expr $$min + 1); newpatch=0 ;; \
	  patch) newpatch=$$(expr $$patch + 1) ;; \
	  *) echo "BUMP 只支持 patch|minor, got $(BUMP)"; exit 1 ;; \
	esac; \
	new="v$$maj.$$min.$$newpatch"; \
	if ! grep -q "$$new" ../CHANGELOG.md; then \
	  echo "✗ CHANGELOG.md 中没有 $$new 版本段：请先把 Unreleased 内容整理到该版本标题下再发版"; exit 1; \
	fi; \
	printf "Bump: v%s -> %s\n" "$$current" "$$new"; \
	sed -E -i.bak 's/(const Version = ")([^"]+)(")/\1'"$$new"'\3/' version.go; \
	git add version.go; \
	git commit -m "chore(release): 发布 logger/v2 $$new"; \
	printf "Release: %s\n" "$$new"; \
	git push gtkit HEAD; \
	git tag -a "$$new" -m "发布 logger/v2 $$new"; \
	printf "Tag: %s\n" "$$new"; \
	git push gtkit "$$new"; \
	printf "Done\n"
	rm -f version.go.bak

gittag: ## Show latest tag
	git tag --list 'v2.*' --sort=-version:refname | head -1

fix-tags: ## Backfill canonical v2.x.y tags from legacy v2/v2.x.y tags
	@for legacy in $$(git tag --list 'v2/v2.*'); do \
		version=$${legacy#v2/}; \
		if git rev-parse "$$version" >/dev/null 2>&1; then \
			printf "skip %s (already exists)\n" "$$version"; \
			continue; \
		fi; \
		obj=$$(git rev-parse "$$legacy^{}"); \
		git tag -a "$$version" "$$obj" -m "发布 logger/v2 $$version（补规范 tag）"; \
		printf "created %s -> %s\n" "$$version" "$$obj"; \
	done
