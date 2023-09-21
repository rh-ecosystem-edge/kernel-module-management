package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

//+kubebuilder:rbac:groups="core",resources=nodes,verbs=get;list;watch

const (
	KernelDTKReconcilerName = "KernelDTK"
)

// We expect an osImageVersion of the form 411.86.202210072320-0 for example
var osVersionRegexp = regexp.MustCompile(`\d+\.\d+\.\d+\-\d`)

type KernelDTKReconciler struct {
	client             client.Client
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
}

func NewKernelDTKReconciler(client client.Client, kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping) *KernelDTKReconciler {
	return &KernelDTKReconciler{
		client:             client,
		kernelOsDtkMapping: kernelOsDtkMapping,
	}
}

func (r *KernelDTKReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	node := v1.Node{}

	logger := log.FromContext(ctx)

	if err := r.client.Get(ctx, types.NamespacedName{Name: req.Name}, &node); err != nil {
		return ctrl.Result{}, fmt.Errorf("could not get node: %v", err)
	}

	kernelVersion := strings.TrimSuffix(node.Status.NodeInfo.KernelVersion, "+")
	osImageVersion := osVersionRegexp.FindString(node.Status.NodeInfo.OSImage)
	if osImageVersion == "" {
		return ctrl.Result{}, fmt.Errorf("could not get node %s osImageVersion", node.Name)
	}
	r.kernelOsDtkMapping.SetNodeInfo(kernelVersion, osImageVersion)
	logger.Info("registered node info mapping", "kernelVersion", kernelVersion, "osImageVersion", osImageVersion)

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *KernelDTKReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.
		NewControllerManagedBy(mgr).
		Named(KernelDTKReconcilerName).
		For(&v1.Node{}).
		WithEventFilter(
			filter.KernelDTKReconcilerPredicate(),
		).
		Complete(r)
}
