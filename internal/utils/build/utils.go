package build

import (
	"fmt"

	buildv1 "github.com/openshift/api/build/v1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/constants"
)

const (
	HashAnnotation = "kmm.node.kubernetes.io/last-hash"

	buildTypeLabel = "kmm.openshift.io/build.type"
)

func GetBuildAnnotations(hash uint64) map[string]string {
	return map[string]string{HashAnnotation: fmt.Sprintf("%d", hash)}
}

func GetBuildLabels(mld *api.ModuleLoaderData, buildType string) map[string]string {
	return map[string]string{
		constants.ModuleNameLabel:    mld.Name,
		constants.TargetKernelTarget: mld.KernelVersion,
		buildTypeLabel:               buildType,
	}
}

func IsBuildChanged(existingBuild *buildv1.Build, newBuild *buildv1.Build) (bool, error) {
	existingAnnotations := existingBuild.GetAnnotations()
	newAnnotations := newBuild.GetAnnotations()
	if existingAnnotations == nil {
		return false, fmt.Errorf("annotations are not present in the existing Build %s", existingBuild.Name)
	}
	return existingAnnotations[HashAnnotation] != newAnnotations[HashAnnotation], nil
}
