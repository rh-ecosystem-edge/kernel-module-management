package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	openapivi "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/preflight"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	"go.uber.org/mock/gomock"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	dtkImageReference = "dtkImageReference"
)

var _ = Describe("PreflightValidationOCPReconciler_getDTKFromImage", func() {
	var (
		ctrl            *gomock.Controller
		clnt            *client.MockClient
		mockSU          *preflight.MockOCPStatusUpdater
		mockRegistry    *registry.MockRegistry
		mockAuthFactory *auth.MockRegistryAuthGetterFactory
		mockAuth        *auth.MockRegistryAuthGetter
		mockSKODM       *syncronizedmap.MockKernelOsDtkMapping
		ctx             context.Context
		pro             *PreflightValidationOCPReconciler
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		clnt = client.NewMockClient(ctrl)
		mockSU = preflight.NewMockOCPStatusUpdater(ctrl)
		mockRegistry = registry.NewMockRegistry(ctrl)
		mockAuthFactory = auth.NewMockRegistryAuthGetterFactory(ctrl)
		mockAuth = auth.NewMockRegistryAuthGetter(ctrl)
		mockSKODM = syncronizedmap.NewMockKernelOsDtkMapping(ctrl)
		ctx = context.Background()
		mockAuthFactory.EXPECT().NewClusterAuthGetter().Return(mockAuth)
		pro = NewPreflightValidationOCPReconciler(clnt, nil, mockRegistry, mockAuthFactory, mockSKODM, mockSU, scheme)
	})

	AfterEach(func() {
		ctrl.Finish()
	})

	Describe("getDTKFromImage", func() {
		var releaseOCPData openapivi.ImageStream
		BeforeEach(func() {
			releaseOCPData = openapivi.ImageStream{
				Spec: openapivi.ImageStreamSpec{
					Tags: []openapivi.TagReference{
						{
							Name: driverToolkitSpecName,
							From: &corev1.ObjectReference{Name: dtkImageReference},
						},
						{
							Name: "some other component",
							From: &corev1.ObjectReference{Name: "some other component image"},
						},
					},
				},
			}
		})

		It("good flow", func() {
			releaseImageData, err := json.Marshal(&releaseOCPData)
			Expect(err).To(BeNil())
			gomock.InOrder(
				mockRegistry.EXPECT().LastLayer(ctx, "ocpReleaseImage", nil, mockAuth).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, releaseManifestImagesRefFile).Return(releaseImageData, nil),
			)

			res, err := pro.getDTKFromImage(ctx, "ocpReleaseImage")

			Expect(err).To(BeNil())
			Expect(res).To(Equal(dtkImageReference))
		})

		It("dtk data invalid", func() {
			releaseOCPData.Spec.Tags[0].From = nil
			releaseImageData, err := json.Marshal(&releaseOCPData)
			Expect(err).To(BeNil())
			gomock.InOrder(
				mockRegistry.EXPECT().LastLayer(ctx, "ocpReleaseImage", nil, mockAuth).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, releaseManifestImagesRefFile).Return(releaseImageData, nil),
			)

			_, err = pro.getDTKFromImage(ctx, "ocpReleaseImage")

			Expect(err).To(HaveOccurred())
		})

		It("dtk data missing", func() {
			releaseOCPData.Spec.Tags[0].Name = "some name"
			releaseImageData, err := json.Marshal(&releaseOCPData)
			Expect(err).To(BeNil())
			gomock.InOrder(
				mockRegistry.EXPECT().LastLayer(ctx, "ocpReleaseImage", nil, mockAuth).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, releaseManifestImagesRefFile).Return(releaseImageData, nil),
			)

			_, err = pro.getDTKFromImage(ctx, "ocpReleaseImage")

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("getKernelVersionAndOSFromDTK", func() {
		var dtkReleaseData dtkRelease

		BeforeEach(func() {
			dtkReleaseData = dtkRelease{KernelVersion: "kernelVersion", RTKernelVersion: "rtKernelVersion", RHELVersion: "rhelVersion"}
		})

		It("good flow", func() {
			digests := []string{"digest1", "digest2"}
			dtkDataBytes, err := json.Marshal(&dtkReleaseData)
			Expect(err).To(BeNil())
			gomock.InOrder(
				mockRegistry.EXPECT().GetLayersDigests(ctx, "dtkImage", nil, mockAuth).Return(digests, &registry.RepoPullConfig{}, nil),
				mockRegistry.EXPECT().GetLayerByDigest(digests[1], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(dtkDataBytes, nil),
			)

			res1, res2, res3, err := pro.getKernelVersionAndOSFromDTK(ctx, "dtkImage")

			Expect(err).To(BeNil())
			Expect(res1).To(Equal("kernelVersion"))
			Expect(res2).To(Equal("rtKernelVersion"))
			Expect(res3).To(Equal("rhelVersion"))
		})

		It("etc/driver-toolkit-release.json not present in dtk", func() {
			digests := []string{"digest1", "digest2"}
			gomock.InOrder(
				mockRegistry.EXPECT().GetLayersDigests(ctx, "dtkImage", nil, mockAuth).Return(digests, &registry.RepoPullConfig{}, nil),
				mockRegistry.EXPECT().GetLayerByDigest(digests[1], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(nil, fmt.Errorf("some error")),
				mockRegistry.EXPECT().GetLayerByDigest(digests[0], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(nil, fmt.Errorf("some error")),
			)

			_, _, _, err := pro.getKernelVersionAndOSFromDTK(ctx, "dtkImage")

			Expect(err).To(HaveOccurred())
		})

		It("etc/driver-toolkit-release.json invalid format", func() {
			digests := []string{"digest1", "digest2"}
			dtkReleaseData.KernelVersion = ""
			dtkDataBytes, err := json.Marshal(&dtkReleaseData)
			Expect(err).To(BeNil())
			gomock.InOrder(
				mockRegistry.EXPECT().GetLayersDigests(ctx, "dtkImage", nil, mockAuth).Return(digests, &registry.RepoPullConfig{}, nil),
				mockRegistry.EXPECT().GetLayerByDigest(digests[1], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(dtkDataBytes, nil),
			)

			_, _, _, err = pro.getKernelVersionAndOSFromDTK(ctx, "dtkImage")

			Expect(err).To(HaveOccurred())
		})
	})

	Describe("preparePreflightValidation", func() {
		var (
			releaseOCPData openapivi.ImageStream
			dtkReleaseData dtkRelease
			pvo            v1beta2.PreflightValidationOCP
		)
		BeforeEach(func() {
			dtkReleaseData = dtkRelease{KernelVersion: "kernelVersion", RTKernelVersion: "rtKernelVersion", RHELVersion: "rhelVersion"}
			releaseOCPData = openapivi.ImageStream{
				Spec: openapivi.ImageStreamSpec{
					Tags: []openapivi.TagReference{
						{
							Name: driverToolkitSpecName,
							From: &corev1.ObjectReference{Name: dtkImageReference},
						},
						{
							Name: "some other component",
							From: &corev1.ObjectReference{Name: "some other component image"},
						},
					},
				},
			}
			pvo = v1beta2.PreflightValidationOCP{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "pvo name",
					Namespace: "pvo namespace",
				},
				Spec: v1beta2.PreflightValidationOCPSpec{
					ReleaseImage:   "ocpReleaseImage",
					PushBuiltImage: true,
				},
			}
		})

		It("good flow", func() {
			digests := []string{"digest1", "digest2"}
			releaseImageData, err := json.Marshal(&releaseOCPData)
			Expect(err).To(BeNil())
			dtkDataBytes, err := json.Marshal(&dtkReleaseData)
			Expect(err).To(BeNil())
			gomock.InOrder(
				mockRegistry.EXPECT().LastLayer(ctx, "ocpReleaseImage", nil, mockAuth).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, releaseManifestImagesRefFile).Return(releaseImageData, nil),
				mockRegistry.EXPECT().GetLayersDigests(ctx, dtkImageReference, nil, mockAuth).Return(digests, &registry.RepoPullConfig{}, nil),
				mockRegistry.EXPECT().GetLayerByDigest(digests[1], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(dtkDataBytes, nil),
				mockSKODM.EXPECT().GetImage(dtkReleaseData.KernelVersion).Return("", fmt.Errorf("some error")),
				mockSKODM.EXPECT().SetNodeInfo(dtkReleaseData.KernelVersion, dtkReleaseData.RHELVersion),
				mockSKODM.EXPECT().SetImageStreamInfo(dtkReleaseData.RHELVersion, dtkImageReference),
			)

			pv, err := pro.preparePreflightValidation(ctx, &pvo)
			Expect(err).To(BeNil())
			Expect(pv.Spec.KernelVersion).To(Equal(dtkReleaseData.KernelVersion))
			Expect(pv.Spec.PushBuiltImage).To(Equal(pvo.Spec.PushBuiltImage))
		})

		It("good flow with RT kernel", func() {
			dtkReleaseData.RTKernelVersion = ""
			digests := []string{"digest1", "digest2"}
			releaseImageData, err := json.Marshal(&releaseOCPData)
			Expect(err).To(BeNil())
			dtkDataBytes, err := json.Marshal(&dtkReleaseData)
			Expect(err).To(BeNil())
			pvo.Spec.UseRTKernel = true
			gomock.InOrder(
				mockRegistry.EXPECT().LastLayer(ctx, "ocpReleaseImage", nil, mockAuth).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, releaseManifestImagesRefFile).Return(releaseImageData, nil),
				mockRegistry.EXPECT().GetLayersDigests(ctx, dtkImageReference, nil, mockAuth).Return(digests, &registry.RepoPullConfig{}, nil),
				mockRegistry.EXPECT().GetLayerByDigest(digests[1], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(dtkDataBytes, nil),
			)

			_, err = pro.preparePreflightValidation(ctx, &pvo)
			Expect(err).NotTo(BeNil())
		})

		It("error flow, RT kernel missing", func() {
			digests := []string{"digest1", "digest2"}
			releaseImageData, err := json.Marshal(&releaseOCPData)
			Expect(err).To(BeNil())
			dtkDataBytes, err := json.Marshal(&dtkReleaseData)
			Expect(err).To(BeNil())
			pvo.Spec.UseRTKernel = true
			gomock.InOrder(
				mockRegistry.EXPECT().LastLayer(ctx, "ocpReleaseImage", nil, mockAuth).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, releaseManifestImagesRefFile).Return(releaseImageData, nil),
				mockRegistry.EXPECT().GetLayersDigests(ctx, dtkImageReference, nil, mockAuth).Return(digests, &registry.RepoPullConfig{}, nil),
				mockRegistry.EXPECT().GetLayerByDigest(digests[1], &registry.RepoPullConfig{}).Return(nil, nil),
				mockRegistry.EXPECT().GetHeaderDataFromLayer(nil, driverToolkitJSONFilePath).Return(dtkDataBytes, nil),
				mockSKODM.EXPECT().GetImage(dtkReleaseData.RTKernelVersion).Return("", fmt.Errorf("some error")),
				mockSKODM.EXPECT().SetNodeInfo(dtkReleaseData.RTKernelVersion, dtkReleaseData.RHELVersion),
				mockSKODM.EXPECT().SetImageStreamInfo(dtkReleaseData.RHELVersion, dtkImageReference),
			)

			pv, err := pro.preparePreflightValidation(ctx, &pvo)
			Expect(err).To(BeNil())
			Expect(pv.Spec.KernelVersion).To(Equal(dtkReleaseData.RTKernelVersion))
			Expect(pv.Spec.PushBuiltImage).To(Equal(pvo.Spec.PushBuiltImage))
		})
	})
})
