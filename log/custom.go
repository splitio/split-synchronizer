package log

import (
	"fmt"
	"sync"

	"github.com/splitio/go-toolkit/v5/logging"
)

const logLevelCount = (logging.LevelVerbose - logging.LevelError) + 1

type historicBuffer struct {
	enabled bool
	buffer  []string
	start   int
	count   int
	mutex   sync.Mutex
}

func newHistoricBuffer(enabled bool, size int) *historicBuffer {
	return &historicBuffer{
		enabled: enabled,
		buffer:  make([]string, size),
		start:   0,
		count:   0,
	}
}

func (b *historicBuffer) record(message string) {
	if !b.enabled {
		return
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	pos := (b.start + b.count) % len(b.buffer)
	b.buffer[pos] = message
	if b.count < len(b.buffer) {
		// if we haven't filled the buffer we keep incrementing the count
		b.count++
	} else {
		// once the buffer is full, the count becomes fixed, and each time we add a message, we shift the start
		b.start = b.start % len(b.buffer)
	}
}

func (b *historicBuffer) messages() []string {
	if !b.enabled {
		return []string{}
	}

	messages := make([]string, 0, b.count)

	b.mutex.Lock()
	defer b.mutex.Unlock()

	for idx, remaining := b.start, b.count; remaining > 0; idx, remaining = (idx+1)%len(b.buffer), remaining-1 {
		messages = append(messages, b.buffer[idx])
	}
	return messages
}

type HistoricLogger interface {
	logging.LoggerInterface
	Messages(level int) []string
}

func NewHistoricLoggerWrapper(l logging.LoggerInterface, enabled [logLevelCount]bool, size int) *HistoricLoggerWrapper {
	return &HistoricLoggerWrapper{
		LoggerInterface: l,
		buffers: [logLevelCount]historicBuffer{
			*newHistoricBuffer(enabled[logging.LevelError-logging.LevelError], size),
			*newHistoricBuffer(enabled[logging.LevelWarning-logging.LevelError], size),
			*newHistoricBuffer(enabled[logging.LevelInfo-logging.LevelError], size),
			*newHistoricBuffer(enabled[logging.LevelDebug-logging.LevelError], size),
			*newHistoricBuffer(enabled[logging.LevelVerbose-logging.LevelError], size),
		},
	}
}

type HistoricLoggerWrapper struct {
	logging.LoggerInterface
	buffers [logLevelCount]historicBuffer
}

func (l *HistoricLoggerWrapper) toHistory(level int, m ...interface{}) {
	bufferIndex := level - logging.LevelError
	l.buffers[bufferIndex].record(fmt.Sprint(m...))
}

func (l *HistoricLoggerWrapper) Error(msg ...interface{}) {
	l.toHistory(logging.LevelError, msg...)
	l.LoggerInterface.Error(msg...)
}

func (l *HistoricLoggerWrapper) Warning(msg ...interface{}) {
	l.toHistory(logging.LevelWarning, msg...)
	l.LoggerInterface.Warning(msg...)
}

func (l *HistoricLoggerWrapper) Info(msg ...interface{}) {
	l.toHistory(logging.LevelInfo, msg...)
	l.LoggerInterface.Info(msg...)
}

func (l *HistoricLoggerWrapper) Debug(msg ...interface{}) {
	l.toHistory(logging.LevelDebug, msg...)
	l.LoggerInterface.Debug(msg...)
}

func (l *HistoricLoggerWrapper) Verbose(msg ...interface{}) {
	l.toHistory(logging.LevelVerbose, msg...)
	l.LoggerInterface.Verbose(msg...)
}

func (l *HistoricLoggerWrapper) Messages(level int) []string {
	bufferIndex := level - logging.LevelError
	return l.buffers[bufferIndex].messages()
}

var _ HistoricLogger = (*HistoricLoggerWrapper)(nil)
