package log

import (
	"io"
	"log"
	"sync"

	"github.com/splitio/go-toolkit/v5/logging"
)

// Instance is an instance of log
var Instance logging.LoggerInterface

// ErrorDashboard is an instance of DashboardWriter
var ErrorDashboard = &DashboardWriter{cmutex: &sync.Mutex{}, counts: 0, messages: make([]string, 0), messagesSize: 10}

// DashboardWriter counts each call to Write method
type DashboardWriter struct {
	counts       int64
	cmutex       *sync.Mutex
	messages     []string
	messagesSize int
}

func (c *DashboardWriter) Write(p []byte) (n int, err error) {
	c.cmutex.Lock()
	c.counts++
	c.messages = append(c.messages, string(p))
	if len(c.messages) > c.messagesSize {
		c.messages = c.messages[len(c.messages)-c.messagesSize : len(c.messages)]
	}
	c.cmutex.Unlock()
	return 0, nil
}

// Counts returns the count number
func (c *DashboardWriter) Counts() int64 {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	return c.counts
}

// Messages returns the last logged messages
func (c *DashboardWriter) Messages() []string {
	c.cmutex.Lock()
	defer c.cmutex.Unlock()
	return c.messages
}

// Initialize log module
func Initialize(
	verboseWriter io.Writer,
	debugWriter io.Writer,
	infoWriter io.Writer,
	warningWriter io.Writer,
	errorWriter io.Writer,
	level int) logging.LoggerInterface {

	return logging.NewLogger(&logging.LoggerOptions{
		StandardLoggerFlags: log.Ldate | log.Ltime | log.Lshortfile,
		Prefix:              "SPLITIO-AGENT ",
		VerboseWriter:       verboseWriter,
		DebugWriter:         debugWriter,
		InfoWriter:          infoWriter,
		WarningWriter:       warningWriter,
		ErrorWriter:         errorWriter,
		LogLevel:            level,
	})
}
