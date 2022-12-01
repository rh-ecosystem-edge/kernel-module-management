package buildconfig

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	kmmbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
)

var _ = Describe("Manager_Sync", func() {
	const (
		containerImage = "some-image-name:tag"
		moduleName     = "some-module-names"
		namespace      = "some-namespace"
		targetKernel   = "target-kernel"
	)

	var (
		mockKubeClient            *client.MockClient
		mockMaker                 *MockMaker
		mockOpenShiftBuildsHelper *MockOpenShiftBuildsHelper
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		mockMaker = NewMockMaker(ctrl)
		mockOpenShiftBuildsHelper = NewMockOpenShiftBuildsHelper(ctrl)
	})

	ctx := context.Background()

	It("should create a Build when none is present", func() {
		const (
			buildName      = "some-build-config"
			repoSecretName = "repo-secret"
		)

		By("Authenticating with a secret")

		mod := kmmv1beta1.Module{
			ObjectMeta: metav1.ObjectMeta{
				Name:      moduleName,
				Namespace: namespace,
			},
			Spec: kmmv1beta1.ModuleSpec{
				ImageRepoSecret: &v1.LocalObjectReference{Name: repoSecretName},
			},
		}

		pullOptions := kmmv1beta1.PullOptions{}

		buildCfg := kmmv1beta1.Build{
			Pull: pullOptions,
		}

		mapping := kmmv1beta1.KernelMapping{
			Build:          &buildCfg,
			ContainerImage: containerImage,
		}

		m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper)

		build := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: buildName},
		}

		gomock.InOrder(
			mockMaker.EXPECT().MakeBuildTemplate(ctx, mod, mapping, targetKernel, containerImage, true).Return(&build, nil),
			mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, mod, targetKernel).Return(nil, errNoMatchingBuild),
			mockKubeClient.EXPECT().Create(ctx, &build),
		)

		res, err := m.Sync(ctx, mod, mapping, targetKernel, mapping.ContainerImage, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Status).To(BeEquivalentTo(kmmbuild.StatusCreated))
		Expect(res.Requeue).To(BeTrue())
	})

	DescribeTable(
		"should return the Build status when a Build is present",
		func(phase buildv1.BuildPhase, expectedResult kmmbuild.Result, expectError bool) {
			const buildName = "some-build"

			By("Authenticating with the ServiceAccount's pull secret")

			mod := kmmv1beta1.Module{
				ObjectMeta: metav1.ObjectMeta{
					Name:      moduleName,
					Namespace: namespace,
				},
			}

			pullOptions := kmmv1beta1.PullOptions{}

			buildCfg := kmmv1beta1.Build{
				Pull: pullOptions,
			}

			mapping := kmmv1beta1.KernelMapping{
				Build:          &buildCfg,
				ContainerImage: containerImage,
			}

			m := NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper)

			build := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Name:        buildName,
					Annotations: map[string]string{buildHashAnnotation: "some hash"},
				},
				Status: buildv1.BuildStatus{Phase: phase},
			}

			gomock.InOrder(
				mockMaker.EXPECT().MakeBuildTemplate(ctx, mod, mapping, targetKernel, containerImage, true).Return(&build, nil),
				mockOpenShiftBuildsHelper.EXPECT().GetBuild(ctx, mod, targetKernel).Return(&build, nil),
			)

			res, err := m.Sync(ctx, mod, mapping, targetKernel, mapping.ContainerImage, true)

			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeEquivalentTo(expectedResult))
			}
		},
		Entry(nil, buildv1.BuildPhaseComplete, kmmbuild.Result{Status: kmmbuild.StatusCompleted}, false),
		Entry(nil, buildv1.BuildPhaseNew, kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, false),
		Entry(nil, buildv1.BuildPhasePending, kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, false),
		Entry(nil, buildv1.BuildPhaseRunning, kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, false),
		Entry(nil, buildv1.BuildPhaseFailed, kmmbuild.Result{}, true),
		Entry(nil, buildv1.BuildPhaseCancelled, kmmbuild.Result{}, true),
	)
})

var _ = Describe("OpenShiftBuildsHelper_GetBuild", func() {
	const targetKernel = "target-kernels"

	var mockKubeClient *client.MockClient

	ctx := context.Background()

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
	})

	It("should return an error if an error occurred", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Return(errors.New("random error"))

		osbh := NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetBuild(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if there are two Builids with the same labels", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = make([]buildv1.Build, 2)
			})

		osbh := NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetBuild(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
		bc := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bcs.Items = []buildv1.Build{*bc}
			})

		osbh := NewOpenShiftBuildsHelper(mockKubeClient)

		res, err := osbh.GetBuild(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(bc))
	})
})
