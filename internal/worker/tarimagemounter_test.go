package worker

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/empty"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
)

var _ = Describe("TarImageMounter_MountImage", func() {
	var oimh *MockociImageMounterHelperAPI

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		oimh = NewMockociImageMounterHelperAPI(ctrl)
	})

	It("good flow", func() {
		tmpDir := GinkgoT().TempDir()
		testImage := prepareTestTarball(tmpDir)

		tim := &tarImageMounter{
			ociImageHelper: oimh,
			baseDir:        tmpDir,
			logger:         GinkgoLogr,
		}

		oimh.EXPECT().mountOCIImage(gomock.Any(), filepath.Join(tmpDir, testImage, "fs")).Return(nil)

		res, err := tim.MountImage(context.Background(), testImage, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(res).To(Equal(filepath.Join(tmpDir, testImage, "fs")))
	})

	It("failed to mount OCI image", func() {
		tmpDir := GinkgoT().TempDir()
		testImage := prepareTestTarball(tmpDir)

		tim := &tarImageMounter{
			ociImageHelper: oimh,
			baseDir:        tmpDir,
			logger:         GinkgoLogr,
		}

		oimh.EXPECT().mountOCIImage(gomock.Any(), filepath.Join(tmpDir, testImage, "fs")).Return(fmt.Errorf("some error"))

		res, err := tim.MountImage(context.Background(), testImage, nil)
		Expect(err).To(HaveOccurred())
		Expect(res).To(BeEmpty())
	})
})

func prepareTestTarball(tarballDir string) string {
	srcImg, err := crane.Append(empty.Image, "testdata/archive.tar")
	Expect(err).NotTo(HaveOccurred())
	reference, err := name.NewTag("some-tag")
	Expect(err).NotTo(HaveOccurred())
	tarballFilePath := filepath.Join(tarballDir, "input-image-tarball-file")
	err = tarball.WriteToFile(tarballFilePath, reference, srcImg)
	Expect(err).NotTo(HaveOccurred())
	return tarballFilePath
}
