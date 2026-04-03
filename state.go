package logger

import (
	"fmt"
	"io"
	"os"
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

func currentLoggerState() *loggerState {
	for {
		state := currentState.Load()
		if state == nil {
			return nil
		}
		if state.acquire() {
			return state
		}
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
