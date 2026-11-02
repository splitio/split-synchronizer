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
	name     string
	addr     string
	start    func() error
	exitCode int
}

func serverAddr(host string, port int) string {
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// serveUntilShutdown starts every server and blocks until block returns, which is a normal shutdown (nil),
// or a server fails with an error other than http.ErrServerClosed. A failure is returned as *common.InitializationError
// so the caller exits with the server's exit code instead of blocking with no listener.
func serveUntilShutdown(block func(), servers ...serverSpec) error {
	// Buffered so a later failure does not block its goroutine after we return.
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
