package config

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

var _ = Describe("ParseFile", func() {
	It("should return an error if the file does not exist", func() {
		_, err := ParseFile("/non/existent/path")
		Expect(err).To(HaveOccurred())
	})

	It("should parse the file correctly", func() {
		expected := &Config{
			HealthProbeBindAddress: ":8081",
			Job: Job{
				GCDelay: time.Hour,
			},
			LeaderElection: LeaderElection{
				Enabled:    true,
				ResourceID: "some-resource-id",
			},
			Webhook: Webhook{
				DisableHTTP2: true,
				Port:         9443,
			},
			Metrics: Metrics{
				BindAddress:      "0.0.0.0:8443",
				DisableHTTP2:     true,
				EnableAuthnAuthz: true,
				SecureServing:    true,
			},
			Worker: Worker{
				RunAsUser:        ptr.To[int64](1234),
				SELinuxType:      "mySELinuxType",
				FirmwareHostPath: ptr.To("/some/path"),
			},
		}

		Expect(
			ParseFile("testdata/config.yaml"),
		).To(
			Equal(expected),
		)
	})
})

var _ = Describe("Config_ManagerOptions", func() {
	It("should work as expected", func() {
		const (
			healthProbeBindAddress   = ":8081"
			leaderElectionEnabled    = true
			leaderElectionResourceID = "some-resource-id"
			metricsBindAddress       = "127.0.0.1:8080"
			webhookPort              = 1234
		)

		cfg := Config{
			HealthProbeBindAddress: healthProbeBindAddress,
			Metrics:                Metrics{BindAddress: metricsBindAddress},
			LeaderElection: LeaderElection{
				Enabled:    leaderElectionEnabled,
				ResourceID: leaderElectionResourceID,
			},
			Webhook: Webhook{Port: webhookPort},
			Worker:  Worker{},
		}

		opts := cfg.ManagerOptions(GinkgoLogr)

		Expect(opts.HealthProbeBindAddress).To(Equal(healthProbeBindAddress))
		Expect(opts.Metrics.BindAddress).To(Equal(metricsBindAddress))
		Expect(opts.LeaderElection).To(Equal(leaderElectionEnabled))
		Expect(opts.LeaderElectionID).To(Equal(leaderElectionResourceID))
		Expect(opts.WebhookServer.(*webhook.DefaultServer).Options.Port).To(Equal(webhookPort))
	})

	DescribeTable(
		"should enable or disable HTTP/2 in the webhook server",
		func(disableHTTP2 bool, expectedTLSOptsLen int) {
			cfg := Config{
				Webhook: Webhook{DisableHTTP2: disableHTTP2},
			}

			Expect(cfg.ManagerOptions(GinkgoLogr).WebhookServer.(*webhook.DefaultServer).Options.TLSOpts).To(HaveLen(expectedTLSOptsLen))
		},
		Entry("HTTP/2 disabled", true, 1),
		Entry("HTTP/2 enabled", false, 0),
	)

	DescribeTable(
		"should enable or disable HTTP/2 in the metrics server",
		func(secureServing, disableHTTP2 bool, expectedTLSOptsLen int) {
			cfg := Config{
				Metrics: Metrics{
					DisableHTTP2:  disableHTTP2,
					SecureServing: secureServing,
				},
			}

			Expect(cfg.ManagerOptions(GinkgoLogr).Metrics.TLSOpts).To(HaveLen(expectedTLSOptsLen))
		},
		Entry("secure serving disabled, HTTP/2 disabled", false, true, 0),
		Entry("secure serving enabled, HTTP/2 disabled", true, true, 1),
		Entry("secure serving disabled, HTTP/2 enabled", false, false, 0),
		Entry("secure serving enabled, HTTP/2 enabled", true, false, 0),
	)

	DescribeTable(
		"should enable authn/authz if configured",
		func(enabled bool) {
			c := &Config{
				Metrics: Metrics{EnableAuthnAuthz: enabled},
			}

			mo := c.ManagerOptions(GinkgoLogr)

			if enabled {
				Expect(mo.Metrics.FilterProvider).NotTo(BeNil())
			} else {
				Expect(mo.Metrics.FilterProvider).To(BeNil())
			}
		},
		Entry(nil, false),
		Entry(nil, true),
	)
})
