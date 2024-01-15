package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/go-logr/logr"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

type tarImageMounter struct {
	ociImageHelper ociImageMounterHelperAPI
	logger         logr.Logger
	baseDir        string
}

func NewTarImageMounter(baseDir string, logger logr.Logger) ImageMounter {
	ociImageHelper := newOCIImageMounterHelper(logger)
	return &tarImageMounter{
		ociImageHelper: ociImageHelper,
		logger:         logger,
		baseDir:        baseDir,
	}
}

func (tim *tarImageMounter) MountImage(ctx context.Context, imagePath string, cfg *kmmv1beta1.ModuleConfig) (string, error) {
	dstDir := filepath.Join(tim.baseDir, imagePath)

	dstDirFS := filepath.Join(dstDir, "fs")
	if err := os.MkdirAll(dstDirFS, os.ModeDir|0755); err != nil {
		return "", fmt.Errorf("could not create the filesystem directory %s: %v", dstDirFS, err)
	}

	tim.logger.V(1).Info("Converting tarball into OCI image")

	img, err := tarball.ImageFromPath(imagePath, nil)
	if err != nil {
		return "", fmt.Errorf("could not create image from path  %s: %v", imagePath, err)
	}

	err = tim.ociImageHelper.mountOCIImage(img, dstDirFS)
	if err != nil {
		return "", fmt.Errorf("failed mounting oci image: %v", err)
	}

	tim.logger.Info("Image written to the filesystem")

	return dstDirFS, nil
}
