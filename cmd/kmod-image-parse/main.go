/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	GitCommit = "undefined"
	Version   = "undefined"
)

var rootCmd = &cobra.Command{
	CompletionOptions: cobra.CompletionOptions{DisableDefaultCmd: true},
	SilenceUsage:      true,
	SilenceErrors:     true,
	Use:               "kmod-image-parse",
	Version:           Version,
	RunE:              parseFunc,
}

func main() {
	setCommandsFlags(rootCmd)
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s (%s): %s\n", Version, GitCommit, err)
		os.Exit(1)
	}
}
