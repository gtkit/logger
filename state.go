package logger

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"sync/atomic"

	"go.uber.org/zap"
)

// maxDynamicChannels 动态 channel 缓存上限，防止无限增长导致内存泄漏。
const maxDynamicChannels = 1024

type loggerState struct {
	root              *zap.Logger
	sugar             *zap.SugaredLogger
	messager          Messager
	asyncMsg          *asyncMessager
	contextFields     ContextFieldsFunc
	atomicLevel       zap.AtomicLevel
	channelBases      map[string]*zap.Logger
	dynamicChannel    sync.Map
	dynamicChannelCnt atomic.Int64
	closers           []io.Closer
	undo              func()
	done              chan struct{}
	doneOnce          sync.Once
	retired           atomic.Bool
	refs              atomic.Int64
	fallback          bool
}

// fallbackWarnOnce 保证"未初始化即打日志"的告警全进程只打一次。
var fallbackWarnOnce sync.Once

// warnFallbackUsage 在业务代码尚未调用 New()/NewZap() 就打日志时，往 stderr 告警一次。
// 此时日志走的是开发期 fallback logger（console），WithPath/WithOutJSON/WithLevel 等配置
// 均未生效——这是高发的 init 顺序错误，显式告警便于排查。
func warnFallbackUsage() {
	fallbackWarnOnce.Do(func() {
		fmt.Fprintln(os.Stderr, "logger: 检测到在 New()/NewZap() 之前打日志，当前回退到开发期 logger（输出到 console），"+
			"WithPath/WithOutJSON/WithLevel 等配置尚未生效——请确保在打日志前先初始化 logger。")
	})
}

func newLoggerState(root *zap.Logger, channelBases map[string]*zap.Logger, closers []io.Closer, messager Messager, asyncMsg *asyncMessager, contextFields ContextFieldsFunc, atomicLevel zap.AtomicLevel) *loggerState {
	if channelBases == nil {
		channelBases = make(map[string]*zap.Logger)
	}

	return &loggerState{
		root:          root,
		sugar:         root.Sugar(),
		messager:      messager,
		asyncMsg:      asyncMsg,
		contextFields: contextFields,
		atomicLevel:   atomicLevel,
		channelBases:  channelBases,
		closers:       closers,
		done:          make(chan struct{}),
	}
}

// currentLoggerState 返回当前活跃 loggerState 的引用，并将其引用计数 +1。
//
// 调用方必须**保证最终 defer 调用 state.release()**，否则旧 state 的资源（zap core / 文件句柄 /
// async messager 队列）永远不会被关闭——`NewZap` 的 reconfigure 路径等待引用归零才会关闭旧 state。
//
// 返回 nil 仅在两种情况：
//  1. 全局 logger 尚未初始化（业务代码在调用 NewZap 之前打日志）；
//  2. 程序已经处于关闭流程的极后期，currentState 被显式 Store(nil)（当前不会发生，预留语义）。
//
// 实现细节：CAS retry + Gosched
//
// 函数对每条日志都被调用，**是 hot path**。99.99% 的情况下：
//
//	state := currentState.Load()  // 拿到当前 state
//	state.acquire()                // 引用计数 CAS +1，成功
//	return state                   // 立即返回（这里就退出循环了，0 次 Gosched）
//
// 仅在极窄的 reconfigure 窗口（旧 state.retire() 已 CAS 成功，新 state 尚未通过
// currentState.Store(new) 公开），acquire() 会返回 false。这时 currentState.Load()
// 还能取到旧的（已 retired）state，导致循环空转。
//
// 加 runtime.Gosched() 的目的就是在这个窗口让出 P，让 reconfigure 路径有机会跑完
// currentState.Store(new)。窗口期一般在亚毫秒级，单次让步通常足够。
//
// 这里**不能用 sync.Once 或 channel 等待**：reconfigure 期间多个 goroutine 可能同时打日志，
// 它们必须各自 acquire 新 state——基于 atomic CAS 的轻量 retry 是最低开销的方案。
func currentLoggerState() *loggerState {
	for {
		state := currentState.Load()
		if state == nil {
			return nil
		}
		if state.acquire() {
			// 仅在"从未 New()"且确实用到 fallback 时告警一次；Sync() 之后的 fallback 不告警。
			if state.fallback && !loggerInitialized.Load() {
				warnFallbackUsage()
			}
			return state
		}
		// state 已 retired 但新 state 尚未通过 currentState.Store 公开。
		// 让出 P 避免 tight-loop——这段窗口极短，单次 Gosched 通常就能让出到 reconfigure 路径完成 Store。
		runtime.Gosched()
	}
}

func snapshotLoggerState() *loggerState {
	return currentState.Load()
}

func (s *loggerState) acquire() bool {
	if s == nil {
		return false
	}

	for {
		if s.retired.Load() {
			return false
		}

		refs := s.refs.Load()
		if s.refs.CompareAndSwap(refs, refs+1) {
			if s.retired.Load() {
				s.release()
				return false
			}
			return true
		}
	}
}

func (s *loggerState) release() {
	if s == nil {
		return
	}

	if s.refs.Add(-1) == 0 {
		s.signalIfDrained()
	}
}

func (s *loggerState) retire() {
	if s == nil {
		return
	}

	if s.retired.CompareAndSwap(false, true) {
		s.signalIfDrained()
	}
}

func (s *loggerState) wait() {
	if s == nil {
		return
	}

	<-s.done
}

func (s *loggerState) closeResources() {
	if s == nil {
		return
	}

	if s.asyncMsg != nil {
		s.asyncMsg.close()
	}
	if err := s.root.Sync(); err != nil {
		fmt.Fprintf(os.Stderr, "logger: sync root logger: %v\n", err)
	}
	closeClosers(s.closers)
}

func (s *loggerState) signalIfDrained() {
	if s == nil {
		return
	}

	if s.retired.Load() && s.refs.Load() == 0 {
		s.doneOnce.Do(func() {
			close(s.done)
		})
	}
}

func (s *loggerState) channelLogger(name string) *zap.Logger {
	if name == "" {
		return s.root
	}

	if logger, ok := s.channelBases[name]; ok {
		return logger
	}

	if cached, ok := s.dynamicChannel.Load(name); ok {
		if l, ok := cached.(*zap.Logger); ok {
			return l
		}
	}

	logger := s.root.With(zap.String("channel", name))

	// CAS 预留缓存 slot，确保计数不超过上限。
	for {
		cnt := s.dynamicChannelCnt.Load()
		if cnt >= maxDynamicChannels {
			return logger
		}
		if s.dynamicChannelCnt.CompareAndSwap(cnt, cnt+1) {
			break
		}
	}

	actual, loaded := s.dynamicChannel.LoadOrStore(name, logger)
	if loaded {
		// 已有缓存，释放预留的 slot。
		s.dynamicChannelCnt.Add(-1)
	}

	if l, ok := actual.(*zap.Logger); ok {
		return l
	}
	return logger
}
