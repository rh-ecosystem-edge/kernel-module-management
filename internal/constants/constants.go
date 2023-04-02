package constants

const (
	OCPBuilderServiceAccountName = "builder"
	DTKImageStreamNamespace      = "openshift"
	ModuleNameLabel              = "kmm.node.kubernetes.io/module.name"
	NodeLabelerFinalizer         = "kmm.node.kubernetes.io/node-labeler"
	TargetKernelTarget           = "kmm.node.kubernetes.io/target-kernel"
	DaemonSetRole                = "kmm.node.kubernetes.io/role"
	KernelLabel                  = "kmm.node.kubernetes.io/kernel-version.full"

	ModuleLoaderVersionLabelPrefix = "beta.kmm.node.kubernetes.io/version-module-loader"
	DevicePluginVersionLabelPrefix = "beta.kmm.node.kubernetes.io/version-device-plugin"
	ModuleVersionLabelPrefix       = "kmm.node.kubernetes.io/version-module"

	ManagedClusterModuleNameLabel  = "kmm.node.kubernetes.io/managedclustermodule.name"
	KernelVersionsClusterClaimName = "kernel-versions.kmm.node.kubernetes.io"
	DockerfileCMKey                = "dockerfile"
	PublicSignDataKey              = "cert"
	PrivateSignDataKey             = "key"

	ModuleLoaderRoleLabelValue = "module-loader"
	DevicePluginRoleLabelValue = "device-plugin"

	OperatorNamespaceEnvVar = "OPERATOR_NAMESPACE"
)
