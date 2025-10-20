package mcfg

import (
	"fmt"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"os"

	"github.com/google/go-cmp/cmp"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	apioperatorv1 "github.com/openshift/api/operator/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
)

var _ = Describe("UpdateDisruptionPolicies", func() {
	It("checking the flow", func() {
		mc := &apioperatorv1.MachineConfiguration{}
		bmc := &kmmv1beta1.BootModuleConfig{
			Spec: kmmv1beta1.BootModuleConfigSpec{
				MachineConfigName: "test-name",
			},
		}
		mcfgAPI := NewMCFG("")
		mcfgAPI.UpdateDisruptionPolicies(mc, bmc)

		expectedUnit := apioperatorv1.NodeDisruptionPolicySpecUnit{
			Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
				{
					Type: apioperatorv1.NoneSpecAction,
				},
			},
		}

		expectedFile := apioperatorv1.NodeDisruptionPolicySpecFile{
			Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
				{
					Type: apioperatorv1.NoneSpecAction,
				},
			},
		}

		By("check the pull service")
		expectedUnit.Name = "test-name-pull-kernel-module-image.service"
		res := verifyUnitPresent(mc, expectedUnit)
		Expect(res).To(BeTrue())

		By("check the replace service")
		expectedUnit.Name = "test-name-replace-kernel-module.service"
		res = verifyUnitPresent(mc, expectedUnit)
		Expect(res).To(BeTrue())

		By("check the crio-wipe service")
		expectedUnit.Name = "crio-wipe.service"
		res = verifyUnitPresent(mc, expectedUnit)
		Expect(res).To(BeTrue())

		By("replace-kernel-module.sh")
		expectedFile.Path = "/usr/local/bin/replace-kernel-module.sh"
		res = verifyFilePresent(mc, expectedFile)
		Expect(res).To(BeTrue())

		By("pull-kernel-module-image.sh")
		expectedFile.Path = "/usr/local/bin/pull-kernel-module-image.sh"
		res = verifyFilePresent(mc, expectedFile)
		Expect(res).To(BeTrue())

		By("wait-for-dispatcher.sh")
		expectedFile.Path = "/usr/local/bin/wait-for-dispatcher.sh"
		res = verifyFilePresent(mc, expectedFile)
		Expect(res).To(BeTrue())
	})
})

var _ = Describe("UpdateMachineConfig", func() {
	It("adding labels", func() {
		mc := &mcfgv1.MachineConfig{}
		bmc := &kmmv1beta1.BootModuleConfig{
			Spec: kmmv1beta1.BootModuleConfigSpec{
				MachineConfigPoolName: "test-pool-name",
			},
		}

		By("machine config labels are empty")
		mcfgAPI := NewMCFG("")
		err := mcfgAPI.UpdateMachineConfig(mc, bmc)
		Expect(err).To(BeNil())
		expectedLabels := map[string]string{"machineconfiguration.openshift.io/role": "test-pool-name"}
		Expect(mc.GetLabels()).To(Equal(expectedLabels))

		By("machine config labels are not empty")
		mc.SetLabels(map[string]string{"some label": "some value"})
		err = mcfgAPI.UpdateMachineConfig(mc, bmc)
		Expect(err).To(BeNil())
		expectedLabels = map[string]string{"machineconfiguration.openshift.io/role": "test-pool-name", "some label": "some value"}
		Expect(mc.GetLabels()).To(Equal(expectedLabels))

		By("machine config labels are not empty and machine pool label is set")
		mc.SetLabels(map[string]string{"some label": "some value", "machineconfiguration.openshift.io/role": "test-pool-name"})
		err = mcfgAPI.UpdateMachineConfig(mc, bmc)
		Expect(err).To(BeNil())
		expectedLabels = map[string]string{"machineconfiguration.openshift.io/role": "test-pool-name", "some label": "some value"}
		Expect(mc.GetLabels()).To(Equal(expectedLabels))
	})

})

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
		mcfgAPI := NewMCFG("")

		_, yamlRes, err := mcfgAPI.GenerateIgnition(imageName, kernelModuleName, inTreeKernelModuleName, firmwareFilesPath, workerImage, "test-mc")

		Expect(err).ToNot(HaveOccurred())
		expectedRes, err := os.ReadFile("testdata/ignition-test.yaml")
		Expect(err).ToNot(HaveOccurred())

		if string(expectedRes) != yamlRes {
			fmt.Printf("<%s>\n", yamlRes)
			fmt.Printf("<%s>\n", expectedRes)
		}
		Expect(string(yamlRes)).To(Equal(string(expectedRes)))
	})

	It("verify the usage of the default worker image", func() {
		mcfgAPI := NewMCFG(workerImage)

		_, yamlRes, err := mcfgAPI.GenerateIgnition(imageName, kernelModuleName, inTreeKernelModuleName, firmwareFilesPath, "", "test-mc")

		Expect(err).ToNot(HaveOccurred())
		expectedRes, err := os.ReadFile("testdata/ignition-test.yaml")
		Expect(err).ToNot(HaveOccurred())

		if string(expectedRes) != yamlRes {
			fmt.Printf("<%s>\n", yamlRes)
			fmt.Printf("<%s>\n", expectedRes)
		}
		Expect(string(yamlRes)).To(Equal(string(expectedRes)))
	})
})

func verifyUnitPresent(mc *apioperatorv1.MachineConfiguration, expectedUnit apioperatorv1.NodeDisruptionPolicySpecUnit) bool {
	for _, unit := range mc.Spec.NodeDisruptionPolicy.Units {
		if cmp.Equal(unit, expectedUnit) {
			return true
		}
	}
	return false
}

func verifyFilePresent(mc *apioperatorv1.MachineConfiguration, expectedFile apioperatorv1.NodeDisruptionPolicySpecFile) bool {
	for _, file := range mc.Spec.NodeDisruptionPolicy.Files {
		if cmp.Equal(file, expectedFile) {
			return true
		}
	}
	return false
}
