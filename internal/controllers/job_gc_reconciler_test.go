package controllers

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	buildv1 "github.com/openshift/api/build/v1"
	testclient "github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
	"go.uber.org/mock/gomock"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("JobGCReconciler_Reconcile", func() {
	ctx := context.Background()

	type testCase struct {
		deletionTimestamp     time.Time
		gcDelay               time.Duration
		buildPhase            buildv1.BuildPhase
		shouldRemoveFinalizer bool
		shouldSetRequeueAfter bool
	}

	DescribeTable(
		"should work as expected",
		func(tc testCase) {
			ctrl := gomock.NewController(GinkgoT())
			mockClient := testclient.NewMockClient(ctrl)

			build := buildv1.Build{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: tc.deletionTimestamp},
				},
				Status: buildv1.BuildStatus{Phase: tc.buildPhase},
			}

			if tc.shouldRemoveFinalizer {
				mockClient.EXPECT().Patch(ctx, &build, gomock.Any())
			}

			res, err := NewJobGCReconciler(mockClient, time.Minute).Reconcile(ctx, &build)

			Expect(err).NotTo(HaveOccurred())

			if tc.shouldSetRequeueAfter {
				Expect(res.RequeueAfter).NotTo(BeZero())
			} else {
				Expect(res.RequeueAfter).To(BeZero())
			}
		},
		Entry(
			"build succeeded, before now+delay",
			testCase{
				deletionTimestamp:     time.Now(),
				gcDelay:               time.Hour,
				buildPhase:            buildv1.BuildPhaseComplete,
				shouldSetRequeueAfter: true,
			},
		),
		Entry(
			"build succeeded, after now+delay",
			testCase{
				deletionTimestamp:     time.Now().Add(-time.Hour),
				gcDelay:               time.Minute,
				buildPhase:            buildv1.BuildPhaseComplete,
				shouldRemoveFinalizer: true,
			},
		),
		Entry(
			"build failed, before now+delay",
			testCase{
				deletionTimestamp:     time.Now(),
				gcDelay:               time.Hour,
				buildPhase:            buildv1.BuildPhaseFailed,
				shouldRemoveFinalizer: true,
			},
		),
		Entry(
			"build failed, after now+delay",
			testCase{
				deletionTimestamp:     time.Now().Add(-time.Hour),
				gcDelay:               time.Minute,
				buildPhase:            buildv1.BuildPhaseFailed,
				shouldRemoveFinalizer: true,
			},
		),
	)

	It("should return an error if the patch failed", func() {
		ctrl := gomock.NewController(GinkgoT())
		mockClient := testclient.NewMockClient(ctrl)

		build := buildv1.Build{
			ObjectMeta: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{
					Time: time.Now().Add(-2 * time.Minute),
				},
			},
		}

		mockClient.EXPECT().Patch(ctx, &build, gomock.Any()).Return(errors.New("random error"))

		_, err := NewJobGCReconciler(mockClient, time.Minute).Reconcile(ctx, &build)

		Expect(err).To(HaveOccurred())
	})
})
