package buildconfig

import (
	"context"
	"errors"
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	kmmbuild "github.com/rh-ecosystem-edge/kernel-module-management/internal/build"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const buildHashAnnotation = "kmm.node.kubernetes.io/last-hash"

var (
	errNoMatchingBuild = errors.New("no matching Build")
)

type buildManager struct {
	client          client.Client
	maker           Maker
	ocpBuildsHelper OpenShiftBuildsHelper
}

func NewManager(client client.Client, maker Maker, ocpBuildsHelper OpenShiftBuildsHelper) *buildManager {
	return &buildManager{
		client:          client,
		maker:           maker,
		ocpBuildsHelper: ocpBuildsHelper,
	}
}

func (bcm *buildManager) GarbageCollect(ctx context.Context, mod kmmv1beta1.Module) ([]string, error) {

	//Garbage Collection noti (yet) implemented for Build
	return nil, nil
}

func (bcm *buildManager) Sync(
	ctx context.Context,
	mod kmmv1beta1.Module,
	m kmmv1beta1.KernelMapping,
	targetKernel,
	targetImage string,
	pushImage bool,
) (kmmbuild.Result, error) {

	logger := log.FromContext(ctx)

	buildTemplate, err := bcm.maker.MakeBuildTemplate(ctx, mod, m, targetKernel, targetImage, pushImage)
	if err != nil {
		return kmmbuild.Result{}, fmt.Errorf("could not make Build template: %v", err)
	}

	build, err := bcm.ocpBuildsHelper.GetBuild(ctx, mod, targetKernel)
	if err != nil {
		if !errors.Is(err, errNoMatchingBuild) {
			return kmmbuild.Result{}, fmt.Errorf("error getting the build: %v", err)
		}

		logger.Info("Creating Build")

		if err = bcm.client.Create(ctx, buildTemplate); err != nil {
			return kmmbuild.Result{}, fmt.Errorf("could not create Build: %v", err)
		}

		return kmmbuild.Result{Status: kmmbuild.StatusCreated, Requeue: true}, nil
	}

	changed, err := bcm.isBuildChanged(build, buildTemplate)
	if err != nil {
		return kmmbuild.Result{}, fmt.Errorf("could not determine if Build has changed: %v", err)
	}

	if changed {
		logger.Info("The module's build spec has been changed, deleting the current Build so a new one can be created", "name", build.Name)
		opts := []client.DeleteOption{
			client.PropagationPolicy(metav1.DeletePropagationBackground),
		}
		err = bcm.client.Delete(ctx, build, opts...)
		if err != nil {
			logger.Info(utils.WarnString(fmt.Sprintf("failed to delete Build %s: %v", build.Name, err)))
		}
		return kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, nil
	}

	switch build.Status.Phase {
	case buildv1.BuildPhaseComplete:
		return kmmbuild.Result{Status: kmmbuild.StatusCompleted}, nil
	case buildv1.BuildPhaseNew, buildv1.BuildPhasePending, buildv1.BuildPhaseRunning:
		return kmmbuild.Result{Status: kmmbuild.StatusInProgress, Requeue: true}, nil
	case buildv1.BuildPhaseFailed:
		return kmmbuild.Result{}, fmt.Errorf("build failed: %v", build.Status.LogSnippet)
	default:
		return kmmbuild.Result{}, fmt.Errorf("unknown status: %v", build.Status)
	}
}

func (bcm *buildManager) isBuildChanged(existingBuild *buildv1.Build, newBuild *buildv1.Build) (bool, error) {
	existingAnnotations := existingBuild.GetAnnotations()
	newAnnotations := newBuild.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing Build %s", existingBuild.Name)
	}
	if existingAnnotations[buildHashAnnotation] == newAnnotations[buildHashAnnotation] {
		return false, nil
	}
	return true, nil
}

//go:generate mockgen -source=manager.go -package=buildconfig -destination=mock_manager.go

type OpenShiftBuildsHelper interface {
	GetBuild(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*buildv1.Build, error)
}

type openShiftBuildsHelper struct {
	client client.Client
}

func NewOpenShiftBuildsHelper(client client.Client) OpenShiftBuildsHelper {
	return &openShiftBuildsHelper{client: client}
}

func (osbh *openShiftBuildsHelper) GetBuild(ctx context.Context, mod kmmv1beta1.Module, targetKernel string) (*buildv1.Build, error) {
	buildList := buildv1.BuildList{}

	opts := []client.ListOption{
		client.MatchingLabels(kmmbuild.GetBuildLabels(mod, targetKernel)),
		client.InNamespace(mod.Namespace),
	}

	if err := osbh.client.List(ctx, &buildList, opts...); err != nil {
		return nil, fmt.Errorf("could not list Build: %v", err)
	}

	if n := len(buildList.Items); n == 0 {
		return nil, errNoMatchingBuild
	} else if n > 1 {
		return nil, fmt.Errorf("expected 0 or 1 Builds, got %d", n)
	}

	return &buildList.Items[0], nil
}
