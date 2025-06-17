package webhook

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta2 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
)

var _ = Describe("validatePreflight", func() {
	It("should fail with invalid kernel version", func() {
		pv := &kmmv1beta2.PreflightValidationOCP{
			Spec: kmmv1beta2.PreflightValidationOCPSpec{
				KernelVersion: "test",
			},
		}
		_, err := validatePreflight(pv)
		Expect(err).To(HaveOccurred())
	})

	It("should pass with valid kernel version", func() {
		pv := &kmmv1beta2.PreflightValidationOCP{
			Spec: kmmv1beta2.PreflightValidationOCPSpec{
				KernelVersion: "6.0.15-300.fc37.x86_64",
			},
		}
		_, err := validatePreflight(pv)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("PreflightValidationValidator", func() {
	v := NewPreflightValidationValidator(GinkgoLogr)
	ctx := context.TODO()

	It("ValidateDelete should return not implemented", func() {
		_, err := v.ValidateDelete(ctx, nil)
		Expect(err).To(Equal(NotImplemented))
	})
})
