package utils

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/a8m/envsubst/parse"
)

const (
	kernelVersionMajorIdx = 0
	kernelVersionMinorIdx = 1
	kernelVersionPatchIdx = 2
)

var KernelRegexp = regexp.MustCompile("[.,-]")

func KernelComponentsAsEnvVars(kernel string) ([]string, error) {
	osConfigFieldsList := KernelRegexp.Split(kernel, -1)
	if len(osConfigFieldsList) < 3 {
		return nil, fmt.Errorf("invalid kernel version %s: expected at least three components (major.minor.patch)", kernel)
	}

	envvars := []string{
		"KERNEL_FULL_VERSION=" + kernel,
		"KERNEL_VERSION=" + kernel,
		"KERNEL_XYZ=" + strings.Join(osConfigFieldsList[:kernelVersionPatchIdx+1], "."),
		"KERNEL_X=" + osConfigFieldsList[kernelVersionMajorIdx],
		"KERNEL_Y=" + osConfigFieldsList[kernelVersionMinorIdx],
		"KERNEL_Z=" + osConfigFieldsList[kernelVersionPatchIdx],
	}

	return envvars, nil
}

func ReplaceInTemplates(envvars []string, templates ...string) ([]string, error) {
	parser := parse.New("mapping", envvars, &parse.Restrictions{})

	replacedStrings := make([]string, 0, len(templates))

	for _, v := range templates {
		resultString, err := parser.Parse(v)
		if err != nil {
			return nil, fmt.Errorf("failed to substitute %q: %v", v, err)
		}

		replacedStrings = append(replacedStrings, resultString)
	}
	return replacedStrings, nil
}
