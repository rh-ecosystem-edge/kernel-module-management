package http

import "crypto/tls"

func DisableHTTP2(c *tls.Config) {
	c.NextProtos = []string{"http/1.1"}
}
