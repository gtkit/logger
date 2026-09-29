package logger

import (
	"context"

	"go.uber.org/zap"
)

// requestId 的跨库标准约定：调用方（通常是 HTTP 中间件）把 requestId 写入
// context，本库所有 *Ctx 方法自动合并 request_id 字段（无需任何配置）；
// 其他基础设施（如 GORM 日志适配器的 trace 提取）可直接复用 RequestIDFromContext，
// 使访问、业务、SQL 三类日志共享同一个 id 值做全链路检索。
// 注意字段名并不跨库统一：本库为 request_id，ormx/zlogger 为 trace_id。

// requestIDFieldName 是 *Ctx 方法自动合并的字段名。
const requestIDFieldName = "request_id"

type requestIDCtxKey struct{}

// ContextWithRequestID 把 requestId 写入 context；ctx 为 nil 或 id 为空时原样返回
// （与 RequestIDFromContext 的 nil 宽容语义一致，不 panic）。
func ContextWithRequestID(ctx context.Context, id string) context.Context {
	if ctx == nil || id == "" {
		return ctx
	}

	return context.WithValue(ctx, requestIDCtxKey{}, id)
}

// RequestIDFromContext 从 context 提取 requestId；不存在时返回空串。
func RequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	id, _ := ctx.Value(requestIDCtxKey{}).(string)

	return id
}

// extractCtxFields 汇总 ctx 携带的日志字段：内建 request_id 在前，
// WithContextFields 注入的自定义字段在后。
//
// request_id 去重优先级：调用点字段 > 自定义上下文字段 > 内建 RequestID，
// 同名 key 只保留最高优先级的一个，避免同一条 JSON 出现重复 key
// （不同解析器对重复 key 的取值行为未定义）。
// callSiteHasRequestID 由调用方按调用点字段预先判定。
func (l *Logger) extractCtxFields(ctx context.Context, callSiteHasRequestID bool) []zap.Field {
	var custom []zap.Field
	if l.contextFields != nil {
		custom = l.contextFields(ctx)
	}

	if callSiteHasRequestID {
		// 调用点已提供：自定义字段中的同名 key 一并剔除，内建不再产出。
		return filterOutRequestID(custom)
	}
	if hasRequestIDField(custom) {
		// 自定义 extractor 已提供：内建不再产出；同源内重复保留最后一个。
		return normalizeRequestID(custom)
	}
	id := RequestIDFromContext(ctx)
	if id == "" {
		return custom
	}

	out := make([]zap.Field, 0, len(custom)+1)
	out = append(out, zap.String(requestIDFieldName, id))
	out = append(out, custom...)

	return out
}

// normalizeRequestID 同源归一化：同一来源内出现多个 request_id 时只保留最后一个
// （与 zap With 链式覆盖直觉、JSON 解析器普遍取末值的行为一致）；
// 至多一个时零分配原样返回。
func normalizeRequestID(fields []zap.Field) []zap.Field {
	if len(fields) < 2 {
		return fields
	}
	count, last := 0, -1
	for i := range fields {
		if fields[i].Key == requestIDFieldName {
			count++
			last = i
		}
	}
	if count <= 1 {
		return fields
	}
	out := make([]zap.Field, 0, len(fields)-count+1)
	for i := range fields {
		if fields[i].Key == requestIDFieldName && i != last {
			continue
		}
		out = append(out, fields[i])
	}

	return out
}

// normalizeRequestIDKV 同源归一化的 kv 形态：兼容「字符串 key+值」与内联 Field，
// 重复时只保留最后一次出现；至多一个时零分配原样返回。
func normalizeRequestIDKV(kv []any) []any {
	if len(kv) < 2 {
		return kv
	}
	// 第一遍：定位全部 request_id 项（起始下标与宽度）
	type hit struct{ idx, width int }
	var hits []hit
	for i := 0; i < len(kv); i++ {
		switch v := kv[i].(type) {
		case string:
			if v == requestIDFieldName {
				w := 1
				if i+1 < len(kv) {
					w = 2
				}
				hits = append(hits, hit{idx: i, width: w})
				i += w - 1
				continue
			}
			i++ // 跳过普通 key 的 value
		case zap.Field:
			if v.Key == requestIDFieldName {
				hits = append(hits, hit{idx: i, width: 1})
			}
		}
	}
	if len(hits) <= 1 {
		return kv
	}
	drop := make(map[int]int, len(hits)-1) // idx → width（保留最后一个命中）
	for _, h := range hits[:len(hits)-1] {
		drop[h.idx] = h.width
	}
	out := make([]any, 0, len(kv))
	for i := 0; i < len(kv); i++ {
		if w, ok := drop[i]; ok {
			i += w - 1
			continue
		}
		out = append(out, kv[i])
	}

	return out
}

// hasRequestIDField 判断 zap 字段集中是否已含 request_id。
func hasRequestIDField(fields []zap.Field) bool {
	for i := range fields {
		if fields[i].Key == requestIDFieldName {
			return true
		}
	}

	return false
}

// filterOutRequestID 剔除字段集中的 request_id；无命中时零分配原样返回。
func filterOutRequestID(fields []zap.Field) []zap.Field {
	if !hasRequestIDField(fields) {
		return fields
	}
	out := make([]zap.Field, 0, len(fields)-1)
	for i := range fields {
		if fields[i].Key != requestIDFieldName {
			out = append(out, fields[i])
		}
	}

	return out
}

// filterOutRequestIDKV 剔除 sugar 风格 keysAndValues 中的 request_id 项
// （「字符串 key+值」成对剔除，内联 zap.Field 单个剔除）；无命中时零分配原样返回。
func filterOutRequestIDKV(kv []any) []any {
	if !hasRequestIDKey(kv) {
		return kv
	}
	out := make([]any, 0, len(kv))
	for i := 0; i < len(kv); i++ {
		switch v := kv[i].(type) {
		case string:
			if v == requestIDFieldName {
				i++ // 连同 value 一起剔除
				continue
			}
			out = append(out, v)
			if i+1 < len(kv) {
				out = append(out, kv[i+1])
				i++
			}
		case zap.Field:
			if v.Key == requestIDFieldName {
				continue
			}
			out = append(out, v)
		default:
			out = append(out, v)
		}
	}

	return out
}

// hasRequestIDKey 判断 sugar 风格 keysAndValues 中是否已含 request_id 键。
// 兼容「字符串 key + value」与直接内联 zap.Field 两种形态；
// 对畸形输入只保证不 panic（畸形本身由 zap sugar 报告）。
func hasRequestIDKey(kv []any) bool {
	for i := 0; i < len(kv); i++ {
		switch v := kv[i].(type) {
		case string:
			if v == requestIDFieldName {
				return true
			}
			i++ // 跳过该 key 对应的 value
		case zap.Field:
			if v.Key == requestIDFieldName {
				return true
			}
		}
	}

	return false
}
