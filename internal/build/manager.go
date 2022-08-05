package build

import (
	"context"

	kmmv1alpha1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1alpha1"
)

type Status string

const (
	StatusCompleted  = "completed"
	StatusCreated    = "created"
	StatusInProgress = "in progress"
)

type Result struct {
	Requeue bool
	Status  Status
}

//go:generate mockgen -source=manager.go -package=build -destination=mock_manager.go

type Manager interface {
	Sync(ctx context.Context, mod kmmv1alpha1.Module, m kmmv1alpha1.KernelMapping, targetKernel string) (Result, error)
}
