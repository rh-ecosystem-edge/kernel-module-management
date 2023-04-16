package mcproducer

import (
	"bytes"
	"embed"
	"encoding/base64"
	"fmt"
	"text/template"

	"github.com/google/go-containerregistry/pkg/name"
)

var (
	//go:embed templates
	templateFS embed.FS

	machineConfigTemplate = template.Must(
		template.ParseFS(templateFS, "templates/machine-config.gotmpl"),
	)

	scriptPullImage = template.Must(
		template.ParseFS(templateFS, "templates/pull-image.gotmpl"),
	)

	scriptReplaceKmod = template.Must(
		template.ParseFS(templateFS, "templates/replace-kernel-module.gotmpl"),
	)
)

func ProduceMachineConfig(machineConfigName, machineConfigPoolRef, kernelModuleImage, kernelModuleName string) (string, error) {
	err := verifyImageFormat(kernelModuleImage)
	if err != nil {
		return "", fmt.Errorf("image %s is in incorrect format: %v", kernelModuleImage, err)
	}

	templateParams := map[string]any{
		"Image":                kernelModuleImage,
		"KernelModule":         kernelModuleName,
		"MachineConfigPoolRef": machineConfigPoolRef,
		"MachineConfigName":    machineConfigName,
	}

	templateParams["ReplaceInTreeDriverContents"], err = executeIntoBase64(scriptReplaceKmod, templateParams)
	if err != nil {
		return "", err
	}

	templateParams["PullKernelModuleContents"], err = executeIntoBase64(scriptPullImage, templateParams)
	if err != nil {
		return "", err
	}

	var machineConfig bytes.Buffer

	if err = machineConfigTemplate.Execute(&machineConfig, templateParams); err != nil {
		return "", fmt.Errorf("could not render the MachineConfig: %v", err)
	}

	return machineConfig.String(), nil
}

func executeIntoBase64(tmpl *template.Template, params map[string]any) (string, error) {
	var buf bytes.Buffer

	enc := base64.NewEncoder(base64.StdEncoding, &buf)

	if err := tmpl.Execute(enc, params); err != nil {
		return "", fmt.Errorf("could not render %s: %v", tmpl.Name(), err)
	}

	if err := enc.Close(); err != nil {
		return "", err
	}

	return buf.String(), nil
}

func verifyImageFormat(image string) error {
	_, digestErr := name.NewDigest(image)
	if digestErr == nil {
		return nil
	}
	_, tagErr := name.NewTag(image)
	if tagErr == nil {
		return nil
	}
	return fmt.Errorf("invalid image %s, input should be either in tag or digest format. Digest error %v, Tag error %v", image, digestErr, tagErr)
}
