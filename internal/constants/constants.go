package constants

const (
	OCPBuilderServiceAccountName = "builder"
	DTKImageStreamNamespace      = "openshift"
	ModuleNameLabel              = "kmm.node.kubernetes.io/module.name"
	NodeLabelerFinalizer         = "kmm.node.kubernetes.io/node-labeler"
	TargetKernelTarget           = "kmm.node.kubernetes.io/target-kernel"
	JobType                      = "kmm.node.kubernetes.io/job-type"
	JobHashAnnotation            = "kmm.node.kubernetes.io/last-hash"
	KernelLabel                  = "kmm.node.kubernetes.io/kernel-version.full"

	ManagedClusterModuleNameLabel = "kmm.node.kubernetes.io/managedclustermodule.name"
	DockerfileCMKey               = "dockerfile"
	PublicSignDataKey             = "cert"
	PrivateSignDataKey            = "key"
)
