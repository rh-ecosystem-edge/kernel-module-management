package main

import (
	"fmt"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/worker"
	"github.com/spf13/cobra"
)

func rootFuncPreRunE(cmd *cobra.Command, args []string) error {
	logger.Info("Starting worker", "version", Version, "git commit", GitCommit)

	im, err := getImageMounter(cmd)
	if err != nil {
		return fmt.Errorf("failed to get appropriate ImageMounter: %v", err)
	}
	mr := worker.NewModprobeRunner(logger)
	w = worker.NewWorker(im, mr, logger)

	return nil
}

func kmodLoadFunc(cmd *cobra.Command, args []string) error {
	cfgPath := args[0]

	logger.V(1).Info("Reading config", "path", cfgPath)

	cfg, err := configHelper.ReadConfigFile(cfgPath)
	if err != nil {
		return fmt.Errorf("could not read config file %s: %v", cfgPath, err)
	}

	if flag := cmd.Flags().Lookup(worker.FlagFirmwareClassPath); flag.Changed {
		logger.V(1).Info(worker.FlagFirmwareClassPath + " set, setting firmware_class.path")

		if err := w.SetFirmwareClassPath(flag.Value.String()); err != nil {
			return fmt.Errorf("could not set the firmware_class.path parameter: %v", err)
		}
	}

	mountPathFlag := cmd.Flags().Lookup(worker.FlagFirmwareMountPath)

	return w.LoadKmod(cmd.Context(), cfg, mountPathFlag.Value.String())
}

func kmodUnloadFunc(cmd *cobra.Command, args []string) error {
	cfgPath := args[0]

	logger.V(1).Info("Reading config", "path", cfgPath)

	cfg, err := configHelper.ReadConfigFile(cfgPath)
	if err != nil {
		return fmt.Errorf("could not read config file %s: %v", cfgPath, err)
	}

	mountPathFlag := cmd.Flags().Lookup(worker.FlagFirmwareMountPath)

	return w.UnloadKmod(cmd.Context(), cfg, mountPathFlag.Value.String())
}

func setCommandsFlags() {
	kmodLoadCmd.Flags().String(
		worker.FlagFirmwareClassPath,
		"",
		"if set, this value will be written to "+worker.FirmwareClassPathLocation,
	)

	kmodLoadCmd.Flags().String(
		worker.FlagFirmwareMountPath,
		"",
		"if set, this the value that firmware host path is mounted to")

	kmodLoadCmd.Flags().Bool(
		"tarball",
		false,
		"If true, extract the image from a tarball image instead of pulling from the registry",
	)

	kmodUnloadCmd.Flags().String(
		worker.FlagFirmwareMountPath,
		"",
		"if set, this the value that firmware host path is mounted to")

	kmodUnloadCmd.Flags().Bool(
		"tarball",
		false,
		"If true, extract the image from a tarball image instead of pulling from the registry",
	)
}

func getImageMounter(cmd *cobra.Command) (worker.ImageMounter, error) {
	flag := cmd.Flags().Lookup("tarball")
	if flag.Changed {
		return worker.NewTarImageMounter(worker.ImagesDir, logger), nil
	}

	logger.Info("Reading pull secrets", "base dir", worker.PullSecretsDir)
	keyChain, err := worker.ReadKubernetesSecrets(cmd.Context(), worker.PullSecretsDir, logger)
	if err != nil {
		return nil, fmt.Errorf("could not read pull secrets: %v", err)
	}
	res := worker.NewMirrorResolver(logger)
	return worker.NewRemoteImageMounter(worker.ImagesDir, res, keyChain, logger), nil
}
