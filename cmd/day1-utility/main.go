package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/rh-ecosystem-edge/kernel-module-management/pkg/mcproducer"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
)

func customUsage() {
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s MC_NAME MCP_NAME IMAGE_NAME KMOD_NAME IN_TREE_KMOD_TO_REMOVE WORKER_IMAGE\n\n", os.Args[0])
	fmt.Fprintln(flag.CommandLine.Output(), "Arguments:")
	fmt.Fprintln(flag.CommandLine.Output(), "MC_NAME: name of the MachineConfig to be created")
	fmt.Fprintln(flag.CommandLine.Output(), "MCP_NAME: MachineConfigPool name that should be referenced by MachineConfig")
	fmt.Fprintln(flag.CommandLine.Output(), "IMAGE_NAME: container image that contains kernel module .ko file")
	fmt.Fprintln(flag.CommandLine.Output(), "KMOD_NAME: kernel module name, that should be loaded (oot)")
	fmt.Fprintln(flag.CommandLine.Output(), "IN_TREE_KMOD_TO_REMOVE: in-tree kernel module name, that should be unloaded.If no need - pass empty string")
	fmt.Fprintln(flag.CommandLine.Output(), "FIRMWARE_FILES_PATH: path of the firmware files.Passing empty string means there are no files")
	fmt.Fprintln(flag.CommandLine.Output(), "WORKER_IMAGE: kernel-management worker image to use.Passing empty string means using default image")
}

func main() {
	var (
		mcName             string
		mpName             string
		kernelModuleImage  string
		kernelModule       string
		workerImage        string
		inTreeKernelModule string
		firmwareFilesPath  string
	)
	flag.StringVar(&mcName, "machine-config", "", "name of the machine config to create")
	flag.StringVar(&mpName, "machine-config-pool", "", "name of the machine config pool to use")
	flag.StringVar(&kernelModuleImage, "image", "", "container image that contains kernel module .ko file")
	flag.StringVar(&kernelModule, "kernel-module", "", "container image that contains kernel module .ko file")
	flag.StringVar(&workerImage, "worker-image", "", "kernel-management worker image to use. If not passed, a default value will be used")
	flag.StringVar(&inTreeKernelModule, "in-tree-module-to-remove", "", "in-tree kernel module that should be removed prior to loading the oot module")
	flag.StringVar(&firmwareFilesPath, "firmware-files-path", "", "path where the firmware files are located at")

	flag.Parse()
	flag.Usage = customUsage

	yaml, err := mcproducer.ProduceMachineConfig(mcName, mpName, kernelModuleImage, kernelModule, inTreeKernelModule, firmwareFilesPath, workerImage)
	if err != nil {
		fmt.Println("failed to produce MachineConfig yaml", "error", err, "git commit", GitCommit, "version", Version)
		os.Exit(1)
	}

	fmt.Printf("%s", yaml)
}
