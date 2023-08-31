package ca

import (
	"context"
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	clusterCAType = "cluster"
	serviceCAType = "service"
	typeKey       = "kmm.openshift.io/ca.type"
)

type ConfigMap struct {
	KeyName string
	Name    string
}

var errConfigMapNotFound = errors.New("ConfigMap not found")

//go:generate mockgen -source=helper.go -package=ca -destination=mock_helper.go

type Helper interface {
	GetClusterCA(ctx context.Context, namespace string) (*ConfigMap, error)
	GetServiceCA(ctx context.Context, namespace string) (*ConfigMap, error)
	Sync(ctx context.Context, namespace string, owner client.Object) error
}

type helperImpl struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewHelper(client client.Client, scheme *runtime.Scheme) Helper {
	return &helperImpl{
		client: client,
		scheme: scheme,
	}
}

func (h *helperImpl) Sync(ctx context.Context, namespace string, owner client.Object) error {
	logger := ctrl.LoggerFrom(ctx)

	logger.Info("Syncing the Cluster CA ConfigMap")

	clusterCM, err := h.getOrCreateClusterCAConfigMap(ctx, namespace)
	if err != nil {
		return fmt.Errorf("could not get the cluster CA ConfigMap: %v", err)
	}

	if err = h.syncConfigMapOwners(ctx, clusterCM, owner, logger); err != nil {
		return fmt.Errorf("could not sync the cluster CA ConfigMap owners: %v", err)
	}

	logger.Info("Syncing the service CA ConfigMap")

	serviceCM, err := h.getOrCreateServiceCAConfigMap(ctx, namespace)
	if err != nil {
		return fmt.Errorf("could not get the service CA ConfigMap: %v", err)
	}

	if err = h.syncConfigMapOwners(ctx, serviceCM, owner, logger); err != nil {
		return fmt.Errorf("could not sync the service CA ConfigMap owners: %v", err)
	}

	logger.Info("Synced CA ConfigMaps")

	return nil
}

func (h *helperImpl) GetClusterCA(ctx context.Context, namespace string) (*ConfigMap, error) {
	cm, err := h.getConfigMap(ctx, namespace, clusterCAType)
	if err != nil {
		return nil, fmt.Errorf("could not get CA ConfigMap: %v", err)
	}

	caCM := ConfigMap{
		KeyName: "ca-bundle.crt",
		Name:    cm.Name,
	}

	return &caCM, nil
}

func (h *helperImpl) GetServiceCA(ctx context.Context, namespace string) (*ConfigMap, error) {
	cm, err := h.getConfigMap(ctx, namespace, serviceCAType)
	if err != nil {
		return nil, fmt.Errorf("could not get CA ConfigMap: %v", err)
	}

	caCM := ConfigMap{
		KeyName: "service-ca.crt",
		Name:    cm.Name,
	}

	return &caCM, nil
}

func (h *helperImpl) getOrCreateClusterCAConfigMap(ctx context.Context, namespace string) (*v1.ConfigMap, error) {
	cm, err := h.getConfigMap(ctx, namespace, clusterCAType)
	if err != nil {
		if !errors.Is(err, errConfigMapNotFound) {
			return nil, fmt.Errorf("could not get the cluster CA ConfigMap: %v", err)
		}

		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kmm-cluster-ca-",
				Namespace:    namespace,
				Labels: map[string]string{
					typeKey: clusterCAType,
					"config.openshift.io/inject-trusted-cabundle": "true",
				},
			},
		}

		if err = h.client.Create(ctx, cm); err != nil {
			return nil, fmt.Errorf("could not create the cluster CA ConfigMap: %v", err)
		}
	}

	return cm, nil
}

func (h *helperImpl) getOrCreateServiceCAConfigMap(ctx context.Context, namespace string) (*v1.ConfigMap, error) {
	cm, err := h.getConfigMap(ctx, namespace, serviceCAType)
	if err != nil {
		if !errors.Is(err, errConfigMapNotFound) {
			return nil, fmt.Errorf("could not get the cluster CA ConfigMap: %v", err)
		}

		cm = &v1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				GenerateName: "kmm-service-ca-",
				Namespace:    namespace,
				Annotations:  map[string]string{"service.beta.openshift.io/inject-cabundle": "true"},
				Labels:       map[string]string{typeKey: serviceCAType},
			},
		}

		if err = h.client.Create(ctx, cm); err != nil {
			return nil, fmt.Errorf("could not create the cluster CA ConfigMap: %v", err)
		}
	}

	return cm, nil
}

func (h *helperImpl) getConfigMap(ctx context.Context, namespace, caType string) (*v1.ConfigMap, error) {
	cmList := v1.ConfigMapList{}

	opts := []client.ListOption{
		client.InNamespace(namespace),
		client.MatchingLabels{typeKey: caType},
	}

	if err := h.client.List(ctx, &cmList, opts...); err != nil {
		return nil, fmt.Errorf("could not list ConfigMaps: %v", err)
	}

	if len(cmList.Items) == 0 {
		return nil, errConfigMapNotFound
	}

	if l := len(cmList.Items); l > 1 {
		return nil, fmt.Errorf("expected 1 ConfigMap, got %d", l)
	}

	return &cmList.Items[0], nil
}

func (h *helperImpl) syncConfigMapOwners(ctx context.Context, cm *v1.ConfigMap, owner client.Object, logger logr.Logger) error {
	logger = logger.WithValues("cm-name", cm.Name)

	logger.Info("Reconciling CA ConfigMap owners")

	p := client.MergeFrom(cm.DeepCopy())

	if err := controllerutil.SetOwnerReference(owner, cm, h.scheme); err != nil {
		return fmt.Errorf("could not set %s/%s as the owner of the ConfigMap: %v", owner.GetNamespace(), owner.GetName(), err)
	}

	if err := h.client.Patch(ctx, cm, p); err != nil {
		return fmt.Errorf("error patching ConfigMap %s/%s: %v", cm.Namespace, cm.Name, err)
	}

	logger.Info("Reconciled CA ConfigMap owners")

	return nil
}
