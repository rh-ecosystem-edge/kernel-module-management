package constants

const (
	OCPBuilderServiceAccountName = "builder"
	DTKImageStreamNamespace      = "openshift"

	ModuleNameLabel        = "kmm.node.kubernetes.io/module.name"
	ModuleNamespaceLabel   = "kmm.node.kubernetes.io/module.namespace"
	NodeLabelerFinalizer   = "kmm.node.kubernetes.io/node-labeler"
	TargetKernelTarget     = "kmm.node.kubernetes.io/target-kernel"
	ResourceType           = "kmm.openshift.io/build.type"
	ResourceHashAnnotation = "kmm.node.kubernetes.io/last-hash"
	NamespaceLabelKey      = "kmm.node.k8s.io/contains-modules"

	WorkerPodVersionLabelPrefix   = "beta.kmm.node.kubernetes.io/version-worker-pod"
	SchedulePodVersionLabelPrefix = "beta.kmm.node.kubernetes.io/version-schedule-pod"
	ModuleVersionLabelPrefix      = "kmm.node.kubernetes.io/version-module"

	GCDelayFinalizer  = "kmm.node.kubernetes.io/gc-delay"
	ModuleFinalizer   = "kmm.node.kubernetes.io/module-finalizer"
	JobEventFinalizer = "kmm.node.kubernetes.io/job-event-finalizer"
	BMCFinalizer      = "kmm.node.kubernetes.io/bmc-finalizer"

	ManagedClusterModuleNameLabel  = "kmm.node.kubernetes.io/managedclustermodule.name"
	KernelVersionsClusterClaimName = "kernel-versions.kmm.node.kubernetes.io"
	DockerfileCMKey                = "dockerfile"
	PublicSignDataKey              = "cert"
	PrivateSignDataKey             = "key"

	DaemonSetRole              = "kmm.node.kubernetes.io/role"
	DevicePluginRoleLabelValue = "device-plugin"
	DRARoleLabelValue          = "dra"

	OperatorNamespaceEnvVar = "OPERATOR_NAMESPACE"

	MinOCPMajorForDRA = 4
	MinOCPMinorForDRA = 21
)
