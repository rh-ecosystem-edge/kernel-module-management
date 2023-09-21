package worker

import (
	"github.com/containers/image/v5/pkg/sysregistriesv2"
	"github.com/containers/image/v5/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	imageName = "registry.example.com/ns/img"
	mirror0   = "mirror0.example.com/ns/img"
	mirror1   = "mirror1.example.com/ns/img"
)

var _ = Describe("mirrorResolver_GetAllReferences", func() {
	It("should return the original name if no registry could be found", func() {
		findRegistry := func(_ *types.SystemContext, _ string) (*sysregistriesv2.Registry, error) {
			return nil, nil
		}

		mr := &mirrorResolver{
			findRegistry: findRegistry,
			logger:       GinkgoLogr,
		}

		Expect(
			mr.GetAllReferences(imageName),
		).To(
			Equal([]string{imageName}),
		)
	})

	It("should return the original name and mirrors if the source is not blocked", func() {
		findRegistry := func(_ *types.SystemContext, _ string) (*sysregistriesv2.Registry, error) {
			reg := sysregistriesv2.Registry{
				Prefix: imageName,
				Mirrors: []sysregistriesv2.Endpoint{
					{Location: mirror0},
					{Location: mirror1},
				},
				Endpoint: sysregistriesv2.Endpoint{Location: imageName},
				Blocked:  false,
			}

			return &reg, nil
		}

		mr := &mirrorResolver{
			findRegistry: findRegistry,
			logger:       GinkgoLogr,
		}

		Expect(
			mr.GetAllReferences(imageName),
		).To(
			Equal([]string{mirror0, mirror1, imageName}),
		)
	})

	It("should return the mirrors only if the source is blocked", func() {
		findRegistry := func(_ *types.SystemContext, _ string) (*sysregistriesv2.Registry, error) {
			reg := sysregistriesv2.Registry{
				Prefix: imageName,
				Mirrors: []sysregistriesv2.Endpoint{
					{Location: mirror0},
					{Location: mirror1},
					// should not be included in the list of pull sources because we are not getting mirrors for a digest image
					{Location: "mirror-digest.example.com/ns/img", PullFromMirror: sysregistriesv2.MirrorByDigestOnly},
				},
				Endpoint: sysregistriesv2.Endpoint{Location: imageName},
				Blocked:  true,
			}

			return &reg, nil
		}

		mr := &mirrorResolver{
			findRegistry: findRegistry,
			logger:       GinkgoLogr,
		}

		Expect(
			mr.GetAllReferences(imageName),
		).To(
			Equal([]string{mirror0, mirror1}),
		)
	})
})
