package webhook

import (
	"context"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("validateInitramfsContainerImage", func() {
	DescribeTable("validates the tagged image reference",
		func(image string, valid bool) {
			err := validateInitramfsContainerImage(image)
			if valid {
				Expect(err).NotTo(HaveOccurred())
			} else {
				Expect(err).To(HaveOccurred())
			}
		},
		Entry("repository and tag", "registry.example.com/kmods/nic:v1", true),
		Entry("registry port", "registry.example.com:5000/kmods/nic:dev", true),
		Entry("short repository and tag", "nic:v1", true),
		Entry("missing tag", "registry.example.com/kmods/nic", false),
		Entry("digest", "registry.example.com/kmods/nic@sha256:"+strings.Repeat("a", 64), false),
		Entry("tag and digest", "registry.example.com/kmods/nic:v1@sha256:"+strings.Repeat("a", 64), false),
		Entry("uppercase repository", "registry.example.com/Kmods/nic:v1", false),
		Entry("empty tag", "registry.example.com/kmods/nic:", false),
		Entry("whitespace", "registry.example.com/kmods/nic:v1 bad", false),
	)
})

var _ = Describe("validateInitramfsModule", func() {
	validIRM := func() *kmmv1beta1.InitramfsModule {
		return &kmmv1beta1.InitramfsModule{
			ObjectMeta: metav1.ObjectMeta{Name: "nic", Namespace: "kmm"},
			Spec: kmmv1beta1.InitramfsModuleSpec{
				ModuleNames:    []string{"nic"},
				ContainerImage: "registry.example.com/kmods/nic:v1",
			},
		}
	}

	It("accepts a valid resource", func() {
		Expect(validateInitramfsModule(validIRM())).To(Succeed())
	})

	DescribeTable("rejects invalid module name lists",
		func(moduleNames, omit []string) {
			irm := validIRM()
			irm.Spec.ModuleNames = moduleNames
			irm.Spec.Omit = omit
			Expect(validateInitramfsModule(irm)).To(HaveOccurred())
		},
		Entry("empty moduleNames", []string{}, nil),
		Entry("invalid moduleNames entry", []string{"nic.ko"}, nil),
		Entry("duplicate moduleNames entry", []string{"nic", "nic"}, nil),
		Entry("invalid omit entry", []string{"nic"}, []string{"ahci.ko"}),
		Entry("duplicate omit entry", []string{"nic"}, []string{"ahci", "ahci"}),
	)

	It("validates sign paths relative to the default dirName", func() {
		irm := validIRM()
		irm.Spec.Sign = &kmmv1beta1.Sign{FilesToSign: []string{"/usr/lib/nic.ko"}}

		Expect(validateInitramfsModule(irm)).To(MatchError(ContainSubstring("failed to validate sign")))
	})

	It("validates sign paths relative to an explicit dirName", func() {
		irm := validIRM()
		irm.Spec.DirName = "/modules"
		irm.Spec.Sign = &kmmv1beta1.Sign{FilesToSign: []string{"/modules/lib/nic.ko"}}

		Expect(validateInitramfsModule(irm)).To(Succeed())
	})
})

var _ = Describe("InitramfsModuleValidator singleton validation", func() {
	newValidator := func(objects ...*kmmv1beta1.InitramfsModule) *InitramfsModuleValidator {
		scheme := runtime.NewScheme()
		Expect(kmmv1beta1.AddToScheme(scheme)).To(Succeed())

		return &InitramfsModuleValidator{
			logger: GinkgoLogr,
			client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(objectsToRuntimeObjects(objects)...).Build(),
		}
	}

	validIRM := func(namespace, name string) *kmmv1beta1.InitramfsModule {
		return &kmmv1beta1.InitramfsModule{
			ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
			Spec: kmmv1beta1.InitramfsModuleSpec{
				ModuleNames:    []string{"nic"},
				ContainerImage: "registry.example.com/kmods/nic:v1",
			},
		}
	}

	It("allows the first create", func() {
		validator := newValidator()

		_, err := validator.ValidateCreate(context.Background(), validIRM("kmm", "nic"))
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects a second create and identifies the existing object", func() {
		validator := newValidator(validIRM("other", "existing"))

		_, err := validator.ValidateCreate(context.Background(), validIRM("kmm", "nic"))
		Expect(err).To(MatchError(ContainSubstring("other/existing")))
	})

	It("allows an update of the existing object", func() {
		existing := validIRM("kmm", "nic")
		validator := newValidator(existing)

		updated := existing.DeepCopy()
		updated.Spec.Omit = []string{"ahci"}
		_, err := validator.ValidateUpdate(context.Background(), existing, updated)
		Expect(err).NotTo(HaveOccurred())
	})

	It("rejects an update when another object exists", func() {
		existing := validIRM("kmm", "nic")
		validator := newValidator(existing, validIRM("other", "existing"))

		_, err := validator.ValidateUpdate(context.Background(), existing, existing.DeepCopy())
		Expect(err).To(MatchError(ContainSubstring("other/existing")))
	})
})

func objectsToRuntimeObjects(objects []*kmmv1beta1.InitramfsModule) []client.Object {
	result := make([]client.Object, len(objects))
	for i, object := range objects {
		result[i] = object
	}
	return result
}
