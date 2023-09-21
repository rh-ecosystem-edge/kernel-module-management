package worker

import (
	"errors"
	"fmt"

	"github.com/containers/image/v5/docker/reference"
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/containers/image/v5/types"
	"github.com/go-logr/logr"
)

//go:generate mockgen -source=mirror.go -package=worker -destination=mock_mirror.go

type MirrorResolver interface {
	GetAllReferences(imageName string) ([]string, error)
}

type mirrorResolver struct {
	findRegistry func(*types.SystemContext, string) (*sysregistriesv2.Registry, error)
	logger       logr.Logger
}

func NewMirrorResolver(logger logr.Logger) MirrorResolver {
	return &mirrorResolver{
		findRegistry: sysregistriesv2.FindRegistry,
		logger:       logger,
	}
}

// GetAllReferences reads /etc/containers/registries.conf and files under /etc/containers/registries.conf.d/
// to return a slice of all pull sources (also known as mirrors) for imageName.
// It honors the pull-from-mirror setting and adds limited support for the blocked setting.
func (m *mirrorResolver) GetAllReferences(imageName string) ([]string, error) {
	r, err := m.findRegistry(nil, imageName)
	if err != nil {
		return nil, fmt.Errorf("could not find registry for image %q: %w", imageName, err)
	}

	logger := m.logger.WithValues("image name", imageName)

	if r == nil {
		logger.Info("No configuration found for registry")
		return []string{imageName}, nil
	}

	if r.Blocked {
		m.logger.Info("registries.conf: registry blocked", "registry", r.Prefix)
	}

	n, err := reference.ParseNamed(imageName)
	if err != nil {
		return nil, fmt.Errorf("could not parse image name %q: %w", imageName, err)
	}

	pullSources, err := r.PullSourcesFromReference(n)
	if err != nil {
		return nil, fmt.Errorf("could not obtain pull sources: %v", err)
	}

	names := make([]string, 0, len(pullSources))

	for _, ps := range pullSources {
		name := ps.Reference.String()

		// Registry.PullSourcesFromReference() does not seem to handle Registry.Blocked yet.
		// Exclude the source name manually if it is blocked.
		if r.Blocked && name == imageName {
			continue
		}

		names = append(names, name)
	}

	if len(names) == 0 {
		return nil, errors.New("no pull sources to return")
	}

	return names, nil
}
