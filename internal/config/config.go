package config

import (
	"crypto/tls"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/http"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
)

type Webhook struct {
	DisableHTTP2 bool `yaml:"disableHTTP2"`
	Port         int  `yaml:"port"`
}

type Worker struct {
	RunAsUser            *int64  `yaml:"runAsUser"`
	SELinuxType          string  `yaml:"seLinuxType"`
	SetFirmwareClassPath *string `yaml:"setFirmwareClassPath,omitempty"`
}

type LeaderElection struct {
	Enabled    bool   `yaml:"enabled"`
	ResourceID string `yaml:"resourceID"`
}

type Config struct {
	HealthProbeBindAddress string         `yaml:"healthProbeBindAddress"`
	MetricsBindAddress     string         `yaml:"metricsBindAddress"`
	LeaderElection         LeaderElection `yaml:"leaderElection"`
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

	return &manager.Options{
		HealthProbeBindAddress: c.HealthProbeBindAddress,
		LeaderElection:         c.LeaderElection.Enabled,
		LeaderElectionID:       c.LeaderElection.ResourceID,
		MetricsBindAddress:     c.MetricsBindAddress,
		WebhookServer:          webhook.NewServer(webhookOpts),
	}
}
