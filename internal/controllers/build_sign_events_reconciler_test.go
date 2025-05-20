package controllers

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/api-hub/v1beta1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	kmmv1beta2 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	testclient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/meta"
	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

func getOwnerReferenceFromObject(obj ctrlclient.Object) metav1.OwnerReference {
	GinkgoHelper()

	uo := &unstructured.Unstructured{}
	uo.SetNamespace(obj.GetNamespace())

	Expect(
		controllerutil.SetOwnerReference(obj, uo, scheme),
	).NotTo(
		HaveOccurred(),
	)

	return uo.GetOwnerReferences()[0]
}

func mustJobEvent(jobType string) *jobEvent {
	GinkgoHelper()

	je, err := newJobEvent(jobType)
	Expect(err).NotTo(HaveOccurred())

	return je
}

var _ = Describe("jobEvent_ReasonCreated", func() {
	It("should work as expected", func() {
		Expect(
			mustJobEvent("build").ReasonCreated(),
		).To(
			Equal("BuildCreated"),
		)
	})
})

var _ = Describe("jobEvent_ReasonFailed", func() {
	It("should work as expected", func() {
		Expect(
			mustJobEvent("build").ReasonFailed(),
		).To(
			Equal("BuildFailed"),
		)
	})
})

var _ = Describe("jobEvent_ReasonSucceeded", func() {
	It("should work as expected", func() {
		Expect(
			mustJobEvent("build").ReasonSucceeded(),
		).To(
			Equal("BuildSucceeded"),
		)
	})
})

var _ = Describe("jobEvent_String", func() {
	DescribeTable(
		"should capitalize correctly",
		func(jobType string, expectedError bool, expectedString string) {
			je, err := newJobEvent(jobType)

			if expectedError {
				Expect(err).To(HaveOccurred())
				return
			}

			Expect(je.String()).To(Equal(expectedString))
		},
		Entry(nil, "", true, ""),
		Entry(nil, "sign", false, "Sign"),
		Entry(nil, "build", false, "Build"),
		Entry(nil, "anything", false, "Anything"),
		Entry(nil, "with-hyphen", false, "With-Hyphen"),
	)
})

var (
	ownerModule = &kmmv1beta1.Module{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "module-name",
			Namespace: namespace,
		},
	}
	ownerPreflight = &kmmv1beta2.PreflightValidation{
		ObjectMeta: metav1.ObjectMeta{Name: "preflight-name"},
	}
	ownerMCM = &v1beta1.ManagedClusterModule{
		ObjectMeta: metav1.ObjectMeta{Name: "mcm"},
	}
)

var _ = Describe("JobEventReconciler_Reconcile", func() {
	const (
		kernelVersion = "1.2.3"
		moduleName    = "module-name"
		namespace     = "namespace"
	)

	var (
		ctx = context.TODO()

		fakeRecorder *record.FakeRecorder
		mockClient   *testclient.MockClient
		mockHelper   *MockJobEventReconcilerHelper
		r            *JobEventReconciler
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		fakeRecorder = record.NewFakeRecorder(2)
		mockClient = testclient.NewMockClient(ctrl)
		mockHelper = NewMockJobEventReconcilerHelper(ctrl)
		r = NewBuildSignEventsReconciler(mockClient, mockHelper, fakeRecorder)
	})

	closeAndGetAllEvents := func(events chan string) []string {
		GinkgoHelper()

		close(events)

		elems := make([]string, 0)

		for s := range events {
			elems = append(elems, s)
		}

		return elems
	}

	It("should add the annotation and send the created event", func() {
		or := getOwnerReferenceFromObject(ownerModule)

		build := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					constants.BuildTypeLabel:     string(kmmv1beta1.BuildImage),
					constants.TargetKernelTarget: kernelVersion,
				},
				OwnerReferences: []metav1.OwnerReference{or},
				Namespace:       namespace,
			},
		}

		podWithAnnotation := *build
		meta.SetAnnotation(&podWithAnnotation, createdAnnotationKey, "")

		gomock.InOrder(
			mockHelper.EXPECT().GetOwner(ctx, or, namespace),
			mockClient.EXPECT().Patch(ctx, &podWithAnnotation, gomock.Any()),
		)

		Expect(
			r.Reconcile(ctx, build),
		).To(
			Equal(ctrl.Result{}),
		)

		events := closeAndGetAllEvents(fakeRecorder.Events)
		Expect(events).To(HaveLen(1))
		Expect(events[0]).To(ContainSubstring("Normal BuildimageCreated Buildimage created for kernel " + kernelVersion))
	})

	It("should do nothing if the annotation is already there and the Build is still running", func() {
		or := getOwnerReferenceFromObject(ownerModule)

		pod := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Annotations:     map[string]string{createdAnnotationKey: ""},
				Labels:          map[string]string{constants.BuildTypeLabel: string(kmmv1beta1.SignImage)},
				Namespace:       namespace,
				OwnerReferences: []metav1.OwnerReference{or},
			},
		}

		mockHelper.EXPECT().GetOwner(ctx, or, namespace)

		Expect(
			r.Reconcile(ctx, pod),
		).To(
			Equal(ctrl.Result{}),
		)

		events := closeAndGetAllEvents(fakeRecorder.Events)
		Expect(events).To(BeEmpty())
	})

	It("should just remove the finalizer if the module could not be found", func() {
		or := getOwnerReferenceFromObject(ownerModule)

		build := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Labels:          map[string]string{constants.BuildTypeLabel: string(kmmv1beta1.BuildImage)},
				Finalizers:      []string{constants.JobEventFinalizer},
				Namespace:       namespace,
				OwnerReferences: []metav1.OwnerReference{or},
			},
		}

		podWithoutFinalizer := *build
		controllerutil.RemoveFinalizer(&podWithoutFinalizer, constants.JobEventFinalizer)

		gomock.InOrder(
			mockHelper.
				EXPECT().
				GetOwner(ctx, or, namespace).
				Return(
					nil,
					k8serrors.NewNotFound(schema.GroupResource{}, moduleName),
				),
			mockClient.EXPECT().Patch(ctx, &podWithoutFinalizer, gomock.Any()),
		)

		Expect(
			r.Reconcile(ctx, build),
		).To(
			Equal(ctrl.Result{}),
		)

		events := closeAndGetAllEvents(fakeRecorder.Events)
		Expect(events).To(BeEmpty())
	})

	DescribeTable(
		"should send the event for terminated builds",
		func(jobType string, phase buildv1.BuildPhase, sendEventAndRemoveFinalizer bool, substring string, owner ctrlclient.Object) {
			or := getOwnerReferenceFromObject(owner)

			build := &buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{createdAnnotationKey: ""},
					Labels: map[string]string{
						constants.BuildTypeLabel:     jobType,
						constants.TargetKernelTarget: kernelVersion,
					},
					Finalizers:      []string{constants.JobEventFinalizer},
					Namespace:       namespace,
					OwnerReferences: []metav1.OwnerReference{or},
				},
				Status: buildv1.BuildStatus{Phase: phase},
			}

			podWithoutFinalizer := *build
			controllerutil.RemoveFinalizer(&podWithoutFinalizer, constants.JobEventFinalizer)

			getOwner := mockHelper.EXPECT().GetOwner(ctx, or, namespace)
			if sendEventAndRemoveFinalizer {
				mockClient.EXPECT().Patch(ctx, &podWithoutFinalizer, gomock.Any()).After(getOwner)
			}

			Expect(
				r.Reconcile(ctx, build),
			).To(
				Equal(ctrl.Result{}),
			)

			events := closeAndGetAllEvents(fakeRecorder.Events)

			if !sendEventAndRemoveFinalizer {
				Expect(events).To(HaveLen(0))
				return
			}

			Expect(events).To(HaveLen(1))
			Expect(events[0]).To(ContainSubstring(substring))
		},
		Entry(nil, "test", buildv1.BuildPhaseRunning, false, "", ownerModule),
		Entry(nil, "test", buildv1.BuildPhasePending, false, "", ownerModule),
		Entry(nil, "build", buildv1.BuildPhaseFailed, true, "Warning BuildFailed Build job failed for kernel "+kernelVersion, ownerModule),
		Entry(nil, "sign", buildv1.BuildPhaseComplete, true, "Normal SignSucceeded Sign job succeeded for kernel "+kernelVersion, ownerModule),
		Entry(nil, "build", buildv1.BuildPhaseFailed, true, "Warning BuildFailed Build job failed for kernel "+kernelVersion, ownerPreflight),
		Entry(nil, "sign", buildv1.BuildPhaseComplete, true, "Normal SignSucceeded Sign job succeeded for kernel "+kernelVersion, ownerPreflight),
		Entry(nil, "build", buildv1.BuildPhaseFailed, true, "Warning BuildFailed Build job failed for kernel "+kernelVersion, ownerMCM),
		Entry(nil, "sign", buildv1.BuildPhaseComplete, true, "Normal SignSucceeded Sign job succeeded for kernel "+kernelVersion, ownerMCM),
		Entry(nil, "random", buildv1.BuildPhaseFailed, true, "Warning RandomFailed Random job failed for kernel "+kernelVersion, ownerModule),
		Entry(nil, "random", buildv1.BuildPhaseComplete, true, "Normal RandomSucceeded Random job succeeded for kernel "+kernelVersion, ownerModule),
		Entry(nil, "random", buildv1.BuildPhaseCancelled, true, "Normal RandomCancelled Random job cancelled for kernel "+kernelVersion, ownerModule),
	)
})

var _ = Describe("jobEventReconcilerHelper_GetOwner", func() {
	var (
		ctx = context.TODO()

		mockClient *testclient.MockClient
		r          *jobEventReconcilerHelper
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mockClient = testclient.NewMockClient(ctrl)
		r = &jobEventReconcilerHelper{client: mockClient}
	})

	DescribeTable(
		"should look for the right object",
		func(owner ctrlclient.Object, namespaced bool) {
			or := getOwnerReferenceFromObject(owner)

			nsn := types.NamespacedName{
				Name: owner.GetName(),
			}

			gvk, err := apiutil.GVKForObject(owner, scheme)
			Expect(err).NotTo(HaveOccurred())

			uo := &unstructured.Unstructured{}
			uo.SetGroupVersionKind(gvk)

			if namespaced {
				nsn.Namespace = namespace
			}

			gomock.InOrder(
				mockClient.EXPECT().IsObjectNamespaced(uo).Return(namespaced, nil),
				mockClient.EXPECT().Get(ctx, nsn, uo),
			)

			Expect(
				r.GetOwner(ctx, or, namespace),
			).To(
				Equal(uo),
			)
		},
		Entry(nil, ownerModule, true),
		Entry(nil, ownerPreflight, false),
		Entry(nil, ownerMCM, false),
	)

	It("should return an error wrapping the original one", func() {
		or := getOwnerReferenceFromObject(ownerModule)

		ownerName := ownerModule.GetName()

		nsn := types.NamespacedName{
			Name:      ownerName,
			Namespace: namespace,
		}

		gvk, err := apiutil.GVKForObject(ownerModule, scheme)
		Expect(err).NotTo(HaveOccurred())

		uo := &unstructured.Unstructured{}
		uo.SetGroupVersionKind(gvk)

		gomock.InOrder(
			mockClient.EXPECT().IsObjectNamespaced(uo).Return(true, nil),
			mockClient.EXPECT().Get(ctx, nsn, uo).Return(
				k8serrors.NewNotFound(schema.GroupResource{Group: gvk.Group, Resource: gvk.Kind}, ownerName),
			),
		)

		_, err = r.GetOwner(ctx, or, namespace)
		Expect(
			k8serrors.IsNotFound(err),
		).To(
			BeTrue(),
		)
	})
})

var _ = Describe("jobEventPredicate", func() {
	DescribeTable(
		"should work as expected",
		func(buildType string, hasFinalizer, expectedResult bool) {
			build := &buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{constants.BuildTypeLabel: buildType},
				},
			}

			if hasFinalizer {
				controllerutil.AddFinalizer(build, constants.JobEventFinalizer)
			}

			Expect(
				jobEventPredicate.Create(event.CreateEvent{Object: build}),
			).To(
				Equal(expectedResult),
			)

			Expect(
				jobEventPredicate.Delete(event.DeleteEvent{Object: build}),
			).To(
				Equal(expectedResult),
			)

			Expect(
				jobEventPredicate.Generic(event.GenericEvent{Object: build}),
			).To(
				Equal(expectedResult),
			)

			Expect(
				jobEventPredicate.Update(event.UpdateEvent{ObjectNew: build}),
			).To(
				Equal(expectedResult),
			)
		},
		Entry(nil, string(kmmv1beta1.BuildImage), true, true),
		Entry(nil, string(kmmv1beta1.SignImage), true, true),
		Entry(nil, "random", true, false),
		Entry(nil, "", true, false),
		Entry("finalizer", string(kmmv1beta1.BuildImage), true, true),
		Entry("no finalizer", string(kmmv1beta1.BuildImage), false, false),
	)
})
