package logger

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// customRIDExtractor 模拟用户自定义 extractor 也产出 request_id 的场景。
func customRIDExtractor(value string, extra ...zap.Field) ContextFieldsFunc {
	return func(context.Context) []zap.Field {
		return append([]zap.Field{zap.String("request_id", value)}, extra...)
	}
}

func newDedupLogger(t *testing.T, opts ...Option) (*Logger, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "logs", "app")
	base := append([]Option{
		WithConsole(false), WithFile(true), WithOutJSON(true),
		WithPath(path), WithReplaceGlobals(false),
	}, opts...)
	l := MustNew(base...)
	t.Cleanup(l.Sync)
	return l, path
}

// countKey 统计一行 JSON 里某个 key 出现的次数（重复 key 是解析器未定义行为，必须恰好一次）。
func countKey(line, key string) int {
	return strings.Count(line, `"`+key+`":`)
}

// TestRequestIDDedupMatrix 三源组合矩阵：内建（ctx 携带）× 自定义 extractor × 调用点字段，
// 断言 request_id 恰好出现一次，且值符合优先级：调用点 > 自定义 > 内建。
func TestRequestIDDedupMatrix(t *testing.T) {
	ctxWithID := func() context.Context { return ContextWithRequestID(t.Context(), "rid-builtin") }
	callSite := zap.String("request_id", "rid-callsite")

	tests := []struct {
		name      string
		custom    ContextFieldsFunc // nil = 无自定义 extractor
		ctx       func() context.Context
		fields    []zap.Field
		wantValue string
	}{
		{
			name: "仅内建", ctx: ctxWithID,
			wantValue: "rid-builtin",
		},
		{
			name: "内建+自定义→自定义胜", ctx: ctxWithID,
			custom:    customRIDExtractor("rid-custom", zap.String("tenant", "acme")),
			wantValue: "rid-custom",
		},
		{
			name: "内建+调用点→调用点胜", ctx: ctxWithID,
			fields:    []zap.Field{callSite},
			wantValue: "rid-callsite",
		},
		{
			name: "三源并存→调用点胜", ctx: ctxWithID,
			custom:    customRIDExtractor("rid-custom"),
			fields:    []zap.Field{callSite},
			wantValue: "rid-callsite",
		},
		{
			name:      "自定义+调用点→调用点胜",
			ctx:       func() context.Context { return t.Context() },
			custom:    customRIDExtractor("rid-custom"),
			fields:    []zap.Field{callSite},
			wantValue: "rid-callsite",
		},
		{
			name:      "仅调用点",
			ctx:       func() context.Context { return t.Context() },
			fields:    []zap.Field{callSite},
			wantValue: "rid-callsite",
		},
		{
			name:      "仅自定义",
			ctx:       func() context.Context { return t.Context() },
			custom:    customRIDExtractor("rid-custom"),
			wantValue: "rid-custom",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.custom != nil {
				opts = append(opts, WithContextFields(tt.custom))
			}
			l, path := newDedupLogger(t, opts...)

			l.InfoCtx(tt.ctx(), "dedup-check", tt.fields...)
			l.Sync()

			line := logLineContaining(t, readLogFile(t, path+"-info.log"), "dedup-check")
			if got := countKey(line, "request_id"); got != 1 {
				t.Fatalf("request_id 应恰好出现 1 次, got %d: %s", got, line)
			}
			if !strings.Contains(line, tt.wantValue) {
				t.Fatalf("request_id 值应为 %q（优先级：调用点>自定义>内建）: %s", tt.wantValue, line)
			}
			// 自定义 extractor 的其他字段不受去重影响
			if tt.name == "内建+自定义→自定义胜" && !strings.Contains(line, "acme") {
				t.Fatalf("非 request_id 的自定义字段应保留: %s", line)
			}
		})
	}
}

// TestRequestIDDedupKVVariants w 风格（keysAndValues）的三源去重：
// 字符串 key 与内联 zap.Field 两种调用点形态。
func TestRequestIDDedupKVVariants(t *testing.T) {
	ctx := ContextWithRequestID(t.Context(), "rid-builtin")

	tests := []struct {
		name      string
		kv        []any
		wantValue string
	}{
		{name: "字符串 key 形态", kv: []any{"request_id", "rid-kv"}, wantValue: "rid-kv"},
		{name: "内联 Field 形态", kv: []any{zap.String("request_id", "rid-field")}, wantValue: "rid-field"},
		{name: "无调用点冲突", kv: []any{"order_id", "A100"}, wantValue: "rid-builtin"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, path := newDedupLogger(t)

			l.InfowCtx(ctx, "kv-dedup-check", tt.kv...)
			l.Sync()

			line := logLineContaining(t, readLogFile(t, path+"-info.log"), "kv-dedup-check")
			if got := countKey(line, "request_id"); got != 1 {
				t.Fatalf("request_id 应恰好出现 1 次, got %d: %s", got, line)
			}
			if !strings.Contains(line, tt.wantValue) {
				t.Fatalf("request_id 值应为 %q: %s", tt.wantValue, line)
			}
		})
	}
}

// TestHasRequestIDKeyMalformedInputNoPanic 畸形 keysAndValues 只保证不 panic。
func TestHasRequestIDKeyMalformedInputNoPanic(t *testing.T) {
	t.Parallel()

	cases := [][]any{
		{"request_id"},              // 奇数长度
		{123, "v", "request_id", 1}, // 非法 key 类型混入
		{nil, nil},                  // nil 键值
		{zap.Int("n", 1), "k"},      // Field 与孤儿 key 混排
	}
	for _, kv := range cases {
		_ = hasRequestIDKey(kv)
	}
	if !hasRequestIDKey([]any{123, "request_id"}) {
		// 非法 key(123) 后步进 1，"request_id" 被当作 key 检出——宽松匹配可接受
		t.Log("宽松匹配未命中（可接受，仅要求不 panic）")
	}
}

// TestContextWithRequestIDNilQuadrants nil ctx × 空 id 四象限：不 panic、语义一致。
func TestContextWithRequestIDNilQuadrants(t *testing.T) {
	t.Parallel()

	//nolint:staticcheck // 显式验证 nil ctx 的宽容语义
	if got := ContextWithRequestID(nil, "id"); got != nil {
		t.Fatalf("nil ctx 应原样返回 nil, got %v", got)
	}
	//nolint:staticcheck // 同上
	if got := ContextWithRequestID(nil, ""); got != nil {
		t.Fatalf("nil ctx + 空 id 应原样返回 nil, got %v", got)
	}
	base := t.Context()
	if got := ContextWithRequestID(base, ""); got != base {
		t.Fatal("空 id 应原样返回")
	}
	if got := RequestIDFromContext(ContextWithRequestID(base, "x")); got != "x" {
		t.Fatalf("正常路径回读 = %q, want x", got)
	}
}

// TestRequestIDDedupWithPreBoundFields 第四来源：With() 预绑定 request_id 已烧进
// zap 视图，内建不得再产出（否则重复 key 以「预绑定+内建」形态复现）。
func TestRequestIDDedupWithPreBoundFields(t *testing.T) {
	ctx := ContextWithRequestID(t.Context(), "rid-builtin")

	l, path := newDedupLogger(t)
	bound := l.With(zap.String("request_id", "rid-prebound"))

	bound.InfoCtx(ctx, "prebound-fields-check")
	bound.InfowCtx(ctx, "prebound-kv-check", "order_id", "A100")
	l.Sync()

	content := readLogFile(t, path+"-info.log")
	for _, marker := range []string{"prebound-fields-check", "prebound-kv-check"} {
		line := logLineContaining(t, content, marker)
		if got := countKey(line, "request_id"); got != 1 {
			t.Fatalf("%s: request_id 应恰好 1 次, got %d: %s", marker, got, line)
		}
		if !strings.Contains(line, "rid-prebound") {
			t.Fatalf("%s: 预绑定值应胜出: %s", marker, line)
		}
	}
}

// TestCtxGateRespectsDynamicSetLevel 门控必须跟随 SetLevel 动态调级，
// 不能把构建期级别冻结进门控判断。
func TestCtxGateRespectsDynamicSetLevel(t *testing.T) {
	l, path := newDedupLogger(t, WithLevel("error"))
	ctx := ContextWithRequestID(t.Context(), "rid-dyn")

	l.InfoCtx(ctx, "gated-before-setlevel") // error 级别下应被门控丢弃
	l.SetLevel("debug")
	l.InfoCtx(ctx, "passes-after-setlevel") // 调级后应放行
	l.Sync()

	content := readLogFile(t, path+"-error.log")
	if strings.Contains(content, "gated-before-setlevel") {
		t.Fatal("error 级别下 InfoCtx 应被门控丢弃")
	}
	line := logLineContaining(t, content, "passes-after-setlevel")
	if !strings.Contains(line, "rid-dyn") {
		t.Fatalf("调级后应放行且带 request_id: %s", line)
	}
}

// TestRequestIDPriorityFullMatrix 四来源全组合矩阵（fields 形态）：
// P=With 预绑定、C=调用点、X=自定义 extractor、B=内建（ctx 携带）。
// 覆盖全部两两冲突对 C(4,2)=6、四个三源组合与四源并存，
// 断言恰好一个 request_id 且值符合优先级 P > C > X > B。
func TestRequestIDPriorityFullMatrix(t *testing.T) {
	combos := []struct {
		name       string
		p, c, x, b bool
		want       string
	}{
		{name: "PC", p: true, c: true, want: "rid-P"},
		{name: "PX", p: true, x: true, want: "rid-P"},
		{name: "PB", p: true, b: true, want: "rid-P"},
		{name: "CX", c: true, x: true, want: "rid-C"},
		{name: "CB", c: true, b: true, want: "rid-C"},
		{name: "XB", x: true, b: true, want: "rid-X"},
		{name: "PCX", p: true, c: true, x: true, want: "rid-P"},
		{name: "PCB", p: true, c: true, b: true, want: "rid-P"},
		{name: "PXB", p: true, x: true, b: true, want: "rid-P"},
		{name: "CXB", c: true, x: true, b: true, want: "rid-C"},
		{name: "PCXB", p: true, c: true, x: true, b: true, want: "rid-P"},
	}

	for _, tt := range combos {
		t.Run(tt.name, func(t *testing.T) {
			var opts []Option
			if tt.x {
				opts = append(opts, WithContextFields(customRIDExtractor("rid-X")))
			}
			l, path := newDedupLogger(t, opts...)

			target := l
			if tt.p {
				target = l.With(zap.String("request_id", "rid-P"))
			}
			ctx := t.Context()
			if tt.b {
				ctx = ContextWithRequestID(ctx, "rid-B")
			}
			var fields []zap.Field
			if tt.c {
				fields = append(fields, zap.String("request_id", "rid-C"))
			}

			target.InfoCtx(ctx, "matrix-"+tt.name, fields...)
			l.Sync()

			line := logLineContaining(t, readLogFile(t, path+"-info.log"), "matrix-"+tt.name)
			if got := countKey(line, "request_id"); got != 1 {
				t.Fatalf("%s: request_id 应恰好 1 次, got %d: %s", tt.name, got, line)
			}
			if !strings.Contains(line, tt.want) {
				t.Fatalf("%s: 应为 %s 胜出: %s", tt.name, tt.want, line)
			}
		})
	}
}

// TestRequestIDPriorityKVMatrix 含调用点来源的组合在 kv 形态下的去重
// （字符串 key 与内联 Field 双写法），外加预绑定×kv 调用点的直接复现用例。
func TestRequestIDPriorityKVMatrix(t *testing.T) {
	kvForms := []struct {
		form string
		kv   func(v string) []any
	}{
		{form: "字符串key", kv: func(v string) []any { return []any{"request_id", v, "order_id", "A1"} }},
		{form: "内联Field", kv: func(v string) []any { return []any{zap.String("request_id", v), "order_id", "A1"} }},
	}
	combos := []struct {
		name string
		p, b bool
		want string
	}{
		{name: "PC", p: true, want: "rid-P"},
		{name: "CB", b: true, want: "rid-C"},
		{name: "PCB", p: true, b: true, want: "rid-P"},
		{name: "onlyC", want: "rid-C"},
	}

	for _, form := range kvForms {
		for _, tt := range combos {
			t.Run(form.form+"/"+tt.name, func(t *testing.T) {
				l, path := newDedupLogger(t)
				target := l
				if tt.p {
					target = l.With(zap.String("request_id", "rid-P"))
				}
				ctx := t.Context()
				if tt.b {
					ctx = ContextWithRequestID(ctx, "rid-B")
				}

				marker := "kvmatrix-" + form.form + "-" + tt.name
				target.InfowCtx(ctx, marker, form.kv("rid-C")...)
				l.Sync()

				line := logLineContaining(t, readLogFile(t, path+"-info.log"), marker)
				if got := countKey(line, "request_id"); got != 1 {
					t.Fatalf("request_id 应恰好 1 次, got %d: %s", got, line)
				}
				if !strings.Contains(line, tt.want) {
					t.Fatalf("应为 %s 胜出: %s", tt.want, line)
				}
				if !strings.Contains(line, "A1") {
					t.Fatalf("非冲突 kv 项应保留: %s", line)
				}
			})
		}
	}
}

// TestRequestIDSameSourceDuplicates 同源重复归一化：同一来源内部提供多个
// request_id 时保留最后一个（矩阵新增的"来源自身重复"维度）。
func TestRequestIDSameSourceDuplicates(t *testing.T) {
	t.Run("调用点fields重复", func(t *testing.T) {
		l, path := newDedupLogger(t)
		l.InfoCtx(t.Context(), "same-callsite-fields",
			zap.String("request_id", "rid-1"), zap.String("order_id", "A1"), zap.String("request_id", "rid-2"))
		l.Sync()
		line := logLineContaining(t, readLogFile(t, path+"-info.log"), "same-callsite-fields")
		if got := countKey(line, "request_id"); got != 1 {
			t.Fatalf("request_id 应恰好 1 次, got %d: %s", got, line)
		}
		if !strings.Contains(line, "rid-2") || !strings.Contains(line, "A1") {
			t.Fatalf("应保留最后一个且不伤及其他字段: %s", line)
		}
	})

	t.Run("调用点kv重复_字符串key", func(t *testing.T) {
		l, path := newDedupLogger(t)
		l.InfowCtx(t.Context(), "same-callsite-kv",
			"request_id", "rid-1", "order_id", "A1", "request_id", "rid-2")
		l.Sync()
		line := logLineContaining(t, readLogFile(t, path+"-info.log"), "same-callsite-kv")
		if got := countKey(line, "request_id"); got != 1 {
			t.Fatalf("request_id 应恰好 1 次, got %d: %s", got, line)
		}
		if !strings.Contains(line, "rid-2") || !strings.Contains(line, "A1") {
			t.Fatalf("应保留最后一个且不伤及其他键值: %s", line)
		}
	})

	t.Run("调用点kv重复_混合形态", func(t *testing.T) {
		l, path := newDedupLogger(t)
		l.InfowCtx(t.Context(), "same-callsite-kv-mixed",
			zap.String("request_id", "rid-1"), "request_id", "rid-2", "order_id", "A1")
		l.Sync()
		line := logLineContaining(t, readLogFile(t, path+"-info.log"), "same-callsite-kv-mixed")
		if got := countKey(line, "request_id"); got != 1 {
			t.Fatalf("request_id 应恰好 1 次, got %d: %s", got, line)
		}
		if !strings.Contains(line, "rid-2") {
			t.Fatalf("应保留最后一个: %s", line)
		}
	})

	t.Run("extractor输出重复", func(t *testing.T) {
		l, path := newDedupLogger(t, WithContextFields(func(context.Context) []zap.Field {
			return []zap.Field{
				zap.String("request_id", "rid-x1"),
				zap.String("tenant", "acme"),
				zap.String("request_id", "rid-x2"),
			}
		}))
		l.InfoCtx(t.Context(), "same-extractor")
		l.Sync()
		line := logLineContaining(t, readLogFile(t, path+"-info.log"), "same-extractor")
		if got := countKey(line, "request_id"); got != 1 {
			t.Fatalf("request_id 应恰好 1 次, got %d: %s", got, line)
		}
		if !strings.Contains(line, "rid-x2") || !strings.Contains(line, "acme") {
			t.Fatalf("应保留最后一个且其余字段完好: %s", line)
		}
	})

	t.Run("链式With重复_构建期归一化惠及非Ctx方法", func(t *testing.T) {
		l, path := newDedupLogger(t)
		bound := l.With(zap.String("request_id", "rid-w1")).With(zap.String("request_id", "rid-w2"))

		bound.Info("same-with-chain-plain") // 非 Ctx 方法同样只出一个
		bound.InfoCtx(t.Context(), "same-with-chain-ctx")
		l.Sync()

		content := readLogFile(t, path+"-info.log")
		for _, marker := range []string{"same-with-chain-plain", "same-with-chain-ctx"} {
			line := logLineContaining(t, content, marker)
			if got := countKey(line, "request_id"); got != 1 {
				t.Fatalf("%s: request_id 应恰好 1 次, got %d: %s", marker, got, line)
			}
			if !strings.Contains(line, "rid-w2") {
				t.Fatalf("%s: 链式 With 应保留最后一个: %s", marker, line)
			}
		}
	})

	t.Run("同源重复与跨源优先级叠加", func(t *testing.T) {
		l, path := newDedupLogger(t)
		ctx := ContextWithRequestID(t.Context(), "rid-B")
		// 调用点重复（归一化取 rid-c2）胜过内建 rid-B
		l.InfoCtx(ctx, "mixed-priority",
			zap.String("request_id", "rid-c1"), zap.String("request_id", "rid-c2"))
		l.Sync()
		line := logLineContaining(t, readLogFile(t, path+"-info.log"), "mixed-priority")
		if got := countKey(line, "request_id"); got != 1 {
			t.Fatalf("request_id 应恰好 1 次, got %d: %s", got, line)
		}
		if !strings.Contains(line, "rid-c2") {
			t.Fatalf("同源归一化后按跨源优先级取调用点末值: %s", line)
		}
	})
}
