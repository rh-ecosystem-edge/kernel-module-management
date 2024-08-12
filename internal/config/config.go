package config

import (
	"crypto/tls"
	"fmt"
	"os"
	"time"

	"github.com/go-logr/logr"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/http"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type Job struct {
	GCDelay time.Duration `yaml:"gcDelay,omitempty"`
}

type Webhook struct {
	DisableHTTP2 bool `yaml:"disableHTTP2"`
	Port         int  `yaml:"port"`
}

type Worker struct {
	RunAsUser        *int64  `yaml:"runAsUser"`
	SELinuxType      string  `yaml:"seLinuxType"`
	FirmwareHostPath *string `yaml:"firmwareHostPath,omitempty"`
}

type LeaderElection struct {
	Enabled    bool   `yaml:"enabled"`
	ResourceID string `yaml:"resourceID"`
}

type Metrics struct {
	BindAddress      string `yaml:"bindAddress"`
	DisableHTTP2     bool   `yaml:"disableHTTP2"`
	EnableAuthnAuthz bool   `yaml:"enableAuthnAuthz"`
	SecureServing    bool   `yaml:"secureServing"`
}

type Config struct {
	HealthProbeBindAddress string         `yaml:"healthProbeBindAddress"`
	Job                    Job            `yaml:"job"`
	LeaderElection         LeaderElection `yaml:"leaderElection"`
	Metrics                Metrics        `yaml:"metrics"`
	Webhook                Webhook        `yaml:"webhook"`
	Worker                 Worker         `yaml:"worker"`
}

func ParseFile(path string) (*Config, error) {
	fd, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("could not open the configuration file: %v", err)
	}
	defer fd.Close()

	cfg := Config{}

	if err = yaml.NewDecoder(fd).Decode(&cfg); err != nil {
		return nil, fmt.Errorf("could not decode configuration file: %v", err)
	}

	return &cfg, nil
}

func (c *Config) ManagerOptions(logger logr.Logger) *manager.Options {
	webhookOpts := webhook.Options{Port: c.Webhook.Port}

	if c.Webhook.DisableHTTP2 {
		logger.Info("Disabling HTTP/2 in the webhook server")
		webhookOpts.TLSOpts = []func(*tls.Config){http.DisableHTTP2}
	}

	metrics := server.Options{
		BindAddress:   c.Metrics.BindAddress,
		SecureServing: c.Metrics.SecureServing,
	}

	if c.Metrics.EnableAuthnAuthz {
		metrics.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	if c.Metrics.SecureServing && c.Metrics.DisableHTTP2 {
		logger.Info("Disabling HTTP/2 in the metrics server")
		metrics.TLSOpts = []func(*tls.Config){http.DisableHTTP2}
	}

	return &manager.Options{
		HealthProbeBindAddress: c.HealthProbeBindAddress,
		LeaderElection:         c.LeaderElection.Enabled,
		LeaderElectionID:       c.LeaderElection.ResourceID,
		Metrics:                metrics,
		WebhookServer:          webhook.NewServer(webhookOpts),
	}
}
