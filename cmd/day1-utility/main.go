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
	fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s MC_NAME MCP_NAME KMOD_NAME IMAGE_NAME\n\n", os.Args[0])
	fmt.Fprintln(flag.CommandLine.Output(), "Arguments:")
	fmt.Fprintln(flag.CommandLine.Output(), "MC_NAME: name of the MachineConfig to be created")
	fmt.Fprintln(flag.CommandLine.Output(), "MCP_NAME: MachineConfigPool name that should be referenced by MachineConfig")
	fmt.Fprintln(flag.CommandLine.Output(), "KMOD_NAME: kernel module name, that should be unloaded (in-tree) and then loaded (oot)")
	fmt.Fprintln(flag.CommandLine.Output(), "IMAGE_NAME: container image that contains kernel module .ko file")
}

func main() {
	flag.Parse()
	flag.Usage = customUsage

	if flag.NArg() != 4 {
		fmt.Println("Wrong number of arguments are provided", "num parameters", flag.NArg(), "git commit", GitCommit, "version", Version)
		flag.Usage()
		os.Exit(1)
	}

	args := flag.Args()

	yaml, err := mcproducer.ProduceMachineConfig(args[0], args[1], args[2], args[3])
	if err != nil {
		fmt.Println("failed to produce MachineConfig yaml", "error", err, "git commit", GitCommit, "version", Version)
		os.Exit(1)
	}

	fmt.Printf("%s", yaml)
}
