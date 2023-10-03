package controllers

import (
	"context"
	"errors"

	"github.com/budougumi0617/cmpmock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	ocpbuildbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build/ocpbuild"
	testclient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/meta"
	ocpbuildsign "github.com/rh-ecosystem-edge/kernel-module-management/internal/sign/ocpbuild"
	"go.uber.org/mock/gomock"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

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
		modNSN       = types.NamespacedName{Namespace: namespace, Name: moduleName}
		buildNSN     = types.NamespacedName{Namespace: namespace, Name: "name"}
		r            *JobEventReconciler
	)

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		fakeRecorder = record.NewFakeRecorder(2)
		mockClient = testclient.NewMockClient(ctrl)
		r = NewBuildSignEventsReconciler(mockClient, fakeRecorder)
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

	It("should return no error if the Build could not be found", func() {
		mockClient.
			EXPECT().
			Get(ctx, buildNSN, &buildv1.Build{}).
			Return(
				k8serrors.NewNotFound(schema.GroupResource{Resource: "builds"}, buildNSN.Name),
			)

		Expect(
			r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN}),
		).To(
			Equal(reconcile.Result{}),
		)
	})

	It("should return no error if we cannot get the Build for a random reason", func() {
		mockClient.
			EXPECT().
			Get(ctx, buildNSN, &buildv1.Build{}).
			Return(
				errors.New("random error"),
			)

		_, err := r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN})
		Expect(err).To(HaveOccurred())
	})

	It("should add the annotation and send the created event", func() {
		build := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{createdAnnotationKey: ""},
				Labels: map[string]string{
					constants.BuildTypeLabel:     ocpbuildbuild.BuildType,
					constants.ModuleNameLabel:    moduleName,
					constants.TargetKernelTarget: kernelVersion,
				},
			},
		}

		gomock.InOrder(
			mockClient.
				EXPECT().
				Get(ctx, buildNSN, &buildv1.Build{}).
				Do(func(_ context.Context, _ types.NamespacedName, build *buildv1.Build, _ ...ctrlclient.GetOption) {
					meta.SetLabel(build, constants.BuildTypeLabel, ocpbuildbuild.BuildType)
					meta.SetLabel(build, constants.ModuleNameLabel, moduleName)
					meta.SetLabel(build, constants.TargetKernelTarget, kernelVersion)
				}),
			mockClient.
				EXPECT().
				Get(ctx, modNSN, &kmmv1beta1.Module{}),
			mockClient.EXPECT().Patch(ctx, cmpmock.DiffEq(build), gomock.Any()),
		)

		Expect(
			r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN}),
		).To(
			Equal(ctrl.Result{}),
		)

		events := closeAndGetAllEvents(fakeRecorder.Events)
		Expect(events).To(HaveLen(1))
		Expect(events[0]).To(ContainSubstring("Normal BuildCreated Build created for kernel " + kernelVersion))
	})

	It("should do nothing if the annotation is already there and the Build is still running", func() {
		gomock.InOrder(
			mockClient.
				EXPECT().
				Get(ctx, buildNSN, &buildv1.Build{}).
				Do(func(_ context.Context, _ types.NamespacedName, b *buildv1.Build, _ ...ctrlclient.GetOption) {
					meta.SetLabel(b, constants.BuildTypeLabel, ocpbuildbuild.BuildType)
					meta.SetLabel(b, constants.ModuleNameLabel, moduleName)
					meta.SetAnnotation(b, createdAnnotationKey, "")
				}),
			mockClient.
				EXPECT().
				Get(ctx, modNSN, &kmmv1beta1.Module{}),
		)

		Expect(
			r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN}),
		).To(
			Equal(ctrl.Result{}),
		)

		events := closeAndGetAllEvents(fakeRecorder.Events)
		Expect(events).To(BeEmpty())
	})

	It("should just remove the finalizer if the module could not be found", func() {
		build := &buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					constants.BuildTypeLabel:  ocpbuildbuild.BuildType,
					constants.ModuleNameLabel: moduleName,
				},
				Finalizers: make([]string, 0),
			},
		}

		gomock.InOrder(
			mockClient.
				EXPECT().
				Get(ctx, buildNSN, &buildv1.Build{}).
				Do(func(_ context.Context, _ types.NamespacedName, b *buildv1.Build, _ ...ctrlclient.GetOption) {
					meta.SetLabel(b, constants.BuildTypeLabel, ocpbuildbuild.BuildType)
					meta.SetLabel(b, constants.ModuleNameLabel, moduleName)
					controllerutil.AddFinalizer(b, constants.JobEventFinalizer)
				}),
			mockClient.
				EXPECT().
				Get(ctx, modNSN, &kmmv1beta1.Module{}).
				Return(
					k8serrors.NewNotFound(schema.GroupResource{}, moduleName),
				),
			mockClient.EXPECT().Patch(ctx, cmpmock.DiffEq(build), gomock.Any()),
		)

		Expect(
			r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN}),
		).To(
			Equal(ctrl.Result{}),
		)

		events := closeAndGetAllEvents(fakeRecorder.Events)
		Expect(events).To(BeEmpty())
	})

	DescribeTable(
		"should send the event for terminated builds",
		func(jobType string, phase buildv1.BuildPhase, sendEventAndRemoveFinalizer bool, substring string) {
			build := &buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{createdAnnotationKey: ""},
					Labels: map[string]string{
						constants.BuildTypeLabel:     jobType,
						constants.ModuleNameLabel:    moduleName,
						constants.TargetKernelTarget: kernelVersion,
					},
					Finalizers: make([]string, 0),
				},
				Status: buildv1.BuildStatus{Phase: phase},
			}

			calls := []any{
				mockClient.
					EXPECT().
					Get(ctx, buildNSN, &buildv1.Build{}).
					Do(func(_ context.Context, _ types.NamespacedName, b *buildv1.Build, _ ...ctrlclient.GetOption) {
						meta.SetAnnotation(b, createdAnnotationKey, "")
						meta.SetLabel(b, constants.BuildTypeLabel, jobType)
						meta.SetLabel(b, constants.ModuleNameLabel, moduleName)
						meta.SetLabel(b, constants.TargetKernelTarget, kernelVersion)
						controllerutil.AddFinalizer(b, constants.JobEventFinalizer)
						b.Status.Phase = phase
					}),
				mockClient.
					EXPECT().
					Get(ctx, modNSN, &kmmv1beta1.Module{}),
			}

			if sendEventAndRemoveFinalizer {
				calls = append(
					calls,
					mockClient.EXPECT().Patch(ctx, cmpmock.DiffEq(build), gomock.Any()),
				)
			}

			gomock.InOrder(calls...)

			Expect(
				r.Reconcile(ctx, reconcile.Request{NamespacedName: buildNSN}),
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
		Entry(nil, "test", buildv1.BuildPhaseRunning, false, ""),
		Entry(nil, "test", buildv1.BuildPhasePending, false, ""),
		Entry(nil, "build", buildv1.BuildPhaseFailed, true, "Warning BuildFailed Build job failed for kernel "+kernelVersion),
		Entry(nil, "sign", buildv1.BuildPhaseComplete, true, "Normal SignSucceeded Sign job succeeded for kernel "+kernelVersion),
		Entry(nil, "random", buildv1.BuildPhaseFailed, true, "Warning RandomFailed Random job failed for kernel "+kernelVersion),
		Entry(nil, "random", buildv1.BuildPhaseComplete, true, "Normal RandomSucceeded Random job succeeded for kernel "+kernelVersion),
		Entry(nil, "random", buildv1.BuildPhaseCancelled, true, "Normal RandomCancelled Random job cancelled for kernel "+kernelVersion),
	)
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
		Entry(nil, ocpbuildbuild.BuildType, true, true),
		Entry(nil, ocpbuildsign.BuildType, true, true),
		Entry(nil, "random", true, false),
		Entry(nil, "", true, false),
		Entry("finalizer", ocpbuildbuild.BuildType, true, true),
		Entry("no finalizer", ocpbuildbuild.BuildType, false, false),
	)
})
