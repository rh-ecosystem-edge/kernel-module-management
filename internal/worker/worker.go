package worker

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/go-logr/logr"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

type Worker interface {
	LoadKmod(ctx context.Context, cfg *kmmv1beta1.ModuleConfig) error
	UnloadKmod(ctx context.Context, cfg *kmmv1beta1.ModuleConfig) error
}

type worker struct {
	ip     ImagePuller
	logger logr.Logger
	mr     ModprobeRunner
	res    MirrorResolver
}

func NewWorker(ip ImagePuller, mr ModprobeRunner, res MirrorResolver, logger logr.Logger) Worker {
	return &worker{
		ip:     ip,
		logger: logger,
		mr:     mr,
		res:    res,
	}
}

func (w *worker) LoadKmod(ctx context.Context, cfg *kmmv1beta1.ModuleConfig) error {
	imageName := cfg.ContainerImage

	pr, err := w.pullImageOrMirror(ctx, imageName, cfg)
	if err != nil {
		return fmt.Errorf("could not pull %s or any of its mirrors: %v", imageName, err)
	}

	if inTree := cfg.InTreeModuleToRemove; inTree != "" {
		w.logger.Info("Unloading in-tree module", "name", inTree)

		if err = w.mr.Run(ctx, "-rv", inTree); err != nil {
			return fmt.Errorf("could not remove in-tree module %s: %v", inTree, err)
		}
	}

	// TODO copy firmware
	// TODO handle ModulesLoadingOrder

	moduleName := cfg.Modprobe.ModuleName

	var args []string

	if cfg.Modprobe.RawArgs != nil {
		args = cfg.Modprobe.RawArgs.Load
	} else {
		args = []string{"-vd", filepath.Join(pr.fsDir, cfg.Modprobe.DirName)}

		if cfg.Modprobe.Args != nil {
			args = append(args, cfg.Modprobe.Args.Load...)
		}

		args = append(args, moduleName)
		args = append(args, cfg.Modprobe.Parameters...)
	}

	return w.mr.Run(ctx, args...)
}

func (w *worker) UnloadKmod(ctx context.Context, cfg *kmmv1beta1.ModuleConfig) error {
	imageName := cfg.ContainerImage

	pr, err := w.pullImageOrMirror(ctx, imageName, cfg)
	if err != nil {
		return fmt.Errorf("could not pull %s or any of its mirrors: %v", imageName, err)
	}

	moduleName := cfg.Modprobe.ModuleName

	var args []string

	if cfg.Modprobe.RawArgs != nil {
		args = cfg.Modprobe.RawArgs.Unload
	} else {
		args = []string{"-rvd", filepath.Join(pr.fsDir, cfg.Modprobe.DirName)}

		if cfg.Modprobe.Args != nil {
			args = append(args, cfg.Modprobe.Args.Unload...)
		}

		args = append(args, moduleName)
	}

	w.logger.Info("Unloading module", "name", moduleName)

	if err = w.mr.Run(ctx, args...); err != nil {
		return fmt.Errorf("could not unload module %s: %v", moduleName, err)
	}

	// TODO remove firmware

	return nil
}

func (w *worker) pullImageOrMirror(ctx context.Context, imageName string, cfg *kmmv1beta1.ModuleConfig) (PullResult, error) {
	imageNames, err := w.res.GetAllReferences(imageName)
	if err != nil {
		return PullResult{}, fmt.Errorf("could not resolve all mirrored names for %q: %v", imageName, err)
	}

	var (
		ok = false
		pr = PullResult{}
	)

	for _, in := range imageNames {
		logger := w.logger.WithValues("image name", in)
		logger.Info("Pulling image")

		pr, err = w.ip.PullAndExtract(ctx, in, cfg.InsecurePull)
		if err != nil {
			logger.Error(err, "Could not pull image")
			continue
		}

		logger.Info("Image pulled successfully", "dir", pr.fsDir)
		ok = true
		break
	}

	if !ok {
		return pr, errors.New("all mirrors tried")
	}

	return pr, nil
}
