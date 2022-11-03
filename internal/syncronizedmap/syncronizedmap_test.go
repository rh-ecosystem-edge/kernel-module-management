package syncronizedmap

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// testing multiple methods at once because some of them doesn't have much to test
// the the others are easier to test using other methods.
var _ = Describe("SetNodeInfo+SetImageStreamInfo+GetImage", func() {

	const (
		kernelVersion  = "kernel-1.2.3"
		osImageVersion = "411.86.202210072320-0"
		dtkImage       = "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:111"
	)

	var (
		kodm KernelOsDtkMapping
	)

	BeforeEach(func() {
		kodm = NewKernelOsDtkMapping()
	})

	It("should return an error if the kernel doesn't exist in the map", func() {

		_, err := kodm.GetImage("kernel-non-existing")

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if the kernel exist in the map but the OS mapping doesn't", func() {

		kodm.SetNodeInfo(kernelVersion, osImageVersion)

		_, err := kodm.GetImage(kernelVersion)

		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {

		kodm.SetNodeInfo(kernelVersion, osImageVersion)
		kodm.SetImageStreamInfo(osImageVersion, dtkImage)

		image, err := kodm.GetImage(kernelVersion)

		Expect(err).NotTo(HaveOccurred())
		Expect(image).To(Equal(dtkImage))
	})
})
