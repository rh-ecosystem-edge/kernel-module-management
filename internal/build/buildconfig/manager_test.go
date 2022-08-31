package buildconfig_test

import (
	"context"
	"errors"
	"time"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/build/buildconfig"
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
		mockMaker                 *buildconfig.MockMaker
		mockOpenShiftBuildsHelper *buildconfig.MockOpenShiftBuildsHelper
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockKubeClient = client.NewMockClient(ctrl)
		mockMaker = buildconfig.NewMockMaker(ctrl)
		mockOpenShiftBuildsHelper = buildconfig.NewMockOpenShiftBuildsHelper(ctrl)
	})

	ctx := context.Background()

	It("should create a BuildConfig when none is present", func() {
		const (
			buildConfigName = "some-build-config"
			repoSecretName  = "repo-secret"
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

		m := buildconfig.NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper)

		buildConfig := buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: buildConfigName},
		}

		gomock.InOrder(
			mockOpenShiftBuildsHelper.EXPECT().GetBuildConfig(ctx, mod, targetKernel).Return(nil, buildconfig.ErrNoMatchingBuildConfig),
			mockMaker.EXPECT().MakeBuildConfig(mod, mapping, targetKernel, containerImage, true).Return(&buildConfig, nil),
			mockKubeClient.EXPECT().Create(ctx, &buildConfig),
		)

		res, err := m.Sync(ctx, mod, mapping, targetKernel, true)
		Expect(err).NotTo(HaveOccurred())
		Expect(res.Status).To(BeEquivalentTo(build.StatusCreated))
		Expect(res.Requeue).To(BeTrue())
	})

	DescribeTable(
		"should return the Build status when a BuildConfig is present",
		func(phase buildv1.BuildPhase, expectedResult build.Result, expectError bool) {
			const buildConfigName = "some-build-config"

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

			m := buildconfig.NewManager(mockKubeClient, mockMaker, mockOpenShiftBuildsHelper)

			buildConfig := buildv1.BuildConfig{
				ObjectMeta: metav1.ObjectMeta{Name: buildConfigName},
			}

			b := buildv1.Build{
				Status: buildv1.BuildStatus{Phase: phase},
			}

			gomock.InOrder(
				mockOpenShiftBuildsHelper.EXPECT().GetBuildConfig(ctx, mod, targetKernel).Return(&buildConfig, nil),
				mockOpenShiftBuildsHelper.EXPECT().GetLatestBuild(ctx, namespace, buildConfigName).Return(&b, nil),
			)

			res, err := m.Sync(ctx, mod, mapping, targetKernel, true)

			if expectError {
				Expect(err).To(HaveOccurred())
			} else {
				Expect(err).NotTo(HaveOccurred())
				Expect(res).To(BeEquivalentTo(expectedResult))
			}
		},
		Entry(nil, buildv1.BuildPhaseComplete, build.Result{Status: build.StatusCompleted}, false),
		Entry(nil, buildv1.BuildPhaseNew, build.Result{Status: build.StatusInProgress, Requeue: true}, false),
		Entry(nil, buildv1.BuildPhasePending, build.Result{Status: build.StatusInProgress, Requeue: true}, false),
		Entry(nil, buildv1.BuildPhaseRunning, build.Result{Status: build.StatusInProgress, Requeue: true}, false),
		Entry(nil, buildv1.BuildPhaseFailed, build.Result{}, true),
		Entry(nil, buildv1.BuildPhaseCancelled, build.Result{}, true),
	)
})

var _ = Describe("OpenShiftBuildsHelper_GetBuildConfig", func() {
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
			List(ctx, &buildv1.BuildConfigList{}, gomock.Any(), gomock.Any()).
			Return(errors.New("random error"))

		osbh := buildconfig.NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetBuildConfig(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error if there are two BuildConfigs with the same labels", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildConfigList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildConfigList, _ ...ctrlclient.ListOption) {
				bcs.Items = make([]buildv1.BuildConfig, 2)
			})

		osbh := buildconfig.NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetBuildConfig(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).To(HaveOccurred())
	})

	It("should work as expected", func() {
		bc := &buildv1.BuildConfig{
			ObjectMeta: metav1.ObjectMeta{Name: "test"},
		}

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildConfigList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bcs *buildv1.BuildConfigList, _ ...ctrlclient.ListOption) {
				bcs.Items = []buildv1.BuildConfig{*bc}
			})

		osbh := buildconfig.NewOpenShiftBuildsHelper(mockKubeClient)

		res, err := osbh.GetBuildConfig(ctx, kmmv1beta1.Module{}, targetKernel)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(bc))
	})
})

var _ = Describe("OpenShiftBuildsHelper_GetLatestBuild", func() {
	const (
		buildConfigName = "some-buildconfig"
		namespace       = "some-namespace"
	)

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

		osbh := buildconfig.NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetLatestBuild(ctx, namespace, buildConfigName)

		Expect(err).To(HaveOccurred())
	})

	It("should return an error no build was found", func() {
		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any())

		osbh := buildconfig.NewOpenShiftBuildsHelper(mockKubeClient)

		_, err := osbh.GetLatestBuild(ctx, namespace, buildConfigName)

		Expect(err).To(HaveOccurred())
	})

	It("should return the latest of two builds", func() {
		now := metav1.Now()

		b := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "newer",
				CreationTimestamp: now,
			},
		}

		mockKubeClient.
			EXPECT().
			List(ctx, &buildv1.BuildList{}, gomock.Any(), gomock.Any()).
			Do(func(_ context.Context, bl *buildv1.BuildList, _ ...ctrlclient.ListOption) {
				bl.Items = []buildv1.Build{
					b,
					{
						ObjectMeta: metav1.ObjectMeta{
							Name: "older",
							CreationTimestamp: metav1.NewTime(
								now.Add(-1 * time.Minute),
							),
						},
					},
				}
			})

		osbh := buildconfig.NewOpenShiftBuildsHelper(mockKubeClient)

		res, err := osbh.GetLatestBuild(ctx, namespace, buildConfigName)

		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(&b))
	})
})
