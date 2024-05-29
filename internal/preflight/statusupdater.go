package preflight

import (
	"context"
	"fmt"
	"slices"

	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

//go:generate mockgen -source=statusupdater.go -package=preflight -destination=mock_statusupdater.go

type StatusUpdater interface {
	PresetStatuses(
		ctx context.Context,
		pv *v1beta2.PreflightValidation,
		existingModules sets.Set[types.NamespacedName],
		newModules []types.NamespacedName,
	) error
	SetVerificationStatus(
		ctx context.Context,
		preflight *v1beta2.PreflightValidation,
		moduleName types.NamespacedName,
		verificationStatus string,
		message string,
	) error
	SetVerificationStage(
		ctx context.Context,
		preflight *v1beta2.PreflightValidation,
		moduleName types.NamespacedName,
		stage string,
	) error
}

type statusUpdater struct {
	client client.Client
}

func NewStatusUpdater(client client.Client) StatusUpdater {
	return &statusUpdater{
		client: client,
	}
}

func (p *statusUpdater) PresetStatuses(
	ctx context.Context,
	pv *v1beta2.PreflightValidation,
	existingModules sets.Set[types.NamespacedName],
	newModules []types.NamespacedName,
) error {
	pv.Status.Modules = slices.DeleteFunc(pv.Status.Modules, func(status v1beta2.PreflightValidationModuleStatus) bool {
		nsn := types.NamespacedName{
			Namespace: status.Namespace,
			Name:      status.Name,
		}

		return !existingModules.Has(nsn)
	})

	for _, nsn := range newModules {
		status := v1beta2.PreflightValidationModuleStatus{
			Namespace: nsn.Namespace,
			Name:      nsn.Name,
			CRBaseStatus: v1beta2.CRBaseStatus{
				VerificationStatus: v1beta1.VerificationFalse,
				VerificationStage:  v1beta1.VerificationStageImage,
				LastTransitionTime: metav1.Now(),
			},
		}

		pv.Status.Modules = append(pv.Status.Modules, status)
	}

	return p.client.Status().Update(ctx, pv)
}

func (p *statusUpdater) SetVerificationStatus(
	ctx context.Context,
	pv *v1beta2.PreflightValidation,
	moduleName types.NamespacedName,
	verificationStatus string,
	message string,
) error {
	status, ok := FindModuleStatus(pv.Status.Modules, moduleName)
	if !ok {
		return fmt.Errorf("failed to find module status %s in preflight %s", moduleName, pv.Name)
	}

	status.VerificationStatus = verificationStatus
	status.StatusReason = message
	status.LastTransitionTime = metav1.Now()

	return p.client.Status().Update(ctx, pv)
}

func (p *statusUpdater) SetVerificationStage(
	ctx context.Context,
	pv *v1beta2.PreflightValidation,
	moduleName types.NamespacedName,
	stage string,
) error {
	status, ok := FindModuleStatus(pv.Status.Modules, moduleName)
	if !ok {
		return fmt.Errorf("failed to find module status %s in preflight %s", moduleName, pv.Name)
	}

	status.VerificationStage = stage
	status.LastTransitionTime = metav1.Now()

	return p.client.Status().Update(ctx, pv)
}

//go:generate mockgen -source=statusupdater.go -package=preflight -destination=mock_statusupdater.go

type OCPStatusUpdater interface {
	PreflightOCPUpdateStatus(ctx context.Context, pvo *v1beta2.PreflightValidationOCP, pv *v1beta2.PreflightValidation) error
}

func NewOCPStatusUpdater(client client.Client) OCPStatusUpdater {
	return &ocpStatusUpdater{
		client: client,
	}
}

type ocpStatusUpdater struct {
	client client.Client
}

func (p *ocpStatusUpdater) PreflightOCPUpdateStatus(ctx context.Context, pvo *v1beta2.PreflightValidationOCP, pv *v1beta2.PreflightValidation) error {
	pv.Status.DeepCopyInto(&pvo.Status)
	return p.client.Status().Update(ctx, pvo)
}
