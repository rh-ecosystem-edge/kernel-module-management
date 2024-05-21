/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/preflight"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	openapivi "github.com/openshift/api/image/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta2"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/auth"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/filter"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/registry"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
)

const (
	driverToolkitSpecName        = "driver-toolkit"
	driverToolkitJSONFilePath    = "etc/driver-toolkit-release.json"
	releaseManifestImagesRefFile = "release-manifests/image-references"

	PreflightValidationOCPReconcilerName = "PreflightValidationOCP"
)

type dtkRelease struct {
	KernelVersion   string `json:"KERNEL_VERSION"`
	RTKernelVersion string `json:"RT_KERNEL_VERSION"`
	RHELVersion     string `json:"RHEL_VERSION"`
}

// PreflightValidationOCPReconciler reconciles a PreflightValidationOCP object
type PreflightValidationOCPReconciler struct {
	client             client.Client
	filter             *filter.Filter
	registry           registry.Registry
	registryAuthGetter auth.RegistryAuthGetter
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping
	statusUpdater      preflight.OCPStatusUpdater
	scheme             *runtime.Scheme
}

func NewPreflightValidationOCPReconciler(
	client client.Client,
	filter *filter.Filter,
	registry registry.Registry,
	authFactory auth.RegistryAuthGetterFactory,
	kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping,
	statusUpdater preflight.OCPStatusUpdater,
	scheme *runtime.Scheme) *PreflightValidationOCPReconciler {
	registryAuthGetter := authFactory.NewClusterAuthGetter()
	return &PreflightValidationOCPReconciler{
		client:             client,
		filter:             filter,
		registry:           registry,
		registryAuthGetter: registryAuthGetter,
		kernelOsDtkMapping: kernelOsDtkMapping,
		statusUpdater:      statusUpdater,
		scheme:             scheme,
	}
}

func (r *PreflightValidationOCPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(PreflightValidationOCPReconcilerName).
		For(&v1beta2.PreflightValidationOCP{}, builder.WithPredicates(filter.PreflightOCPReconcilerUpdatePredicate())).
		Owns(&v1beta2.PreflightValidation{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1,
		}).
		Complete(r)
}

//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=preflightvalidations,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=preflightvalidations/status,verbs=get;list;watch
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=preflightvalidations/finalizers,verbs=update
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=preflightvalidationsocp,verbs=get;list;watch;update;patch
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=preflightvalidationsocp/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=kmm.sigs.x-k8s.io,resources=preflightvalidationsocp/finalizers,verbs=update

// Reconcile Reconiliation entry point
func (r *PreflightValidationOCPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	log.Info("Start PreflightValidationOCP Reconciliation")

	pvo := v1beta2.PreflightValidationOCP{}
	err := r.client.Get(ctx, req.NamespacedName, &pvo)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Reconciliation object not found; not reconciling")
			return ctrl.Result{}, nil
		}
		log.Error(err, "preflight validation ocp reconcile failed to find object")
		return ctrl.Result{}, err
	}

	err = r.runPreflightValidationOCP(ctx, &pvo)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("runPreflightValidationOCP failed: %v", err)
	}

	return ctrl.Result{}, nil
}

func (r *PreflightValidationOCPReconciler) runPreflightValidationOCP(ctx context.Context, pvo *v1beta2.PreflightValidationOCP) error {
	log := ctrl.LoggerFrom(ctx)

	// get compatible PreflightValidation
	nsn := types.NamespacedName{Name: pvo.Name, Namespace: pvo.Namespace}
	pv := &v1beta2.PreflightValidation{}
	err := r.client.Get(ctx, nsn, pv)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Compatible PreflightValidation not found, creating")
			pv, err = r.preparePreflightValidation(ctx, pvo)
			if err != nil {
				return fmt.Errorf("failed to prepare the data for compatible PreflightValidation: %v", err)
			}
			err = r.client.Create(ctx, pv)
			if err != nil {
				return fmt.Errorf("failed to create PreflightValidation %s with kernel version %s: %v", nsn, pv.Spec.KernelVersion, err)
			}
			return nil
		}
		return fmt.Errorf("failed to get the compatible PreflightValidation, %s: %v", nsn, err)
	}

	log.Info("Updating status from PV ", "pv", nsn, "pvo status", pvo.Status)

	// update statuses
	err = r.statusUpdater.PreflightOCPUpdateStatus(ctx, pvo, pv)
	if err != nil {
		return fmt.Errorf("failed to update the statuses from compatible PreflightValidations: %v", err)
	}

	// no need to check all the modules verification statuses, it will be done by preflight validation reconciler
	return nil
}

func (r *PreflightValidationOCPReconciler) preparePreflightValidation(ctx context.Context,
	pvo *v1beta2.PreflightValidationOCP) (*v1beta2.PreflightValidation, error) {
	log := ctrl.LoggerFrom(ctx)
	dtkImage, err := r.getDTKFromImage(ctx, pvo.Spec.ReleaseImage)
	if err != nil {
		return nil, fmt.Errorf("failed to get DTK image from Release Image %s: %v", pvo.Spec.ReleaseImage, err)
	}

	log.Info("DTK image is", "dtk_image", dtkImage)

	fullKernelVersion, rtKernelVersion, osVersion, err := r.getKernelVersionAndOSFromDTK(ctx, dtkImage)
	if err != nil {
		return nil, fmt.Errorf("failed to get kernel/os version from DTK image %s: %v", dtkImage, err)
	}

	log.Info("os version and kernel version from driver-toolkit", "os_version", osVersion, "full_kernel_version", fullKernelVersion, "rt_kernel_version", rtKernelVersion)
	kernelVersion := fullKernelVersion
	if pvo.Spec.UseRTKernel {
		if rtKernelVersion == "" {
			return nil, fmt.Errorf("rt_kernel_version is missing for this release, probably not and x86_64 architecture")
		}
		kernelVersion = rtKernelVersion
	}

	// os version in the DTK is different then OS version on the node, so in case regular flow
	// already set the data - we don't want to override it
	if _, err := r.kernelOsDtkMapping.GetImage(kernelVersion); err != nil {
		r.kernelOsDtkMapping.SetNodeInfo(kernelVersion, osVersion)
		r.kernelOsDtkMapping.SetImageStreamInfo(osVersion, dtkImage)
	}

	pv := v1beta2.PreflightValidation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvo.Name,
			Namespace: pvo.Namespace,
		},
		Spec: v1beta2.PreflightValidationSpec{
			KernelVersion:  kernelVersion,
			PushBuiltImage: pvo.Spec.PushBuiltImage,
		},
	}

	err = controllerutil.SetControllerReference(pvo, &pv, r.scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to set owner reference for pv %s/%s: %v", pv.Name, pv.Namespace, err)
	}

	return &pv, nil
}

func (r *PreflightValidationOCPReconciler) getDTKFromImage(ctx context.Context, image string) (string, error) {
	layer, err := r.registry.LastLayer(ctx, image, nil, r.registryAuthGetter)
	if err != nil {
		return "", fmt.Errorf("failed to get last layer of image %s: %v", image, err)
	}
	data, err := r.registry.GetHeaderDataFromLayer(layer, releaseManifestImagesRefFile)
	if err != nil {
		return "", fmt.Errorf("failed to get image spec from image %s: %v", image, err)
	}

	manifestImages := openapivi.ImageStream{}

	err = json.Unmarshal(data, &manifestImages)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal %s data: %v", releaseManifestImagesRefFile, err)
	}

	for _, tag := range manifestImages.Spec.Tags {
		if tag.Name == driverToolkitSpecName {
			if tag.From == nil {
				return "", fmt.Errorf("%s local reference is missing from %s", driverToolkitSpecName, releaseManifestImagesRefFile)
			}
			return tag.From.Name, nil
		}
	}

	return "", fmt.Errorf("failed to find %s entry in the %s file", driverToolkitSpecName, releaseManifestImagesRefFile)
}

func (r *PreflightValidationOCPReconciler) getKernelVersionAndOSFromDTK(ctx context.Context, dtkImage string) (string, string, string, error) {
	log := ctrl.LoggerFrom(ctx)
	digests, repo, err := r.registry.GetLayersDigests(ctx, dtkImage, nil, r.registryAuthGetter)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to get layers digests for DTK image %s: %v", dtkImage, err)
	}
	for i := len(digests) - 1; i >= 0; i-- {
		layer, err := r.registry.GetLayerByDigest(digests[i], repo)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to get layer %d for DTK image %s: %v", i, dtkImage, err)
		}
		data, err := r.registry.GetHeaderDataFromLayer(layer, driverToolkitJSONFilePath)
		if err != nil {
			continue
		}

		dtkData := dtkRelease{}
		err = json.Unmarshal(data, &dtkData)
		if err != nil {
			return "", "", "", fmt.Errorf("failed to unmarshal %s data: %v", driverToolkitJSONFilePath, err)
		}

		log.Info("DTK data is:", "dtkData", data)
		if dtkData.KernelVersion == "" || dtkData.RHELVersion == "" {
			return "", "", "", fmt.Errorf("failed format of %s file, both KernelVersion <%s> and RHEL_VERSION <%s> should not be empty",
				driverToolkitJSONFilePath, dtkData.KernelVersion, dtkData.RHELVersion)
		}
		return dtkData.KernelVersion, dtkData.RTKernelVersion, dtkData.RHELVersion, nil
	}
	return "", "", "", fmt.Errorf("file %s is not present in the image %s", driverToolkitJSONFilePath, dtkImage)
}
