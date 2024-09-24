package mcproducer

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"
)

var _ = Describe("ProduceMachineConfig", func() {
	const (
		name                   = "name"
		mcpRef                 = "mcpRef"
		kernelModuleName       = "testKernelModuleName"
		inTreeKernelModuleName = "testInTreeKernelModuleName"
		firmwareFilesPath      = "/opt/lib/test/firmware"
	)

	It("image name format is invalid", func() {
		imageName := "quay.io/project/repo@sha2561f5f1ae25db67aa82707e1b1dc96c8a53ef7094f320b7eeaef12be9a13fa251d"

		res, err := ProduceMachineConfig(name, mcpRef, imageName, kernelModuleName, "", firmwareFilesPath, "")

		Expect(err).To(HaveOccurred())
		Expect(res).To(Equal(""))

	})

	// unit performs verification of the MC output vs the manually created
	// machineconfig-test.yaml. This yaml is created by manually running bas64
	// encoding on testdata/pull-image-test.sh and testdata/replace-kernel-module-test.sh
	// and setting the values into the testdata/machineconfig-test.yaml
	It("verify correct mco output", func() {
		imageName := "quay.io/project/repo:some-tag12"

		res, err := ProduceMachineConfig(name, mcpRef, imageName, kernelModuleName, inTreeKernelModuleName, firmwareFilesPath, "")

		Expect(err).ToNot(HaveOccurred())
		expectedRes, err := os.ReadFile("testdata/machineconfig-test.yaml")
		Expect(err).ToNot(HaveOccurred())
		if string(expectedRes) != res {
			fmt.Printf("<%s>\n", res)
			fmt.Printf("<%s>\n", expectedRes)
		}
		Expect(res).To(Equal(string(expectedRes)))
	})
})
