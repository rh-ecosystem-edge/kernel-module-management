package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	runtimemetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// When adding metric names, see https://prometheus.io/docs/practices/naming/#metric-names
const (
	kmmModulesQuery         = "kmm_module_num"
	kmmInClusterBuildQuery  = "kmm_in_cluster_build_num"
	kmmInClusterSignQuery   = "kmm_in_cluster_sign_num"
	kmmDevicePluginQuery    = "kmm_device_plugin_num"
	kmmPreflightQuery       = "kmm_preflight_num"
	kmmModprobeArgsQuery    = "kmm_modprobe_args"
	kmmModprobeRawArgsQuery = "kmm_modprobe_raw_args"

	acceleratorGpuUtilizationQuery     = "accelerator_gpu_utilization"
	acceleratorMemoryUsedBytesQuery    = "accelerator_memory_used_bytes"
	acceleratorMemoryTotalBytesQuery   = "accelerator_memory_total_bytes"
	acceleratorPowerUsageWattsQuery    = "accelerator_power_usage_watts"
	acceleratorTemperatureCelciusQuery = "accelerator_temperature_celcius"
	acceleratorSMClockHertzQuery       = "accelerator_sm_clock_hertz"
	acceleratorMemoryClockHertzQuery   = "accelerator_memory_clock_hertz"
)

//go:generate mockgen -source=metrics.go -package=metrics -destination=mock_metrics_api.go

// Metrics is an interface representing a prometheus client for the Kernel Module Management Operator
type Metrics interface {
	Register()
	SetKMMModulesNum(value int)
	SetKMMInClusterBuildNum(value int)
	SetKMMInClusterSignNum(value int)
	SetKMMDevicePluginNum(value int)
	SetKMMPreflightsNum(value int)
	SetKMMModprobeArgs(modName, namespace, modprobeArgs string)
	SetKMMModprobeRawArgs(modName, namespace, modprobeArgs string)

	//SetGpuUtilization(value int, vendorId string)
	//SetGpuMemoryUtilization(value int, vendorId string)

	SetAcceleratorGpuUtilization(value int, vendorId string)
	SetAcceleratorMemoryUsedBytes(value int, vendorId string)
	SetAcceleratorMemoryTotalBytes(value int, vendorId string)
	SetAcceleratorPowerUsageWatts(value int, vendorId string)
	SetAcceleratorTemperatureCelcius(value int, vendorId string)
	SetAcceleratorSMClockHertz(value int, vendorId string)
	SetAcceleratorMemoryClockHertz(value int, vendorId string)
}

type metrics struct {
	kmmModuleResourcesNum       prometheus.Gauge
	kmmInClusterBuildNum        prometheus.Gauge
	kmmInClusterSignNum         prometheus.Gauge
	kmmDevicePluginResourcesNum prometheus.Gauge
	kmmPreflightResourceNum     prometheus.Gauge
	kmmModprobeArgs             *prometheus.GaugeVec
	kmmModprobeRawArgs          *prometheus.GaugeVec
	gpuUtilization              *prometheus.GaugeVec
	gpuMemoryUtilization        *prometheus.GaugeVec
	accGPUUtilization           *prometheus.GaugeVec
	accMemoryUsedBytes          *prometheus.GaugeVec
	accMemoryTotalBytes         *prometheus.GaugeVec
	accPowerUsageWatts          *prometheus.GaugeVec
	accTemperatureCelcius       *prometheus.GaugeVec
	accSMClockHertz             *prometheus.GaugeVec
	accMemoryClockHertz         *prometheus.GaugeVec
}

func New() Metrics {

	kmmModuleResourcesNum := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: kmmModulesQuery,
			Help: "Number of existing KMMO Modules",
		},
	)

	kmmInClusterBuildNum := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: kmmInClusterBuildQuery,
			Help: "Number of existing KMMO Modules with in-cluster Build defined",
		},
	)

	kmmInClusterSignNum := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: kmmInClusterSignQuery,
			Help: "Number of existing KMMO Modules with in-cluster Sign defined",
		},
	)

	kmmDevicePluginResourcesNum := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: kmmDevicePluginQuery,
			Help: "Number of existing KMMO Modules with DevicePlugin defined",
		},
	)

	kmmPreflightResourceNum := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: kmmPreflightQuery,
			Help: "Number of existing KMMO Preflights",
		},
	)

	kmmModprobeArgs := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: kmmModprobeArgsQuery,
			Help: "for a given kernel version, describe which modprobe args used (if at all)",
		},
		[]string{"name", "namespace", "modprobeArgs"},
	)

	kmmModprobeRawArgs := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: kmmModprobeRawArgsQuery,
			Help: "for a given kernel version, describe which modprobe raw args used (if at all)",
		},
		[]string{"name", "namespace", "modprobeRawArgs"},
	)

	accGPUUtilization := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorGpuUtilizationQuery,
			Help: "accelerator gpu utilization",
		},
		[]string{"vendor_id"},
	)
	accMemoryUsedBytes := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorMemoryUsedBytesQuery,
			Help: "accelerator memory used bytes",
		},
		[]string{"vendor_id"},
	)

	accMemoryTotalBytes := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorMemoryTotalBytesQuery,
			Help: "accelerator memory total bytes",
		},
		[]string{"vendor_id"},
	)

	accPowerUsageWatts := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorPowerUsageWattsQuery,
			Help: "accelerator power usage in watts",
		},
		[]string{"vendor_id"},
	)

	accTemperatureCelcius := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorTemperatureCelciusQuery,
			Help: "accelerator temperature in celcius",
		},
		[]string{"vendor_id"},
	)

	accSMClockHertz := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorSMClockHertzQuery,
			Help: "accelerator sm clock in hertz",
		},
		[]string{"vendor_id"},
	)

	accMemoryClockHertz := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: acceleratorMemoryClockHertzQuery,
			Help: "accelerator memory clock in hertz",
		},
		[]string{"vendor_id"},
	)

	return &metrics{
		kmmModuleResourcesNum:       kmmModuleResourcesNum,
		kmmInClusterBuildNum:        kmmInClusterBuildNum,
		kmmInClusterSignNum:         kmmInClusterSignNum,
		kmmDevicePluginResourcesNum: kmmDevicePluginResourcesNum,
		kmmPreflightResourceNum:     kmmPreflightResourceNum,
		kmmModprobeArgs:             kmmModprobeArgs,
		kmmModprobeRawArgs:          kmmModprobeRawArgs,
		accGPUUtilization:           accGPUUtilization,
		accMemoryUsedBytes:          accMemoryUsedBytes,
		accMemoryTotalBytes:         accMemoryTotalBytes,
		accPowerUsageWatts:          accPowerUsageWatts,
		accTemperatureCelcius:       accTemperatureCelcius,
		accSMClockHertz:             accSMClockHertz,
		accMemoryClockHertz:         accMemoryClockHertz,
	}
}

func (m *metrics) Register() {
	runtimemetrics.Registry.MustRegister(
		m.kmmModuleResourcesNum,
		m.kmmInClusterBuildNum,
		m.kmmInClusterSignNum,
		m.kmmDevicePluginResourcesNum,
		m.kmmPreflightResourceNum,
		m.kmmModprobeArgs,
		m.accGPUUtilization,
		m.accMemoryUsedBytes,
		m.accMemoryTotalBytes,
		m.accPowerUsageWatts,
		m.accTemperatureCelcius,
		m.accSMClockHertz,
		m.accMemoryClockHertz,
	)
}

func (m *metrics) SetKMMModulesNum(value int) {
	m.kmmModuleResourcesNum.Set(float64(value))
}

func (m *metrics) SetKMMInClusterBuildNum(value int) {
	m.kmmInClusterBuildNum.Set(float64(value))
}

func (m *metrics) SetKMMInClusterSignNum(value int) {
	m.kmmInClusterSignNum.Set(float64(value))
}

func (m *metrics) SetKMMDevicePluginNum(value int) {
	m.kmmDevicePluginResourcesNum.Set(float64(value))
}

func (m *metrics) SetKMMPreflightsNum(value int) {
	m.kmmPreflightResourceNum.Set(float64(value))
}

func (m *metrics) SetKMMModprobeArgs(modName, namespace, modprobeArgs string) {
	m.kmmModprobeArgs.WithLabelValues(modName, namespace, modprobeArgs).Set(float64(1))
}

func (m *metrics) SetKMMModprobeRawArgs(modName, namespace, modprobeRawArgs string) {
	m.kmmModprobeRawArgs.WithLabelValues(modName, namespace, modprobeRawArgs).Set(float64(1))
}

/*
func (m *metrics) SetGpuUtilization(value int, vendorId string) {
	m.gpuUtilization.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetGpuMemoryUtilization(value int, vendorId string) {
	m.gpuMemoryUtilization.WithLabelValues(vendorId).Set(float64(value))
}
*/

func (m *metrics) SetAcceleratorGpuUtilization(value int, vendorId string) {
	m.accGPUUtilization.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetAcceleratorMemoryUsedBytes(value int, vendorId string) {
	m.accMemoryUsedBytes.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetAcceleratorMemoryTotalBytes(value int, vendorId string) {
	m.accMemoryTotalBytes.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetAcceleratorPowerUsageWatts(value int, vendorId string) {
	m.accPowerUsageWatts.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetAcceleratorTemperatureCelcius(value int, vendorId string) {
	m.accTemperatureCelcius.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetAcceleratorSMClockHertz(value int, vendorId string) {
	m.accSMClockHertz.WithLabelValues(vendorId).Set(float64(value))
}

func (m *metrics) SetAcceleratorMemoryClockHertz(value int, vendorId string) {
	m.accMemoryClockHertz.WithLabelValues(vendorId).Set(float64(value))
}
