package sync

import (
	"context"
	"net"
	"net/http"
	"net/http/cookiejar"
	"time"

	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
	"golang.org/x/net/publicsuffix"
)

// chromeSecHeaders are the extra Fetch/sec-ch-ua headers Chrome sends on every
// request. Akamai checks for their presence as part of bot scoring.
var chromeSecHeaders = map[string]string{
	"sec-ch-ua":          `"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`,
	"sec-ch-ua-mobile":   "?0",
	"sec-ch-ua-platform": `"Windows"`,
	"sec-fetch-dest":     "document",
	"sec-fetch-mode":     "navigate",
	"sec-fetch-site":     "none",
	"sec-fetch-user":     "?1",
	"Upgrade-Insecure-Requests": "1",
}

// newChromeClient returns an *http.Client whose TLS handshake is byte-for-byte
// identical to Chrome's (via utls), with full HTTP/2 support and a cookie jar
// so session cookies persist across requests within the same sync run.
func newChromeClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		DialTLSContext: dialChromeTLS,
	}
	// ConfigureTransport wires in HTTP/2 support. When utls negotiates "h2"
	// via ALPN, http.Transport reads the NegotiatedProtocol from UConn's
	// ConnectionState() and routes through the h2 RoundTripper automatically.
	http2.ConfigureTransport(transport) //nolint:errcheck

	jar, _ := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})

	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
		Jar:       jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			for k, v := range browserHeaders {
				req.Header.Set(k, v)
			}
			for k, v := range chromeSecHeaders {
				req.Header.Set(k, v)
			}
			return nil
		},
	}
}

// dialChromeTLS dials a TLS connection impersonating Chrome's Client Hello.
func dialChromeTLS(ctx context.Context, network, addr string) (net.Conn, error) {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}

	conn, err := (&net.Dialer{}).DialContext(ctx, network, addr)
	if err != nil {
		return nil, err
	}

	uconn := utls.UClient(conn, &utls.Config{ServerName: host}, utls.HelloChrome_Auto)
	if err := uconn.HandshakeContext(ctx); err != nil {
		conn.Close()
		return nil, err
	}

	return uconn, nil
}
