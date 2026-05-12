#!/usr/bin/env bash
# scripts/check-modules.sh
#
# 多 module 发版审计脚本——遵循全局规则 4-PRE。
# 同时检测根模块（github.com/gtkit/logger）和 v2 子模块（github.com/gtkit/logger/v2），
# 对每个模块判断三项：
#   1. 自上次 tag 以来是否有代码变更
#   2. 直接依赖是否有可用更新
#   3. README / version.go 引用版本是否对齐最新 tag
#
# 任一项触发即报告"需要发版"。
#
# 用法：
#   bash scripts/check-modules.sh            # 全量检查
#   bash scripts/check-modules.sh --json     # JSON 输出（CI 友好）
#
# 退出码：
#   0  所有模块均无需发版
#   1  有模块需要发版（CI 可用作 fail-fast 信号，但默认报告 + 退出 0）
#   2  执行错误（git/go 不可用等）

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

JSON_MODE=false
for arg in "$@"; do
    case "$arg" in
        --json) JSON_MODE=true ;;
        -h | --help)
            sed -n '2,20p' "${BASH_SOURCE[0]}"
            exit 0
            ;;
        *)
            echo "unknown arg: $arg" >&2
            exit 2
            ;;
    esac
done

command -v git >/dev/null 2>&1 || {
    echo "git not found" >&2
    exit 2
}
command -v go >/dev/null 2>&1 || {
    echo "go not found" >&2
    exit 2
}

# ---- 模块定义 ----
# format: <module-name>|<dir>|<tag-prefix>|<version-file>|<readme>
MODULES=(
    "github.com/gtkit/logger|.|v1.|version.go|README.md"
    "github.com/gtkit/logger/v2|v2|v2.|v2/version.go|v2/README.md"
)

# ---- 工具函数 ----

latest_tag_for_prefix() {
    local prefix="$1"
    # 兼容 git 2.x：--sort=-version:refname 把高版本排在前
    git tag --list "${prefix}*" --sort=-version:refname \
        | grep -E "^${prefix}[0-9]+\.[0-9]+$" \
        | head -n1
}

# 路径 diff：根模块排除 v2/，v2 子模块只看 v2/
diff_files_for_module() {
    local dir="$1"
    local tag="$2"
    if [ -z "$tag" ]; then
        return 0
    fi
    if [ "$dir" = "." ]; then
        git diff --name-only "$tag"..HEAD -- . ':(exclude)v2/' ':(exclude)scripts/' 2>/dev/null || true
    else
        git diff --name-only "$tag"..HEAD -- "$dir/" 2>/dev/null || true
    fi
}

# 读取 version.go 里的 Version 常量
read_version_file() {
    local f="$1"
    if [ ! -f "$f" ]; then
        echo ""
        return
    fi
    grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' "$f" | head -n1
}

# go list 检查依赖更新（在子目录里执行）。使用 Go 模板格式避免 JSON 解析容错问题。
deps_with_updates() {
    local dir="$1"
    (
        cd "$dir"
        # GOWORK=off 防止 workspace 干扰；过滤掉 gtkit/logger 自身（v1/v2 互引用是预期）
        GOWORK=off go list -m -u -f '{{if .Update}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all 2>/dev/null \
            | grep -v '^$' \
            | grep -v -E '^github\.com/gtkit/logger(/v2)? ' \
            || true
    )
}

# README 中是否引用了与最新 tag 不一致的版本号
readme_version_mismatch() {
    local readme="$1"
    local latest_tag="$2"
    if [ ! -f "$readme" ] || [ -z "$latest_tag" ]; then
        return 0
    fi
    local refs
    refs=$(grep -oE 'v[0-9]+\.[0-9]+\.[0-9]+' "$readme" | sort -u || true)
    if [ -z "$refs" ]; then
        return 0
    fi
    local mismatched=""
    local latest_major
    latest_major=$(echo "$latest_tag" | cut -d. -f1)
    for ref in $refs; do
        local ref_major
        ref_major=$(echo "$ref" | cut -d. -f1)
        if [ "$latest_major" = "$ref_major" ] && [ "$ref" != "$latest_tag" ]; then
            mismatched="${mismatched}${ref} "
        fi
    done
    echo "$mismatched" | xargs
}

# ---- 主逻辑 ----

results=()
any_needs_release=false

for entry in "${MODULES[@]}"; do
    IFS='|' read -r mod_name mod_dir tag_prefix version_file readme <<<"$entry"

    latest_tag=$(latest_tag_for_prefix "$tag_prefix")
    declared_version=$(read_version_file "$version_file")
    changed_files=$(diff_files_for_module "$mod_dir" "$latest_tag")
    dep_updates=$(deps_with_updates "$mod_dir" 2>/dev/null || true)
    readme_mismatch=$(readme_version_mismatch "$readme" "$latest_tag")

    has_changes=false
    [ -n "$changed_files" ] && has_changes=true
    has_dep_updates=false
    [ -n "$dep_updates" ] && has_dep_updates=true
    has_readme_drift=false
    [ -n "$readme_mismatch" ] && has_readme_drift=true
    version_drift=false
    if [ -n "$declared_version" ] && [ -n "$latest_tag" ] && [ "$declared_version" != "$latest_tag" ]; then
        version_drift=true
    fi

    needs_release=false
    if $has_changes || $has_dep_updates || $has_readme_drift || $version_drift; then
        needs_release=true
        any_needs_release=true
    fi

    if $JSON_MODE; then
        results+=("$(printf '{"module":"%s","dir":"%s","latest_tag":"%s","declared_version":"%s","needs_release":%s,"has_changes":%s,"has_dep_updates":%s,"has_readme_drift":%s,"version_drift":%s}' \
            "$mod_name" "$mod_dir" "$latest_tag" "$declared_version" "$needs_release" "$has_changes" "$has_dep_updates" "$has_readme_drift" "$version_drift")")
    else
        echo "═══════════════════════════════════════════════════════════════"
        echo "module:           $mod_name"
        echo "directory:        $mod_dir"
        echo "latest tag:       ${latest_tag:-<none>}"
        echo "version.go const: ${declared_version:-<none>}"
        echo "---"
        if $has_changes; then
            changed_count=$(echo "$changed_files" | wc -l | xargs)
            echo "▲ code changes since $latest_tag: $changed_count files"
            echo "$changed_files" | sed 's/^/    /'
        else
            echo "✓ no code changes since $latest_tag"
        fi
        echo "---"
        if $has_dep_updates; then
            echo "▲ dependency updates available:"
            echo "$dep_updates" | sed 's/^/    /'
        else
            echo "✓ dependencies up-to-date"
        fi
        echo "---"
        if $has_readme_drift; then
            echo "▲ README references stale versions: $readme_mismatch (latest: $latest_tag)"
        else
            echo "✓ README version refs aligned"
        fi
        echo "---"
        if $version_drift; then
            echo "▲ version.go ($declared_version) != latest tag ($latest_tag)"
        else
            echo "✓ version.go matches latest tag"
        fi
        echo "---"
        if $needs_release; then
            echo "🔔 RECOMMENDATION: $mod_name needs a new release"
        else
            echo "✅ no release needed"
        fi
        echo ""
    fi
done

if $JSON_MODE; then
    printf '['
    IFS=','
    printf '%s' "${results[*]}"
    printf ']\n'
fi

if $any_needs_release; then
    if ! $JSON_MODE; then
        echo "═══════════════════════════════════════════════════════════════"
        echo "SUMMARY: one or more modules need a release"
        echo "═══════════════════════════════════════════════════════════════"
    fi
    exit 1
fi

if ! $JSON_MODE; then
    echo "═══════════════════════════════════════════════════════════════"
    echo "SUMMARY: all modules are up-to-date"
    echo "═══════════════════════════════════════════════════════════════"
fi
exit 0
