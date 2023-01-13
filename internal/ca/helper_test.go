package ca

import (
	"context"
	"errors"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/client"
)

var _ = Describe("helperImpl_Sync", func() {
	const namespace = "some-namespace"

	var (
		ctrl       *gomock.Controller
		mockClient *client.MockClient
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(ctrl)
	})

	ctx := context.Background()

	owner := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-secret-0",
			Namespace: namespace,
		},
	}

	clusterCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kmm-cluster-ca-",
			Namespace:    namespace,
			Labels: map[string]string{
				typeKey: clusterCAType,
				"config.openshift.io/inject-trusted-cabundle": "true",
			},
		},
	}

	servingCM := &v1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "kmm-service-ca-",
			Namespace:    namespace,
			Annotations:  map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
			Labels:       map[string]string{typeKey: serviceCAType},
		},
	}

	It("should work as expected", func() {
		gomock.InOrder(
			mockClient.EXPECT().List(ctx, &v1.ConfigMapList{}, ctrlclient.InNamespace(namespace)),
			mockClient.EXPECT().Create(ctx, clusterCM),
			mockClient.
				EXPECT().
				Patch(ctx, gomock.AssignableToTypeOf(clusterCM), gomock.Any()).
				Do(func(_ context.Context, cm *v1.ConfigMap, _ ctrlclient.Patch, _ ...ctrlclient.PatchOption) {
					Expect(cm.OwnerReferences).To(HaveLen(1))
					Expect(cm.OwnerReferences[0].Name).To(Equal(owner.GetName()))
					Expect(cm.OwnerReferences[0].Kind).To(Equal("Secret"))
				}),
			mockClient.EXPECT().List(ctx, &v1.ConfigMapList{}, ctrlclient.InNamespace(namespace)),
			mockClient.EXPECT().Create(ctx, servingCM),
			mockClient.
				EXPECT().
				Patch(ctx, gomock.AssignableToTypeOf(servingCM), gomock.Any()).
				Do(func(_ context.Context, cm *v1.ConfigMap, _ ctrlclient.Patch, _ ...ctrlclient.PatchOption) {
					Expect(cm.OwnerReferences).To(HaveLen(1))
					Expect(cm.OwnerReferences[0].Name).To(Equal(owner.GetName()))
					Expect(cm.OwnerReferences[0].Kind).To(Equal("Secret"))
				}),
		)

		helper := NewHelper(mockClient, scheme)

		err := helper.Sync(ctx, namespace, owner)
		Expect(err).NotTo(HaveOccurred())
	})
})

var _ = Describe("helperImpl_GetClusterCA", func() {
	const namespace = "some-namespace"

	var (
		ctrl       *gomock.Controller
		mockClient *client.MockClient
		helper     Helper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(ctrl)
		helper = NewHelper(mockClient, scheme)
	})

	ctx := context.Background()

	It("should return an error if the ConfigMap cannot be found", func() {
		mockClient.
			EXPECT().
			List(ctx, &v1.ConfigMapList{}, ctrlclient.InNamespace(namespace)).
			Return(errors.New("random error"))

		_, err := helper.GetClusterCA(ctx, namespace)
		Expect(err).To(HaveOccurred())
	})

	It("should return the ConfigMap if it exists", func() {
		const cmName = "cluster-ca"

		mockClient.
			EXPECT().
			List(ctx, &v1.ConfigMapList{}, ctrlclient.InNamespace(namespace)).
			Do(func(_ context.Context, cml *v1.ConfigMapList, _ ...ctrlclient.ListOption) {
				cml.Items = []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{Name: cmName},
					},
				}
			})

		cm, err := helper.GetClusterCA(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(cmName))
		Expect(cm.KeyName).To(Equal("ca-bundle.crt"))
	})
})

var _ = Describe("helperImpl_GetServiceCA", func() {
	const namespace = "some-namespace"

	var (
		ctrl       *gomock.Controller
		mockClient *client.MockClient
		helper     Helper
	)

	BeforeEach(func() {
		ctrl = gomock.NewController(GinkgoT())
		mockClient = client.NewMockClient(ctrl)
		helper = NewHelper(mockClient, scheme)
	})

	ctx := context.Background()

	It("should return an error if the ConfigMap cannot be found", func() {
		mockClient.
			EXPECT().
			List(ctx, &v1.ConfigMapList{}, ctrlclient.InNamespace(namespace)).
			Return(errors.New("random error"))

		_, err := helper.GetServiceCA(ctx, namespace)
		Expect(err).To(HaveOccurred())
	})

	It("should return the ConfigMap if it exists", func() {
		const cmName = "service-ca"

		mockClient.
			EXPECT().
			List(ctx, &v1.ConfigMapList{}, ctrlclient.InNamespace(namespace)).
			Do(func(_ context.Context, cml *v1.ConfigMapList, _ ...ctrlclient.ListOption) {
				cml.Items = []v1.ConfigMap{
					{
						ObjectMeta: metav1.ObjectMeta{Name: cmName},
					},
				}
			})

		cm, err := helper.GetServiceCA(ctx, namespace)
		Expect(err).NotTo(HaveOccurred())
		Expect(cm.Name).To(Equal(cmName))
		Expect(cm.KeyName).To(Equal("service-ca.crt"))
	})
})
