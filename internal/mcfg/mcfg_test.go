package mcfg

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("GenerateIgnition", func() {
	const (
		name                   = "name"
		mcpRef                 = "mcpRef"
		kernelModuleName       = "testKernelModuleName"
		inTreeKernelModuleName = "testInTreeKernelModuleName"
		firmwareFilesPath      = "/opt/lib/test/firmware"
		imageName              = "quay.io/project/repo:some-tag12"
		workerImage            = "some-worker-image"
	)

	// unit performs verification of the MC output vs the manually created
	// machineconfig-test.yaml. This yaml is created by manually running bas64
	// encoding on testdata/pull-image-test.sh and testdata/replace-kernel-module-test.sh
	// and setting the values into the testdata/machineconfig-test.yaml
	It("verify correct ignition output", func() {
		mcfgAPI := NewMCFG()

		_, yamlRes, err := mcfgAPI.GenerateIgnition(imageName, kernelModuleName, inTreeKernelModuleName, firmwareFilesPath, workerImage, "test-mc")

		Expect(err).ToNot(HaveOccurred())
		expectedRes, err := os.ReadFile("testdata/ignition-test.yaml")
		Expect(err).ToNot(HaveOccurred())

		if string(expectedRes) != yamlRes {
			fmt.Printf("<%s>\n", yamlRes)
			fmt.Printf("<%s>\n", expectedRes)
		}
		Expect(string(yamlRes)).To(Equal(string(expectedRes)))
	})
})
