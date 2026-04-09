.PHONY: lint check tag gittag fix-tags

LINT_TARGETS ?= ./...

lint: ## Lint Go code
	@ echo "▶️ golangci-lint run"
	golangci-lint run $(LINT_TARGETS)
	gofumpt -l -w .
	@ echo "✅ golangci-lint run"

check: ## Check vulnerabilities
	govulncheck ./...
	gosec ./...

tag: ## Bump patch version and push tag
	@current=$$(grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' version.go | head -n1 | tr -d 'v'); \
	if [ -z "$$current" ]; then echo "version not found in version.go"; exit 1; fi; \
	maj=$$(echo $$current | cut -d. -f1); \
	min=$$(echo $$current | cut -d. -f2); \
	patch=$$(echo $$current | cut -d. -f3); \
	newpatch=$$(expr $$patch + 1); \
	new="v$$maj.$$min.$$newpatch"; \
	printf "Bump: v%s -> %s\n" "$$current" "$$new"; \
	sed -E -i.bak 's/(const Version = ")([^"]+)(")/\1'"$$new"'\3/' version.go; \
	git add version.go; \
	git commit -m "chore(release): logger/v2 $$new"; \
	printf "Release: %s\n" "$$new"; \
	git push gtkit HEAD; \
	git tag -a "$$new" -m "release $$new"; \
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
		git tag -a "$$version" "$$obj" -m "release $$version"; \
		printf "created %s -> %s\n" "$$version" "$$obj"; \
	done
