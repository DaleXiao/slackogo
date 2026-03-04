package transport

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"

	utls "github.com/refraction-networking/utls"
)

// NewEdgeTransport returns an http.Transport that mimics Microsoft Edge's
// TLS ClientHello fingerprint via utls.
//
// Uses HTTP/1.1 only — utls DialTLSContext is not compatible with Go's
// built-in HTTP/2 transport (causes EOF errors). HTTP/1.1 is fine for
// Slack API calls and page loads.
func NewEdgeTransport() *http.Transport {
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
		// Explicitly disable HTTP/2 — utls + Go http2 causes EOF
		TLSClientConfig:   &tls.Config{},
		ForceAttemptHTTP2: false,
	}
}
