package config

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/utils/pointer"
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
			MetricsBindAddress:     "127.0.0.1:8080",
			LeaderElection: LeaderElection{
				Enabled:    true,
				ResourceID: "some-resource-id",
			},
			Webhook: Webhook{
				DisableHTTP2: true,
				Port:         9443,
			},
			Worker: Worker{
				RunAsUser:            pointer.Int64(1234),
				SELinuxType:          "mySELinuxType",
				SetFirmwareClassPath: pointer.String("/some/path"),
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
	DescribeTable(
		"should work as expected",
		func(disableHTTP2 bool) {
			const (
				healthProbeBindAddress   = ":8081"
				metricsBindAddress       = "127.0.0.1:8080"
				leaderElectionEnabled    = true
				leaderElectionResourceID = "some-resource-id"
				webhookPort              = 1234
			)

			cfg := Config{
				HealthProbeBindAddress: healthProbeBindAddress,
				MetricsBindAddress:     metricsBindAddress,
				LeaderElection: LeaderElection{
					Enabled:    leaderElectionEnabled,
					ResourceID: leaderElectionResourceID,
				},
				Webhook: Webhook{
					DisableHTTP2: disableHTTP2,
					Port:         webhookPort,
				},
				Worker: Worker{},
			}

			opts := cfg.ManagerOptions(GinkgoLogr)

			Expect(opts.HealthProbeBindAddress).To(Equal(healthProbeBindAddress))
			Expect(opts.MetricsBindAddress).To(Equal(metricsBindAddress))
			Expect(opts.LeaderElection).To(Equal(leaderElectionEnabled))
			Expect(opts.LeaderElectionID).To(Equal(leaderElectionResourceID))

			expectedTLSOptsLen := 0

			if disableHTTP2 {
				expectedTLSOptsLen = 1
			}

			Expect(opts.WebhookServer.(*webhook.DefaultServer).Options.TLSOpts).To(HaveLen(expectedTLSOptsLen))
			Expect(opts.WebhookServer.(*webhook.DefaultServer).Options.Port).To(Equal(webhookPort))
		},
		Entry("HTTP/2 disabled", false),
		Entry("HTTP/2 enabled", true),
	)
})
