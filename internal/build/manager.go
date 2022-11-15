package build

import (
	"context"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/syncronizedmap"
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
	Sync(ctx context.Context, mod kmmv1beta1.Module, m kmmv1beta1.KernelMapping, targetKernel string, targetImage string,
		pushImage bool, kernelOsDtkMapping syncronizedmap.KernelOsDtkMapping) (Result, error)
	GarbageCollect(ctx context.Context, mod kmmv1beta1.Module) ([]string, error)
}
