package test

import (
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
)

func TestScheme() (*runtime.Scheme, error) {
	s := runtime.NewScheme()

	if err := scheme.AddToScheme(s); err != nil {
		return nil, err
	}

	if err := kmmv1beta1.AddToScheme(s); err != nil {
		return nil, err
	}

	return s, nil
}
