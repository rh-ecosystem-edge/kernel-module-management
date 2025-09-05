package mcfg

import (
	"encoding/json"
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"gopkg.in/yaml.v3"
	"os"
)

var _ = Describe("GenerateIgnition", func() {
	const (
		name                   = "name"
		mcpRef                 = "mcpRef"
		kernelModuleName       = "testKernelModuleName"
		inTreeKernelModuleName = "testInTreeKernelModuleName"
		firmwareFilesPath      = "/opt/lib/test/firmware"
		imageName              = "quay.io/project/repo:some-tag12"
		workerImage            = "some-worker-image"
	)

	// unit performs verification of the MC output vs the manually created
	// machineconfig-test.yaml. This yaml is created by manually running bas64
	// encoding on testdata/pull-image-test.sh and testdata/replace-kernel-module-test.sh
	// and setting the values into the testdata/machineconfig-test.yaml
	It("verify correct ignition output", func() {
		mcfgAPI := NewMCFG()

		res, err := mcfgAPI.GenerateIgnition(imageName, kernelModuleName, inTreeKernelModuleName, firmwareFilesPath, workerImage, "test-mc")

		Expect(err).ToNot(HaveOccurred())
		expectedRes, err := os.ReadFile("testdata/ignition-test.yaml")
		Expect(err).ToNot(HaveOccurred())

		// convert the result to yaml
		var resData map[string]interface{}
		err = json.Unmarshal(res, &resData)
		Expect(err).ToNot(HaveOccurred())
		resYaml, err := yaml.Marshal(&resData)
		Expect(err).ToNot(HaveOccurred())

		if string(expectedRes) != string(resYaml) {
			fmt.Printf("<%s>\n", resYaml)
			fmt.Printf("<%s>\n", expectedRes)
		}
		Expect(string(resYaml)).To(Equal(string(expectedRes)))
	})
})
