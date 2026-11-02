package proxy

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/splitio/split-synchronizer/v5/splitio/common"
)

// serverSpec describes a listener started by serveUntilShutdown.
type serverSpec struct {
	name     string       // name used in error messages, e.g. "admin server"
	addr     string       // host:port the server listens on, used in error messages
	start    func() error // blocks while serving and returns the listen/serve error
	exitCode int          // process exit code reported when this server fails to start
}

// serverAddr returns the host:port string for a listener.
func serverAddr(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// serveUntilShutdown starts every server and blocks until block returns, which is a normal shutdown (nil),
// or a server fails with an error other than http.ErrServerClosed. A failure is returned as *common.InitializationError
// so the caller exits with the server's exit code instead of blocking with no listener.
func serveUntilShutdown(block func(), servers ...serverSpec) error {
	// Buffered so a server that fails after the first one has already been reported does not leak its goroutine.
	failures := make(chan error, len(servers))
	for _, s := range servers {
		go func(s serverSpec) {
			err := s.start()
			if err == nil || errors.Is(err, http.ErrServerClosed) {
				return
			}
			failures <- common.NewInitError(fmt.Errorf("%s failed on %s: %w", s.name, s.addr, err), s.exitCode)
		}(s)
	}

	done := make(chan struct{})
	go func() {
		block()
		close(done)
	}()

	select {
	case err := <-failures:
		return err
	case <-done:
		return nil
	}
}
