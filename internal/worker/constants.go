package worker

const (
	FlagFirmwareClassPath = "set-firmware-class-path"
	FlagFirmwareMountPath = "set-firmware-mount-path"

	FirmwareClassPathLocation = "/sys/module/firmware_class/parameters/path"
	ImagesDir                 = "/var/run/kmm/images"
	PullSecretsDir            = "/var/run/kmm/pull-secrets"
	GlobalPullSecretPath      = "/var/lib/kubelet/config.json"
	FirmwareMountPath         = "/var/lib/firmware"
)
