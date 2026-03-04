package transport

import (
	"context"
	"fmt"
	"net"
	"net/http"

	utls "github.com/refraction-networking/utls"
)

// NewChromeTransport returns an http.Transport with Chrome/Edge TLS fingerprint.
// Uses HTTP/1.1 only — Go's http2 package is incompatible with utls DialTLSContext.
func NewChromeTransport() *http.Transport {
	return &http.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}

			dialer := &net.Dialer{}
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, fmt.Errorf("tcp dial: %w", err)
			}

			tlsConn := utls.UClient(conn, &utls.Config{
				ServerName: host,
			}, utls.HelloChrome_Auto)

			if err := tlsConn.HandshakeContext(ctx); err != nil {
				conn.Close()
				return nil, fmt.Errorf("tls handshake: %w", err)
			}

			return tlsConn, nil
		},
		// HTTP/1.1 only — utls + Go http2 causes EOF
		ForceAttemptHTTP2: false,
	}
}
