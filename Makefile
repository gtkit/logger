.PHONY: tool, check

LINT_TARGETS ?= ./...
tool: ## Lint Go code with the installed golangci-lint
	@ echo "▶️ golangci-lint run"
	golangci-lint  run   $(LINT_TARGETS)
	gofmt -l -w .
	@ echo "✅ golangci-lint run"

## govulncheck 检查漏洞 `go install golang.org/x/vuln/cmd/govulncheck@latest`
## gosec 检查安全漏洞 `go install github.com/securego/gosec/v2/cmd/gosec@latest`
check:
	govulncheck ./...
	gosec ./...
## betteralign 优化结构体字段排序和内存布局 `go install github.com/dkorunic/betteralign/cmd/betteralign@latest`
better:
	betteralign ./...
	betteralign -apply ./...
## gofumpt 格式化代码 `go install mvdan.cc/gofumpt@latest`
fmt:
	gofumpt -l -w .
	go mod tidy
