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
	total   int64
	mutex   sync.Mutex
}

func newHistoricBuffer(enabled bool, size int) *historicBuffer {
	return &historicBuffer{
		enabled: enabled,
		buffer:  make([]string, size),
		start:   0,
		count:   0,
		total:   0,
	}
}

func (b *historicBuffer) record(message string) {
	if !b.enabled {
		return
	}

	b.mutex.Lock()
	defer b.mutex.Unlock()

	b.total++

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

func (b *historicBuffer) totalCount() int64 {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	return b.total
}

// HistoricLogger defines the interface for a logger that allows keeping the last N messages buffered and querying them
type HistoricLogger interface {
	logging.LoggerInterface
	Messages(level int) []string
	TotalCount(level int) int64
}

// NewHistoricLoggerWrapper constructs a new historic logger
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

// HistoricLoggerWrapper is an implementation of the HistoricLogger interface
type HistoricLoggerWrapper struct {
	logging.LoggerInterface
	buffers [logLevelCount]historicBuffer
}

func (l *HistoricLoggerWrapper) toHistory(level int, m ...interface{}) {
	bufferIndex := level - logging.LevelError
	l.buffers[bufferIndex].record(fmt.Sprint(m...))
}

// Error writes a log message with Error level
func (l *HistoricLoggerWrapper) Error(msg ...interface{}) {
	l.toHistory(logging.LevelError, msg...)
	l.LoggerInterface.Error(msg...)
}

// Warning writes a log message with Warning level
func (l *HistoricLoggerWrapper) Warning(msg ...interface{}) {
	l.toHistory(logging.LevelWarning, msg...)
	l.LoggerInterface.Warning(msg...)
}

// Info writes a log message with info level
func (l *HistoricLoggerWrapper) Info(msg ...interface{}) {
	l.toHistory(logging.LevelInfo, msg...)
	l.LoggerInterface.Info(msg...)
}

// Debug writes a log message with debug level
func (l *HistoricLoggerWrapper) Debug(msg ...interface{}) {
	l.toHistory(logging.LevelDebug, msg...)
	l.LoggerInterface.Debug(msg...)
}

// Verbose writes a log message with verbose level
func (l *HistoricLoggerWrapper) Verbose(msg ...interface{}) {
	l.toHistory(logging.LevelVerbose, msg...)
	l.LoggerInterface.Verbose(msg...)
}

// Messages returns the buffered messages for a specific level
func (l *HistoricLoggerWrapper) Messages(level int) []string {
	bufferIndex := level - logging.LevelError
	return l.buffers[bufferIndex].messages()
}

// TotalCount returns the total number of messages logged for a specific level
func (l *HistoricLoggerWrapper) TotalCount(level int) int64 {
	bufferIndex := level - logging.LevelError
	return l.buffers[bufferIndex].totalCount()
}

var _ HistoricLogger = (*HistoricLoggerWrapper)(nil)
