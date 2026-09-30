package mcfg

import (
	"fmt"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/google/go-cmp/cmp"
	mcfgv1 "github.com/openshift/api/machineconfiguration/v1"
	apioperatorv1 "github.com/openshift/api/operator/v1"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
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

		By("check the pull service")
		res := isUnitPresent(mc, "test-name-pull-kernel-module-image.service")
		Expect(res).To(BeTrue())

		By("check the replace service")
		res = isUnitPresent(mc, "test-name-replace-kernel-module.service")
		Expect(res).To(BeTrue())

		By("check the crio-wipe service")
		res = isUnitPresent(mc, "crio-wipe.service")
		Expect(res).To(BeTrue())

		By("replace-kernel-module.sh")
		res = isFilePresent(mc, "/usr/local/bin/replace-kernel-module.sh")
		Expect(res).To(BeTrue())

		By("pull-kernel-module-image.sh")
		res = isFilePresent(mc, "/usr/local/bin/pull-kernel-module-image.sh")
		Expect(res).To(BeTrue())

		By("wait-for-dispatcher.sh")
		res = isFilePresent(mc, "/usr/local/bin/wait-for-dispatcher.sh")
		Expect(res).To(BeTrue())
	})
})

var _ = Describe("RemoveDisruptionPolicies", func() {
	var (
		mc      *apioperatorv1.MachineConfiguration
		bmc     *kmmv1beta1.BootModuleConfig
		mcfgAPI MCFG
	)

	BeforeEach(func() {
		mc = &apioperatorv1.MachineConfiguration{
			Spec: apioperatorv1.MachineConfigurationSpec{},
		}
		bmc = &kmmv1beta1.BootModuleConfig{
			Spec: kmmv1beta1.BootModuleConfigSpec{
				MachineConfigName: "test-name",
			},
		}

		addUnitSpec(mc, "some unit 1")
		addUnitSpec(mc, "some unit 2")
		addUnitSpec(mc, "test-name-pull-kernel-module-image.service")
		addUnitSpec(mc, "test-name-replace-kernel-module.service")
		addUnitSpec(mc, "crio-wipe.service")
		addUnitSpec(mc, "some unit 3")
		addFileSpec(mc, "some file 1")
		addFileSpec(mc, "/usr/local/bin/replace-kernel-module.sh")
		addFileSpec(mc, "/usr/local/bin/pull-kernel-module-image.sh")
		addFileSpec(mc, "/usr/local/bin/wait-for-dispatcher.sh")
		addFileSpec(mc, "some file 2")
		addFileSpec(mc, "some file 3")

		mcfgAPI = NewMCFG("")
	})

	It("remove all", func() {
		mcfgAPI.RemoveDisruptionPolicies(mc, bmc, true)

		By("check the pull service")
		res := isUnitPresent(mc, "test-name-pull-kernel-module-image.service")
		Expect(res).To(BeFalse())

		By("check the replace service")
		res = isUnitPresent(mc, "test-name-replace-kernel-module.service")
		Expect(res).To(BeFalse())

		By("check the crio-wipe service")
		res = isUnitPresent(mc, "crio-wipe.service")
		Expect(res).To(BeFalse())

		By("replace-kernel-module.sh")
		res = isFilePresent(mc, "/usr/local/bin/replace-kernel-module.sh")
		Expect(res).To(BeFalse())

		By("pull-kernel-module-image.sh")
		res = isFilePresent(mc, "/usr/local/bin/pull-kernel-module-image.sh")
		Expect(res).To(BeFalse())

		By("wait-for-dispatcher.sh")
		res = isFilePresent(mc, "/usr/local/bin/wait-for-dispatcher.sh")
		Expect(res).To(BeFalse())

	})

	It("remove only current bmc", func() {
		mcfgAPI.RemoveDisruptionPolicies(mc, bmc, false)

		By("check the pull service")
		res := isUnitPresent(mc, "test-name-pull-kernel-module-image.service")
		Expect(res).To(BeFalse())

		By("check the replace service")
		res = isUnitPresent(mc, "test-name-replace-kernel-module.service")
		Expect(res).To(BeFalse())

		By("check the crio-wipe service")
		res = isUnitPresent(mc, "crio-wipe.service")
		Expect(res).To(BeTrue())

		By("replace-kernel-module.sh")
		res = isFilePresent(mc, "/usr/local/bin/replace-kernel-module.sh")
		Expect(res).To(BeTrue())

		By("pull-kernel-module-image.sh")
		res = isFilePresent(mc, "/usr/local/bin/pull-kernel-module-image.sh")
		Expect(res).To(BeTrue())

		By("wait-for-dispatcher.sh")
		res = isFilePresent(mc, "/usr/local/bin/wait-for-dispatcher.sh")
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
		name              = "name"
		mcpRef            = "mcpRef"
		kernelModuleName  = "testKernelModuleName"
		firmwareFilesPath = "/opt/lib/test/firmware"
		imageName         = "quay.io/project/repo:some-tag12"
		workerImage       = "some-worker-image"
	)

	// unit performs verification of the MC output vs the manually created
	// machineconfig-test.yaml. This yaml is created by manually running bas64
	// encoding on testdata/pull-image-test.sh and testdata/replace-kernel-module-test.sh
	// and setting the values into the testdata/machineconfig-test.yaml
	It("verify correct ignition output", func() {
		mcfgAPI := NewMCFG("")

		_, yamlRes, err := mcfgAPI.GenerateIgnition(imageName, kernelModuleName, firmwareFilesPath, workerImage, "test-mc",
			[]string{"it1", "it2"})

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

		_, yamlRes, err := mcfgAPI.GenerateIgnition(imageName, kernelModuleName, firmwareFilesPath, "", "test-mc",
			[]string{"it1", "it2"})

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

var _ = Describe("initramfs pool and MachineConfig", func() {
	var (
		mcfgAPI MCFG
		irm     *kmmv1beta1.InitramfsModule
	)

	BeforeEach(func() {
		mcfgAPI = NewMCFG("")
		irm = &kmmv1beta1.InitramfsModule{
			ObjectMeta: metav1.ObjectMeta{Name: "nic", Namespace: "ns", UID: types.UID("cr-uid")},
		}
	})

	It("sets the pool selector, maxUnavailable, and stamp annotations", func() {
		pool := &mcfgv1.MachineConfigPool{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{"keep": "yes"}},
		}

		mcfgAPI.UpdateInitramfsPool(pool, irm)

		Expect(pool.Annotations).To(Equal(map[string]string{
			"keep":                             "yes",
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
			InitramfsModuleUIDAnnotation:       string(irm.UID),
		}))
		Expect(pool.Spec.NodeSelector).To(Equal(&metav1.LabelSelector{
			MatchLabels: map[string]string{initramfsNodeRoleLabel: ""},
		}))
		Expect(pool.Spec.MachineConfigSelector).To(Equal(&metav1.LabelSelector{
			MatchExpressions: []metav1.LabelSelectorRequirement{{
				Key:      machineConfigRoleLabel,
				Operator: metav1.LabelSelectorOpIn,
				Values:   []string{"worker", InitramfsPoolName},
			}},
		}))
		Expect(pool.Spec.MaxUnavailable).To(Equal(ptrTo(intstr.FromInt(1))))
	})

	It("reports a stamp only when all three annotations match", func() {
		stamped := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
			InitramfsModuleUIDAnnotation:       string(irm.UID),
		}}}
		mismatched := stamped.DeepCopy()
		mismatched.Annotations[InitramfsModuleUIDAnnotation] = "other-uid"
		partial := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
		}}}

		Expect(mcfgAPI.StampedForInitramfsModule(stamped, irm)).To(BeTrue())
		Expect(mcfgAPI.StampedForInitramfsModule(mismatched, irm)).To(BeFalse())
		Expect(mcfgAPI.StampedForInitramfsModule(partial, irm)).To(BeFalse())
		Expect(mcfgAPI.StampedForInitramfsModule(&mcfgv1.MachineConfig{}, irm)).To(BeFalse())
	})

	It("sets the MachineConfig role label and stamp and leaves Ignition empty", func() {
		mc := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"keep": "yes"}}}
		kernels := map[string]KernelInspectLists{
			"6.1.0": {InTreeModules: []string{"mod"}},
		}

		Expect(mcfgAPI.UpdateInitramfsMachineConfig(mc, irm, kernels)).To(Succeed())

		Expect(mc.Labels).To(Equal(map[string]string{
			"keep":                 "yes",
			machineConfigRoleLabel: InitramfsPoolName,
		}))
		Expect(mc.Annotations).To(Equal(map[string]string{
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
			InitramfsModuleUIDAnnotation:       string(irm.UID),
		}))
		Expect(mc.Spec.Config.Raw).To(BeNil())
	})
})

func ptrTo[T any](v T) *T {
	return &v
}

func isUnitPresent(mc *apioperatorv1.MachineConfiguration, unitName string /*apioperatorv1.NodeDisruptionPolicySpecUnit*/) bool {
	expectedUnit := apioperatorv1.NodeDisruptionPolicySpecUnit{
		Name: apioperatorv1.NodeDisruptionPolicyServiceName(unitName),
		Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
			{
				Type: apioperatorv1.NoneSpecAction,
			},
		},
	}
	for _, unit := range mc.Spec.NodeDisruptionPolicy.Units {
		if cmp.Equal(unit, expectedUnit) {
			return true
		}
	}
	return false
}

func isFilePresent(mc *apioperatorv1.MachineConfiguration, filePath string /*apioperatorv1.NodeDisruptionPolicySpecFile*/) bool {
	expectedFile := apioperatorv1.NodeDisruptionPolicySpecFile{
		Path: filePath,
		Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
			{
				Type: apioperatorv1.NoneSpecAction,
			},
		},
	}
	for _, file := range mc.Spec.NodeDisruptionPolicy.Files {
		if cmp.Equal(file, expectedFile) {
			return true
		}
	}
	return false
}

func addUnitSpec(mc *apioperatorv1.MachineConfiguration, unitName string) {
	unitSpec := apioperatorv1.NodeDisruptionPolicySpecUnit{
		Name: apioperatorv1.NodeDisruptionPolicyServiceName(unitName),
		Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
			{
				Type: apioperatorv1.NoneSpecAction,
			},
		},
	}
	mc.Spec.NodeDisruptionPolicy.Units = append(mc.Spec.NodeDisruptionPolicy.Units, unitSpec)

}

func addFileSpec(mc *apioperatorv1.MachineConfiguration, filePath string) {
	fileSpec := apioperatorv1.NodeDisruptionPolicySpecFile{
		Path: filePath,
		Actions: []apioperatorv1.NodeDisruptionPolicySpecAction{
			{
				Type: apioperatorv1.NoneSpecAction,
			},
		},
	}
	mc.Spec.NodeDisruptionPolicy.Files = append(mc.Spec.NodeDisruptionPolicy.Files, fileSpec)
}
