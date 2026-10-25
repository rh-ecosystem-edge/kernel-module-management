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
	"encoding/json"
	"os"

	"github.com/rh-ecosystem-edge/kernel-module-management/internal/kmodparser"
	"github.com/spf13/cobra"
)

var terminationMessagePath = "/dev/termination-log"

var kmodParser kmodparser.Parser = kmodparser.NewParser()

func parseFunc(cmd *cobra.Command, _ []string) error {
	moduleNames, err := cmd.Flags().GetStringArray("modules-names")
	if err != nil {
		return err
	}
	omit, err := cmd.Flags().GetStringArray("omit")
	if err != nil {
		return err
	}
	modulesRoot, err := cmd.Flags().GetString("modules-root")
	if err != nil {
		return err
	}
	success, parseErr := kmodParser.Parse(kmodparser.Request{
		ModuleNames: moduleNames,
		Omit:        omit,
		ModulesRoot: modulesRoot,
	})
	if parseErr != nil {
		if err := writeTerminationLog(terminationMessagePath, kmodparser.ErrorBody{Error: parseErr.Error()}); err != nil {
			return err
		}
		return parseErr
	}
	return writeTerminationLog(terminationMessagePath, success)
}

func writeTerminationLog(path string, body any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func setCommandsFlags(cmd *cobra.Command) {
	cmd.Flags().StringArray("modules-names", nil, "module names to require in modules.dep")
	cmd.Flags().StringArray("omit", nil, "module name to leave out of the result")
	cmd.Flags().String("modules-root", "/modules", "directory that contains modules.dep")
}
