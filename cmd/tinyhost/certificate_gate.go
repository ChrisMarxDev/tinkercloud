package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"time"
)

func certificateReady(ctx context.Context, host string, c *http.Client) bool {
	if c == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	u := "https://" + host + "/_tiny/auth/login"
	r, e := http.NewRequestWithContext(ctx, "GET", u, nil)
	if e != nil {
		return false
	}
	old := c.CheckRedirect
	c.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	defer func() { c.CheckRedirect = old }()
	res, e := c.Do(r)
	if e != nil {
		return false
	}
	defer res.Body.Close()
	return res.Request.URL.Scheme == "https" && res.Request.URL.Host == host && res.TLS != nil && len(res.TLS.VerifiedChains) > 0 && res.StatusCode < 500
}

var _ = tls.ConnectionState{}
