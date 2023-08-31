package ca

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/test"
	"k8s.io/apimachinery/pkg/runtime"
)

var scheme *runtime.Scheme

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)

	var err error

	scheme, err = test.TestScheme()
	Expect(err).NotTo(HaveOccurred())

	RunSpecs(t, "CA Suite")
}
