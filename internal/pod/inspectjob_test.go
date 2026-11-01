package pod

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	kmmv1beta1 "github.com/rh-ecosystem-edge/kernel-module-management/api/v1beta1"
	batchv1 "k8s.io/api/batch/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func inspectIRM() *kmmv1beta1.InitramfsModule {
	return &kmmv1beta1.InitramfsModule{
		ObjectMeta: metav1.ObjectMeta{Name: "driver", Namespace: "drivers", UID: "irm-uid"},
		Spec:       kmmv1beta1.InitramfsModuleSpec{ModuleNames: []string{"mymod"}},
	}
}

func inspectInput() InspectJobInput {
	return InspectJobInput{KernelVersion: "6.1.0", Image: "registry/driver:v1-6.1.0"}
}

func configureInspectJob(manager InspectJobManager, irm *kmmv1beta1.InitramfsModule, input InspectJobInput) (*batchv1.Job, error) {
	job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "controller-chosen-job", Namespace: irm.Namespace}}
	err := manager.ConfigureJob(job, irm, input)
	return job, err
}

var _ = Describe("InspectJobManager_ConfigureJob new Job", func() {
	var manager InspectJobManager
	BeforeEach(func() { manager = NewInspectJobManager(nil, scheme, "registry/parser:v1") })

	It("builds a namespaced, owned extract/parse Job with persistent status", func() {
		irm := inspectIRM()
		job, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		Expect(job.Namespace).To(Equal(irm.Namespace))
		Expect(job.Name).To(Equal("controller-chosen-job"))
		Expect(job.Labels).To(BeEmpty())
		Expect(job.Annotations).To(BeEmpty())
		Expect(job.Spec.Template.Labels).To(BeEmpty())
		owner := metav1.GetControllerOf(job)
		Expect(owner).NotTo(BeNil())
		Expect(owner.APIVersion).To(Equal(kmmv1beta1.GroupVersion.String()))
		Expect(owner.Kind).To(Equal("InitramfsModule"))
		Expect(owner.Name).To(Equal(irm.Name))
		Expect(owner.UID).To(Equal(irm.UID))
		Expect(job.Spec.BackoffLimit).To(HaveValue(Equal(int32(3))))
		Expect(job.Spec.TTLSecondsAfterFinished).To(BeNil())
		pod := job.Spec.Template.Spec
		Expect(pod.RestartPolicy).To(Equal(v1.RestartPolicyNever))
		Expect(pod.AutomountServiceAccountToken).To(HaveValue(BeFalse()))
		Expect(pod.Volumes).To(ConsistOf(v1.Volume{Name: "tmp", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}))
		Expect(pod.InitContainers).To(HaveLen(1))
		Expect(pod.Containers).To(HaveLen(1))
		extract, parse := pod.InitContainers[0], pod.Containers[0]
		Expect(extract.Name).To(Equal("extract"))
		Expect(extract.Image).To(Equal(inspectInput().Image))
		Expect(extract.Args).To(Equal([]string{"/opt/lib/modules/6.1.0", "/tmp/opt/lib/modules/6.1.0", "/dev/termination-log"}))
		Expect(parse.Name).To(Equal("parse"))
		Expect(parse.Image).To(Equal("registry/parser:v1"))
		Expect(parse.Command).To(Equal([]string{"/usr/local/bin/kmod-image-parse"}))
		Expect(parse.Args).To(Equal([]string{"--modules-root", "/tmp/opt/lib/modules/6.1.0", "--modules-names", "mymod"}))
		for _, container := range []v1.Container{extract, parse} {
			Expect(container.ImagePullPolicy).To(Equal(v1.PullAlways))
			Expect(container.TerminationMessagePolicy).To(Equal(v1.TerminationMessageReadFile))
			Expect(container.TerminationMessagePath).To(Equal("/dev/termination-log"))
		}
		Expect(extract.VolumeMounts).To(ConsistOf(v1.VolumeMount{Name: "tmp", MountPath: "/tmp"}))
		Expect(parse.VolumeMounts).To(ConsistOf(v1.VolumeMount{Name: "tmp", MountPath: "/tmp", ReadOnly: true}))
		Expect(pod.ImagePullSecrets).To(BeEmpty())
	})

	It("uses a custom root, pull secret and repeatable parser flags", func() {
		irm := inspectIRM()
		irm.Spec.DirName = "/some modules; not shell code"
		irm.Spec.ImageRepoSecret = &v1.LocalObjectReference{Name: "pull-secret"}
		irm.Spec.ModuleNames = []string{"zmod", "amod"}
		irm.Spec.Omit = []string{"zomit", "aomit"}
		job, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		Expect(job.Spec.Template.Spec.InitContainers[0].Args[0]).To(Equal("/some modules; not shell code/lib/modules/6.1.0"))
		Expect(job.Spec.Template.Spec.ImagePullSecrets).To(Equal([]v1.LocalObjectReference{{Name: "pull-secret"}}))
		Expect(job.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{
			"--modules-root", "/tmp/some modules; not shell code/lib/modules/6.1.0", "--modules-names", "zmod", "--modules-names", "amod",
			"--omit", "zomit", "--omit", "aomit",
		}))
	})

	DescribeTable("shares the worker tmp volume and module-tree layout", func(dirName, source, stagedRoot string) {
		irm := inspectIRM()
		irm.Spec.DirName = dirName
		job, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		pod := job.Spec.Template.Spec
		extract, parse := pod.InitContainers[0], pod.Containers[0]
		Expect(pod.Volumes).To(ConsistOf(v1.Volume{Name: "tmp", VolumeSource: v1.VolumeSource{EmptyDir: &v1.EmptyDirVolumeSource{}}}))
		Expect(extract.VolumeMounts).To(ConsistOf(v1.VolumeMount{Name: "tmp", MountPath: "/tmp"}))
		Expect(extract.Args).To(Equal([]string{source, stagedRoot, "/dev/termination-log"}))
		Expect(parse.VolumeMounts).To(ConsistOf(v1.VolumeMount{Name: "tmp", MountPath: "/tmp", ReadOnly: true}))
		Expect(parse.Args[:2]).To(Equal([]string{"--modules-root", stagedRoot}))
	},
		Entry("custom root", "/modules", "/modules/lib/modules/6.1.0", "/tmp/modules/lib/modules/6.1.0"),
		Entry("relative root", "../opt", "/opt/lib/modules/6.1.0", "/tmp/opt/lib/modules/6.1.0"),
		Entry("custom root with dot components", "/unused/../custom", "/custom/lib/modules/6.1.0", "/tmp/custom/lib/modules/6.1.0"),
	)

	It("derives the source from dirName and kernel", func() {
		irm := inspectIRM()
		irm.Spec.DirName = "/custom"
		job, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		Expect(job.Spec.Template.Spec.InitContainers[0].Args[0]).To(Equal("/custom/lib/modules/6.1.0"))
	})

	It("normalizes the kernel without changing the CR or argument order", func() {
		irm := inspectIRM()
		irm.Spec.ModuleNames = []string{"zmod", "amod"}
		irm.Spec.Omit = []string{"zomit", "aomit"}
		original := irm.DeepCopy()
		input := inspectInput()
		input.KernelVersion = "6.1.0+debug"
		first, err := configureInspectJob(manager, irm, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(irm).To(Equal(original))
		Expect(first.Spec.Template.Spec.InitContainers[0].Args[0]).To(Equal("/opt/lib/modules/6.1.0-debug"))
		Expect(first.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{
			"--modules-root", "/tmp/opt/lib/modules/6.1.0-debug", "--modules-names", "zmod", "--modules-names", "amod",
			"--omit", "zomit", "--omit", "aomit",
		}))
		input.KernelVersion = "6.1.0-debug"
		second, err := configureInspectJob(manager, irm, input)
		Expect(err).NotTo(HaveOccurred())
		Expect(first).To(Equal(second))
	})

	It("treats empty omit and explicit defaults as the same inputs", func() {
		irm := inspectIRM()
		first, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		irm.Spec.Omit = []string{}
		irm.Spec.DirName = "/opt"
		irm.Spec.Selector = map[string]string{"unrelated": "selector"}
		second, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		Expect(second).To(Equal(first))
	})

	DescribeTable("requires initial inspection inputs", func(input InspectJobInput) {
		_, err := configureInspectJob(manager, inspectIRM(), input)
		Expect(err).To(HaveOccurred())
	},
		Entry("kernel", InspectJobInput{Image: "registry/driver:v1"}),
		Entry("image", InspectJobInput{KernelVersion: "6.1.0"}),
	)

	It("propagates controller-reference assignment errors", func() {
		irm := inspectIRM()
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "controller-chosen-job", Namespace: irm.Namespace}}
		Expect(controllerutil.SetControllerReference(irm, job, scheme)).To(Succeed())
		job.OwnerReferences[0].Name = "another-owner"
		err := manager.ConfigureJob(job, irm, inspectInput())
		var ownerError *controllerutil.AlreadyOwnedError
		Expect(errors.As(err, &ownerError)).To(BeTrue())
	})

	It("preserves caller metadata without aliasing CR credentials", func() {
		irm := inspectIRM()
		irm.Spec.ImageRepoSecret = &v1.LocalObjectReference{Name: "pull-secret"}
		job := &batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "custom-job-name", Namespace: irm.Namespace,
			Labels: map[string]string{"unrelated": "label"}, Annotations: map[string]string{"unrelated": "annotation"}}}
		Expect(manager.ConfigureJob(job, irm, inspectInput())).To(Succeed())
		Expect(job.Name).To(Equal("custom-job-name"))
		Expect(job.Namespace).To(Equal(irm.Namespace))
		Expect(job.Labels["unrelated"]).To(Equal("label"))
		Expect(job.Annotations["unrelated"]).To(Equal("annotation"))
		job.Spec.Template.Spec.ImagePullSecrets[0].Name = "changed"
		Expect(irm.Spec.ImageRepoSecret.Name).To(Equal("pull-secret"))
	})

	It("copies metadata files and dangling host links without interpreting shell characters", func() {
		root := GinkgoT().TempDir()
		irm := inspectIRM()
		irm.Spec.DirName = filepath.Join(root, "source ' $(touch injected)")
		src := filepath.Join(irm.Spec.DirName, "lib", "modules", "6.1.0")
		dst := filepath.Join(root, "destination", "opt", "lib", "modules", "6.1.0")
		Expect(os.MkdirAll(src, 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(src, "modules.dep"), []byte("mymod.ko: host/kernel/a.ko\n"), 0o644)).To(Succeed())
		Expect(os.Symlink("/lib/modules/6.1.0", filepath.Join(src, "host"))).To(Succeed())
		job, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		container := job.Spec.Template.Spec.InitContainers[0]
		Expect(container.Args[0]).To(Equal(src))
		args := append([]string{}, container.Args...)
		args[1], args[2] = dst, filepath.Join(root, "termination-log")
		command := append(append([]string{}, container.Command...), args...)
		cmd := exec.Command(command[0], command[1:]...)
		cmd.Dir = root
		output, err := cmd.CombinedOutput()
		Expect(err).NotTo(HaveOccurred(), string(output))
		data, err := os.ReadFile(filepath.Join(dst, "modules.dep"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("mymod.ko: host/kernel/a.ko\n"))
		target, err := os.Readlink(filepath.Join(dst, "host"))
		Expect(err).NotTo(HaveOccurred())
		Expect(target).To(Equal("/lib/modules/6.1.0"))
		_, err = os.Stat(filepath.Join(root, "injected"))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})

	It("writes the exact missing_modules.dep error and exits one before copying", func() {
		root := GinkgoT().TempDir()
		irm := inspectIRM()
		irm.Spec.DirName = filepath.Join(root, "source")
		src := filepath.Join(irm.Spec.DirName, "lib", "modules", "6.1.0")
		dst, log := filepath.Join(root, "destination"), filepath.Join(root, "termination-log")
		Expect(os.MkdirAll(src, 0o755)).To(Succeed())
		job, err := configureInspectJob(manager, irm, inspectInput())
		Expect(err).NotTo(HaveOccurred())
		container := job.Spec.Template.Spec.InitContainers[0]
		args := append([]string{}, container.Args...)
		args[1], args[2] = dst, log
		command := append(append([]string{}, container.Command...), args...)
		err = exec.Command(command[0], command[1:]...).Run()
		var exitErr *exec.ExitError
		Expect(errors.As(err, &exitErr)).To(BeTrue())
		Expect(exitErr.ExitCode()).To(Equal(1))
		data, err := os.ReadFile(log)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal(`{"error":"missing_modules.dep"}`))
		_, err = os.Stat(dst)
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})
