package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

// NewChromeTransport returns an http.RoundTripper with Chrome TLS fingerprint
// and HTTP/2 support. Uses golang.org/x/net/http2.Transport which handles
// HTTP/2 framing over any net.Conn (doesn't need Go's built-in TLS).
func NewChromeTransport() http.RoundTripper {
	return &http2.Transport{
		DialTLSContext: func(ctx context.Context, network, addr string, _ *tls.Config) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				host = addr
			}

			conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
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
	}
}
