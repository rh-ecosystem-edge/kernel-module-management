/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package version

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Version Suite")
}

var _ = Describe("AtLeast", func() {
	DescribeTable(
		"should compare versions correctly",
		func(major, minor, minMajor, minMinor int, expected bool) {
			ov := OCPVersion{Major: major, Minor: minor}
			Expect(ov.AtLeast(minMajor, minMinor)).To(Equal(expected))
		},
		Entry("exact match", 4, 21, 4, 21, true),
		Entry("higher minor", 4, 22, 4, 21, true),
		Entry("lower minor", 4, 20, 4, 21, false),
		Entry("higher major, lower minor", 5, 0, 4, 21, true),
		Entry("lower major", 3, 11, 4, 21, false),
	)
})

var _ = Describe("ParseOCPVersion", func() {
	DescribeTable(
		"should parse valid version strings",
		func(version string, expectedMajor, expectedMinor int) {
			ov, err := ParseOCPVersion(version)
			Expect(err).NotTo(HaveOccurred())
			Expect(ov.Major).To(Equal(expectedMajor))
			Expect(ov.Minor).To(Equal(expectedMinor))
		},
		Entry("standard version", "4.21.0", 4, 21),
		Entry("release candidate", "4.21.0-rc.1", 4, 21),
		Entry("older minor", "4.20.3", 4, 20),
		Entry("newer minor", "4.22.1", 4, 22),
		Entry("with v prefix", "v4.21.0", 4, 21),
		Entry("hypothetical OCP 5.0", "5.0.0", 5, 0),
	)

	DescribeTable(
		"should return error for invalid strings",
		func(version string) {
			_, err := ParseOCPVersion(version)
			Expect(err).To(HaveOccurred())
		},
		Entry("empty string", ""),
		Entry("garbage", "invalid"),
		Entry("no minor", "v4"),
	)
})
