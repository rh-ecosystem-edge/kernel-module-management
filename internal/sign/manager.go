package sign

import (
	"context"

	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/utils"
)

type SignManager interface {
	Sync(ctx context.Context, mod kmmv1beta1.Module, m kmmv1beta1.KernelMapping, targetKernel string, imageToSign string, targetImage string, pushImage bool) (utils.Result, error)
}
