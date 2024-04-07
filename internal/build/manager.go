package build

import (
	"context"
	"time"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils/ocpbuild"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/api"
)

//go:generate mockgen -source=manager.go -package=build -destination=mock_manager.go

type Manager interface {
	GarbageCollect(ctx context.Context, modName, namespace string, owner metav1.Object, delay time.Duration) ([]string, error)

	ShouldSync(ctx context.Context, mld *api.ModuleLoaderData) (bool, error)

	Sync(
		ctx context.Context,
		mld *api.ModuleLoaderData,
		pushImage bool,
		owner metav1.Object) (ocpbuild.Status, error)
}
