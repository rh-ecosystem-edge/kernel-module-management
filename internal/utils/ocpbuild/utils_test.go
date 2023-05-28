package ocpbuild

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("IsBuildChanged", func() {
	b0 := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{HashAnnotation: "test0"},
		},
	}

	b1 := &buildv1.Build{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{HashAnnotation: "test1"},
		},
	}

	DescribeTable("should work as expected",
		func(b0, b1 *buildv1.Build, exp, errorExpected bool) {
			res, err := IsOCPBuildChanged(b0, b1)

			if errorExpected {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(Equal(exp))
			}

		},
		Entry(nil, &buildv1.Build{}, &buildv1.Build{}, false, true),
		Entry(nil, b0, b0, false, false),
		Entry(nil, b0, b1, true, false),
	)
})
