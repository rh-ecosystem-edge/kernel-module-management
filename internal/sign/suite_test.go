package sign

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/test"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	var err error

	_, err = test.TestScheme()
	Expect(err).NotTo(HaveOccurred())

	RunSpecs(t, "Sign Suite")
}
