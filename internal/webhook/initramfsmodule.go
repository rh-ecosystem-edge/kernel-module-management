/*
Copyright 2026.

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

package webhook

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"github.com/containers/image/v5/docker/reference"
	"github.com/go-logr/logr"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var initramfsModuleNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// InitramfsModuleValidator validates InitramfsModule resources.
type InitramfsModuleValidator struct {
	logger logr.Logger
	client client.Reader
}

func NewInitramfsModuleValidator(logger logr.Logger) *InitramfsModuleValidator {
	return &InitramfsModuleValidator{logger: logger}
}

func (v *InitramfsModuleValidator) SetupWebhookWithManager(mgr ctrl.Manager) error {
	v.client = mgr.GetClient()

	return ctrl.NewWebhookManagedBy(mgr).
		For(&kmmv1beta1.InitramfsModule{}).
		WithValidator(v).
		Complete()
}

func (v *InitramfsModuleValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	irm, ok := obj.(*kmmv1beta1.InitramfsModule)
	if !ok {
		return nil, fmt.Errorf("bad type for the object; expected %T, got %T", irm, obj)
	}

	v.logger.Info("Validating InitramfsModule creation", "name", irm.Name, "namespace", irm.Namespace)

	if err := validateInitramfsModule(irm); err != nil {
		return nil, err
	}

	if err := v.validateSingleton(ctx, irm, false); err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *InitramfsModuleValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldIRM, ok := oldObj.(*kmmv1beta1.InitramfsModule)
	if !ok {
		return nil, fmt.Errorf("bad type for the old object; expected %T, got %T", oldIRM, oldObj)
	}

	newIRM, ok := newObj.(*kmmv1beta1.InitramfsModule)
	if !ok {
		return nil, fmt.Errorf("bad type for the new object; expected %T, got %T", newIRM, newObj)
	}

	v.logger.Info("Validating InitramfsModule update", "name", oldIRM.Name, "namespace", oldIRM.Namespace)

	if err := validateInitramfsModule(newIRM); err != nil {
		return nil, err
	}

	if err := v.validateSingleton(ctx, newIRM, true); err != nil {
		return nil, err
	}

	return nil, nil
}

func (v *InitramfsModuleValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, NotImplemented
}

func validateInitramfsModule(irm *kmmv1beta1.InitramfsModule) error {
	if err := validateInitramfsModuleNames("spec.moduleNames", irm.Spec.ModuleNames, true); err != nil {
		return err
	}

	if err := validateInitramfsModuleNames("spec.omit", irm.Spec.Omit, false); err != nil {
		return err
	}

	if err := validateInitramfsContainerImage(irm.Spec.ContainerImage); err != nil {
		return err
	}

	dirName := irm.Spec.DirName
	if dirName == "" {
		dirName = "/opt"
	}
	if err := validateSignSection(irm.Spec.Sign, dirName); err != nil {
		return fmt.Errorf("failed to validate sign: %v", err)
	}

	return nil
}

func validateInitramfsModuleNames(field string, names []string, required bool) error {
	if required && len(names) == 0 {
		return fmt.Errorf("%s must contain at least one entry", field)
	}

	seen := make(map[string]struct{}, len(names))
	for i, name := range names {
		if !initramfsModuleNamePattern.MatchString(name) {
			return fmt.Errorf("%s[%d] %q must match %q", field, i, name, initramfsModuleNamePattern.String())
		}
		if _, found := seen[name]; found {
			return fmt.Errorf("%s[%d] %q is a duplicate", field, i, name)
		}
		seen[name] = struct{}{}
	}

	return nil
}

func validateInitramfsContainerImage(image string) error {
	parsed, err := reference.Parse(image)
	if err != nil {
		return fmt.Errorf("spec.containerImage is invalid: %w", err)
	}

	if _, ok := parsed.(reference.Digested); ok {
		return errors.New("spec.containerImage must use a tag and must not include a digest")
	}

	if _, ok := parsed.(reference.NamedTagged); !ok {
		return errors.New("spec.containerImage must include a tag")
	}

	return nil
}

func (v *InitramfsModuleValidator) validateSingleton(ctx context.Context, irm *kmmv1beta1.InitramfsModule, allowCurrent bool) error {
	if v.client == nil {
		return errors.New("InitramfsModule validator client is not configured")
	}

	var list kmmv1beta1.InitramfsModuleList
	if err := v.client.List(ctx, &list); err != nil {
		return fmt.Errorf("failed to list InitramfsModules for singleton validation: %w", err)
	}

	for i := range list.Items {
		existing := &list.Items[i]
		if allowCurrent && existing.Namespace == irm.Namespace && existing.Name == irm.Name {
			continue
		}

		return fmt.Errorf("only one InitramfsModule is allowed in the cluster; existing object is %s/%s", existing.Namespace, existing.Name)
	}

	return nil
}
