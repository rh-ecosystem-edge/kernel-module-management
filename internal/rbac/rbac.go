package rbac

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

//go:generate mockgen -source=rbac.go -package=rbac -destination=mock_rbac.go

type RBACCreator interface {
	CreateModuleLoaderRBAC(ctx context.Context, mod kmmv1beta1.Module) error
	CreateDevicePluginRBAC(ctx context.Context, mod kmmv1beta1.Module) error
}

type rbacCreator struct {
	client client.Client
	scheme *runtime.Scheme
}

func NewCreator(client client.Client, scheme *runtime.Scheme) RBACCreator {
	return &rbacCreator{
		client: client,
		scheme: scheme,
	}
}

func GenerateModuleLoaderServiceAccountName(mod kmmv1beta1.Module) string {
	return generateModuleLoaderRBACName(mod)
}

func GenerateDevicePluginServiceAccountName(mod kmmv1beta1.Module) string {
	return generateDevicePluginRBACName(mod)
}

func (rc *rbacCreator) CreateModuleLoaderRBAC(ctx context.Context, mod kmmv1beta1.Module) error {
	logger := log.FromContext(ctx)

	serviceAccountName := GenerateModuleLoaderServiceAccountName(mod)

	opRes, err := rc.createServiceAccount(ctx, mod, serviceAccountName)
	if err != nil {
		return fmt.Errorf("cound not create module-loader's ServiceAccount: %w", err)
	}
	logger.Info("Created module-loader's ServiceAccount", "name", serviceAccountName, "result", opRes)

	roleBindingName := generateModuleLoaderRoleBindingName(mod)
	clusterRoleName := "kmm-operator-module-loader"

	opRes, err = rc.createRoleBinding(ctx, mod, roleBindingName, serviceAccountName, clusterRoleName)
	if err != nil {
		return fmt.Errorf("cound not create module-loader's RoleBinding: %w", err)
	}
	logger.Info("Created module-loader's RoleBinding", "name", roleBindingName, "result", opRes)

	return nil
}

func (rc *rbacCreator) CreateDevicePluginRBAC(ctx context.Context, mod kmmv1beta1.Module) error {
	logger := log.FromContext(ctx)

	serviceAccountName := GenerateDevicePluginServiceAccountName(mod)

	opRes, err := rc.createServiceAccount(ctx, mod, serviceAccountName)
	if err != nil {
		return fmt.Errorf("cound not create device-plugin's ServiceAccount: %w", err)
	}
	logger.Info("Created device-plugin's ServiceAccount", "name", serviceAccountName, "result", opRes)

	roleBindingName := generateDevicePluginRoleBindingName(mod)
	clusterRoleName := "kmm-operator-device-plugin"

	opRes, err = rc.createRoleBinding(ctx, mod, roleBindingName, serviceAccountName, clusterRoleName)
	if err != nil {
		return fmt.Errorf("cound not create device-plugin's RoleBinding: %w", err)
	}
	logger.Info("Created device-plugin's RoleBinding", "name", roleBindingName, "result", opRes)

	return nil
}

func (rc *rbacCreator) createServiceAccount(
	ctx context.Context,
	mod kmmv1beta1.Module,
	name string) (controllerutil.OperationResult, error) {

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: mod.Namespace,
		},
	}

	return controllerutil.CreateOrPatch(ctx, rc.client, sa, func() error {
		return controllerutil.SetControllerReference(&mod, sa, rc.scheme)
	})
}

func (rc *rbacCreator) createRoleBinding(
	ctx context.Context,
	mod kmmv1beta1.Module,
	name,
	serviceAccountName,
	clusterRoleName string) (controllerutil.OperationResult, error) {

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: mod.Namespace,
		},
	}

	return controllerutil.CreateOrPatch(ctx, rc.client, rb, func() error {
		rb.Subjects = []rbacv1.Subject{
			{
				Kind:      rbacv1.ServiceAccountKind,
				Name:      serviceAccountName,
				Namespace: mod.Namespace,
			},
		}

		rb.RoleRef = rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
		}

		return controllerutil.SetControllerReference(&mod, rb, rc.scheme)
	})
}

func generateModuleLoaderRoleBindingName(mod kmmv1beta1.Module) string {
	return generateModuleLoaderRBACName(mod)
}

func generateModuleLoaderRBACName(mod kmmv1beta1.Module) string {
	return mod.Name + "-module-loader"
}

func generateDevicePluginRoleBindingName(mod kmmv1beta1.Module) string {
	return generateDevicePluginRBACName(mod)
}

func generateDevicePluginRBACName(mod kmmv1beta1.Module) string {
	return mod.Name + "-device-plugin"
}
