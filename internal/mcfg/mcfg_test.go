package mcfg

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

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

	It("reports a stamp when namespace and name match", func() {
		stamped := &mcfgv1.MachineConfigPool{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
			InitramfsModuleUIDAnnotation:       string(irm.UID),
		}}}
		otherUID := stamped.DeepCopy()
		otherUID.Annotations[InitramfsModuleUIDAnnotation] = "other-uid"
		noUID := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
		}}}
		otherNamespace := noUID.DeepCopy()
		otherNamespace.Annotations[InitramfsModuleNamespaceAnnotation] = "other-ns"
		otherName := noUID.DeepCopy()
		otherName.Annotations[InitramfsModuleNameAnnotation] = "other-name"

		Expect(mcfgAPI.StampedForInitramfsModule(stamped, irm)).To(BeTrue())
		Expect(mcfgAPI.StampedForInitramfsModule(otherUID, irm)).To(BeTrue())
		Expect(mcfgAPI.StampedForInitramfsModule(noUID, irm)).To(BeTrue())
		Expect(mcfgAPI.StampedForInitramfsModule(otherNamespace, irm)).To(BeFalse())
		Expect(mcfgAPI.StampedForInitramfsModule(otherName, irm)).To(BeFalse())
		Expect(mcfgAPI.StampedForInitramfsModule(&mcfgv1.MachineConfig{}, irm)).To(BeFalse())
	})

	It("writes Ignition for one module and keeps the stamp and existing metadata", func() {
		irm.Spec = kmmv1beta1.InitramfsModuleSpec{
			ModuleNames:    []string{"nic"},
			ContainerImage: "reg.example/kmods:v1",
			ModulesPath:    "/usr/lib/modules",
			DirName:        "/opt/modules",
			Selector:       map[string]string{"node": "a"},
		}
		mc := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{
			Labels:      map[string]string{"keep": "yes"},
			Annotations: map[string]string{"keep": "yes"},
		}}
		kernels := map[string]KernelInspectLists{
			"6.1.0": {InTreeModules: []string{"mod"}},
		}

		Expect(mcfgAPI.UpdateInitramfsMachineConfig(mc, irm, kernels)).To(Succeed())

		Expect(mc.Labels).To(Equal(map[string]string{
			"keep":                 "yes",
			machineConfigRoleLabel: InitramfsPoolName,
		}))
		Expect(mc.Annotations).To(Equal(map[string]string{
			"keep":                             "yes",
			InitramfsModuleNamespaceAnnotation: irm.Namespace,
			InitramfsModuleNameAnnotation:      irm.Name,
		}))

		doc := decodeInitramfsIgnition(mc.Spec.Config.Raw)
		Expect(doc.Ignition.Version).To(Equal("3.2.0"))
		Expect(doc.Systemd.Units).To(HaveLen(1))
		unit := doc.Systemd.Units[0]
		Expect(unit.Name).To(Equal("kmm-initramfs.service"))
		Expect(unit.Enabled).To(BeTrue())
		Expect(unit.Contents).To(ContainSubstring("ExecStart=/usr/local/bin/initramfs.sh"))
		Expect(unit.Contents).To(ContainSubstring(`Environment="ROLLBACK=false"`))
		Expect(unit.Contents).To(ContainSubstring(`Environment="CONTAINER_IMAGE=reg.example/kmods:v1"`))
		Expect(unit.Contents).To(ContainSubstring(`Environment="MODULES_PATH=/usr/lib/modules"`))
		Expect(unit.Contents).To(ContainSubstring(`Environment="DIR_NAME=/opt/modules"`))
		hash := specHashFromUnit(unit.Contents)
		Expect(hash).To(MatchRegexp(`^[0-9a-f]{64}$`))

		script := ignitionFile(doc, "/usr/local/bin/initramfs.sh")
		expectIgnitionFileMeta(script)
		onDisk, err := os.ReadFile("scripts/initramfs.sh")
		Expect(err).NotTo(HaveOccurred())
		Expect(ignitionFileBytes(script)).To(Equal(onDisk))

		hook := ignitionFile(doc, "/usr/local/bin/kmm-pre-udev.sh")
		expectIgnitionFileMeta(hook)
		Expect(string(ignitionFileBytes(hook))).To(ContainSubstring(`modprobe -d "/opt/modules" "nic"`))
	})

	It("uses /opt in the pre-udev hook when DirName is empty", func() {
		irm.Spec = kmmv1beta1.InitramfsModuleSpec{
			ModuleNames:    []string{"nic"},
			ContainerImage: "reg.example/kmods:v1",
		}
		mc := &mcfgv1.MachineConfig{}

		Expect(mcfgAPI.UpdateInitramfsMachineConfig(mc, irm, nil)).To(Succeed())

		doc := decodeInitramfsIgnition(mc.Spec.Config.Raw)
		Expect(doc.Systemd.Units[0].Contents).To(ContainSubstring(`Environment="DIR_NAME="`))
		hook := string(ignitionFileBytes(ignitionFile(doc, "/usr/local/bin/kmm-pre-udev.sh")))
		Expect(hook).To(ContainSubstring(`modprobe -d "/opt" "nic"`))
		Expect(mc.Labels).To(HaveKeyWithValue(machineConfigRoleLabel, InitramfsPoolName))
	})

	It("renders ROLLBACK=true when rollback is set", func() {
		irm.Spec = kmmv1beta1.InitramfsModuleSpec{
			ModuleNames:    []string{"nic"},
			ContainerImage: "reg.example/kmods:v1",
			Rollback:       true,
		}
		mc := &mcfgv1.MachineConfig{}

		Expect(mcfgAPI.UpdateInitramfsMachineConfig(mc, irm, nil)).To(Succeed())

		doc := decodeInitramfsIgnition(mc.Spec.Config.Raw)
		Expect(doc.Systemd.Units[0].Contents).To(ContainSubstring(`Environment="ROLLBACK=true"`))
	})

	It("keeps Ignition stable across calls and ignores selector and kernel lists", func() {
		irm.Spec = kmmv1beta1.InitramfsModuleSpec{
			ModuleNames:    []string{"nic"},
			ContainerImage: "reg.example/kmods:v1",
			ModulesPath:    "/usr/lib/modules",
			DirName:        "/opt/modules",
			Selector:       map[string]string{"node": "a"},
		}
		first := &mcfgv1.MachineConfig{}
		kernels := map[string]KernelInspectLists{
			"6.1.0": {InTreeModules: []string{"mod"}},
		}
		Expect(mcfgAPI.UpdateInitramfsMachineConfig(first, irm, kernels)).To(Succeed())
		Expect(mcfgAPI.UpdateInitramfsMachineConfig(first, irm, kernels)).To(Succeed())

		otherSelector := irm.DeepCopy()
		otherSelector.Spec.Selector = map[string]string{"node": "b"}
		second := &mcfgv1.MachineConfig{}
		Expect(mcfgAPI.UpdateInitramfsMachineConfig(second, otherSelector, nil)).To(Succeed())
		Expect(second.Spec.Config.Raw).To(Equal(first.Spec.Config.Raw))

		changedImage := irm.DeepCopy()
		changedImage.Spec.ContainerImage = "reg.example/kmods:v2"
		third := &mcfgv1.MachineConfig{}
		Expect(mcfgAPI.UpdateInitramfsMachineConfig(third, changedImage, kernels)).To(Succeed())
		firstHash := specHashFromUnit(decodeInitramfsIgnition(first.Spec.Config.Raw).Systemd.Units[0].Contents)
		thirdHash := specHashFromUnit(decodeInitramfsIgnition(third.Spec.Config.Raw).Systemd.Units[0].Contents)
		Expect(thirdHash).NotTo(Equal(firstHash))
	})

	It("leaves the MachineConfig unchanged when moduleNames is not a single entry", func() {
		for _, names := range [][]string{nil, {"nic", "other"}} {
			irm.Spec.ModuleNames = names
			mc := &mcfgv1.MachineConfig{ObjectMeta: metav1.ObjectMeta{
				Labels:      map[string]string{"keep": "yes"},
				Annotations: map[string]string{"keep": "yes"},
			}}
			original := mc.DeepCopy()

			Expect(mcfgAPI.UpdateInitramfsMachineConfig(mc, irm, nil)).NotTo(Succeed())
			Expect(mc).To(Equal(original))
		}
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

type initramfsIgnition struct {
	Ignition struct {
		Version string `json:"version"`
	} `json:"ignition"`
	Systemd struct {
		Units []struct {
			Contents string `json:"contents"`
			Enabled  bool   `json:"enabled"`
			Name     string `json:"name"`
		} `json:"units"`
	} `json:"systemd"`
	Storage struct {
		Files []initramfsIgnitionFile `json:"files"`
	} `json:"storage"`
}

type initramfsIgnitionFile struct {
	Contents struct {
		Source string `json:"source"`
	} `json:"contents"`
	Mode      int    `json:"mode"`
	Overwrite bool   `json:"overwrite"`
	Path      string `json:"path"`
	User      struct {
		Name string `json:"name"`
	} `json:"user"`
}

func decodeInitramfsIgnition(raw []byte) initramfsIgnition {
	var doc initramfsIgnition
	Expect(json.Unmarshal(raw, &doc)).To(Succeed())
	return doc
}

func ignitionFile(doc initramfsIgnition, path string) initramfsIgnitionFile {
	for _, file := range doc.Storage.Files {
		if file.Path == path {
			return file
		}
	}
	Fail("missing ignition file " + path)
	return initramfsIgnitionFile{}
}

func expectIgnitionFileMeta(file initramfsIgnitionFile) {
	Expect(file.Mode).To(Equal(493))
	Expect(file.Overwrite).To(BeTrue())
	Expect(file.User.Name).To(Equal("root"))
}

func ignitionFileBytes(file initramfsIgnitionFile) []byte {
	const prefix = "data:text/plain;base64,"
	Expect(file.Contents.Source).To(HavePrefix(prefix))
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(file.Contents.Source, prefix))
	Expect(err).NotTo(HaveOccurred())
	return decoded
}

func specHashFromUnit(contents string) string {
	const prefix = `Environment="SPEC_HASH=`
	start := strings.Index(contents, prefix)
	Expect(start).NotTo(Equal(-1))
	rest := contents[start+len(prefix):]
	end := strings.Index(rest, `"`)
	Expect(end).To(Equal(64))
	return rest[:end]
}
