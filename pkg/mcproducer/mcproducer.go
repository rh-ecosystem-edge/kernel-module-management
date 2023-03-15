package mcproducer

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"text/template"

	"github.com/google/go-containerregistry/pkg/name"
)

//go:embed templates
var templateFS embed.FS

func ProduceMachineConfig(machineConfigName, machineConfigPoolRef, kernelModuleImage, kernelModuleName string) (string, error) {
	tag, err := name.NewTag(kernelModuleImage)
	if err != nil {
		return "", fmt.Errorf("invalid kernelModuleImage %s, input should be repo:tag format: %v", kernelModuleImage, err)
	}

	templateParams := map[string]interface{}{
		"Image":                tag.Repository.Name(),
		"Tag":                  tag.Identifier(),
		"KernelModule":         kernelModuleName,
		"MachineConfigPoolRef": machineConfigPoolRef,
		"MachineConfigName":    machineConfigName,
	}

	replaceKernelModuleScript, err := generateTemplate(templateFS, "templates/replace-kernel-module.gotmpl", templateParams)
	if err != nil {
		return "", err
	}
	pullKernelModuleScript, err := generateTemplate(templateFS, "templates/pull-image.gotmpl", templateParams)
	if err != nil {
		return "", err
	}
	replaceScriptBase64 := base64.StdEncoding.EncodeToString([]byte(replaceKernelModuleScript))
	pullScriptBase64 := base64.StdEncoding.EncodeToString([]byte(pullKernelModuleScript))

	templateParams["ReplaceInTreeDriverContents"] = replaceScriptBase64
	templateParams["PullKernelModuleContents"] = pullScriptBase64

	machineConfigYAMLString, err := generateTemplate(templateFS, "templates/machine-config.gotmpl", templateParams)
	if err != nil {
		return "", err
	}
	return machineConfigYAMLString, nil
}

func generateTemplate(fsys embed.FS, fileName string, params map[string]interface{}) (string, error) {
	tmpl, err := template.ParseFS(fsys, fileName)
	if err != nil {
		return "", fmt.Errorf("failed to prepare template from file %s: %v", fileName, err)
	}
	buf := &bytes.Buffer{}
	err = tmpl.Execute(buf, params)
	if err != nil {
		return "", fmt.Errorf("failed to execute template parsing for file %s: %v", fileName, err)
	}
	return buf.String(), nil
}
