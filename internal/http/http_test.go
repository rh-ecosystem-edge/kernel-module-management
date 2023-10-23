package http

import (
	"crypto/tls"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DisableHTTP2", func() {
	It("should only contain HTTP/1.1", func() {
		cfg := &tls.Config{}

		DisableHTTP2(cfg)

		Expect(cfg.NextProtos).To(Equal([]string{"http/1.1"}))
	})
})
