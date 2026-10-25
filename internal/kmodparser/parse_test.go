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

package kmodparser

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Parse", func() {
	It("omits an out-of-tree dependency and keeps a host dependency on the same line", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/mymod.ko: extra/helper.ko host/kernel/crypto/crc32.ko.xz\n",
			"extra/mymod.ko":  "",
			"extra/helper.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32"}))
		Expect(jsonText(got)).NotTo(ContainSubstring("softLinks"))
	})

	It("collects host dependencies from an out-of-tree line that is not in moduleNames", func() {
		root := writeFiles(map[string]string{
			"modules.dep":    "extra/mymod.ko: host/kernel/crypto/crc32.ko.xz\nextra/other.ko: host/kernel/lib/crc32c.ko.xz\n",
			"extra/mymod.ko": "",
			"extra/other.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32", "crc32c"}))
	})

	It("adds in-tree softdep names and omits an out-of-tree softdep name", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/mymod.ko:\n",
			"modules.softdep": "softdep mymod pre: crc32c post: helper\n",
			"extra/mymod.ko":  "",
			"extra/helper.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32c"}))
	})

	It("ignores a softdep line whose module is only reachable through host", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/mymod.ko: host/kernel/crypto/crc32.ko.xz\n",
			"modules.softdep": "softdep crc32 pre: othermod\nsoftdep mymod pre: crc32c\n",
			"extra/mymod.ko":  "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32", "crc32c"}))
	})

	It("keeps the hard-dependency result when modules.softdep is missing", func() {
		root := writeFiles(map[string]string{
			"modules.dep":    "extra/mymod.ko: host/kernel/crypto/crc32.ko.xz\n",
			"extra/mymod.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32"}))
	})

	It("leaves out names in moduleNames or omit and still collects their dependencies", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/mymod.ko: host/kernel/crypto/crc32.ko.xz\n",
			"modules.softdep": "softdep mymod pre: crc32c\n",
			"extra/mymod.ko":  "",
		})
		req := baseRequest(root)
		req.Omit = []string{"crc32", "mymod"}

		got := parseOK(req)

		Expect(got.InTreeModules).To(Equal([]string{"crc32c"}))
		Expect(got.InTreeModules).NotTo(ContainElement("mymod"))
		Expect(got.InTreeModules).NotTo(ContainElement("crc32"))
	})

	It("collapses two host dependencies with the same basename and sorts the list", func() {
		root := writeFiles(map[string]string{
			"modules.dep":    "extra/mymod.ko: host/kernel/lib/crc32c.ko.xz host/kernel/crypto/crc32.ko.xz host/kernel/other/crc32.ko.xz\n",
			"extra/mymod.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32", "crc32c"}))
	})

	It("returns an empty list and no link fields when nothing is in-tree", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/mymod.ko: extra/helper.ko\n",
			"extra/mymod.ko":  "",
			"extra/helper.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(jsonText(got)).To(Equal(`{"inTreeModules":[]}`))
	})

	It("returns ErrMissingModulesDep when modules.dep is absent", func() {
		root := writeFiles(map[string]string{})

		_, err := NewParser().Parse(baseRequest(root))

		Expect(err).To(MatchError(ErrMissingModulesDep))
	})

	It("returns ErrUnreadableModulesDep when modules.dep cannot be read", func() {
		root := GinkgoT().TempDir()
		Expect(os.Mkdir(filepath.Join(root, "modules.dep"), 0o755)).To(Succeed())

		_, err := NewParser().Parse(baseRequest(root))

		Expect(err).To(MatchError(ErrUnreadableModulesDep))
	})

	It("returns ErrUnreadableModulesSoftdep when modules.softdep cannot be read", func() {
		root := writeFiles(map[string]string{
			"modules.dep":    "extra/mymod.ko:\n",
			"extra/mymod.ko": "",
		})
		Expect(os.Mkdir(filepath.Join(root, "modules.softdep"), 0o755)).To(Succeed())

		_, err := NewParser().Parse(baseRequest(root))

		Expect(err).To(MatchError(ErrUnreadableModulesSoftdep))
	})

	It("returns ErrModuleNotInModulesDep when a requested module has no line", func() {
		root := writeFiles(map[string]string{
			"modules.dep": "extra/other.ko:\n",
		})

		_, err := NewParser().Parse(baseRequest(root))

		Expect(err).To(MatchError(ErrModuleNotInModulesDep))
	})

	It("returns ErrInvalidModulesDep for a line without a colon", func() {
		root := writeFiles(map[string]string{
			"modules.dep": "extra/mymod.ko host/kernel/crypto/crc32.ko.xz\n",
		})

		_, err := NewParser().Parse(baseRequest(root))

		Expect(err).To(MatchError(ErrInvalidModulesDep))
	})

	It("returns ErrAmbiguousModule when two lines share a normalized basename", func() {
		root := writeFiles(map[string]string{
			"modules.dep": "extra/a/mymod.ko:\nextra/b/mymod.ko.xz:\n",
		})

		_, err := NewParser().Parse(baseRequest(root))

		Expect(err).To(MatchError(ErrAmbiguousModule))
	})

	DescribeTable("rejects a softdep line without a module name",
		func(line string) {
			root := writeFiles(map[string]string{
				"modules.dep":     "extra/mymod.ko:\n",
				"modules.softdep": line + "\n",
				"extra/mymod.ko":  "",
			})

			_, err := NewParser().Parse(baseRequest(root))

			Expect(err).To(MatchError(ErrInvalidModulesSoftdep))
		},
		Entry("missing module name", "softdep"),
		Entry("marker used as the module name", "softdep pre: crc32c"),
	)

	It("collects a softdep name written without a pre or post marker", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/mymod.ko:\n",
			"modules.softdep": "softdep mymod crc32c\n",
			"extra/mymod.ko":  "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32c"}))
	})

	It("treats hyphen and underscore as the same module name", func() {
		root := writeFiles(map[string]string{
			"modules.dep":     "extra/my-mod.ko: host/kernel/crypto/crc-32.ko.xz\n",
			"modules.softdep": "softdep my_mod pre: crc32c\n",
			"extra/my-mod.ko": "",
		})
		req := baseRequest(root)
		req.ModuleNames = []string{"my_mod"}

		got := parseOK(req)

		Expect(got.InTreeModules).To(Equal([]string{"crc32c", "crc_32"}))
	})

	It("returns ErrMissingModulesRoot when the modules root is empty", func() {
		req := baseRequest("")
		req.ModulesRoot = ""

		_, err := NewParser().Parse(req)

		Expect(err).To(MatchError(ErrMissingModulesRoot))
	})

	It("does not add a kernel-prefix dependency", func() {
		root := writeFiles(map[string]string{
			"modules.dep":    "extra/mymod.ko: kernel/crypto/crc32.ko.xz\n",
			"extra/mymod.ko": "",
		})

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{}))
	})

	It("does not treat files behind the host symlink as out-of-tree", func() {
		root := GinkgoT().TempDir()
		outside := GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(outside, "kernel"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(outside, "kernel", "crc32.ko"), []byte(""), 0o644)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(root, "extra"), 0o755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "extra", "mymod.ko"), []byte(""), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "modules.dep"), []byte("extra/mymod.ko:\n"), 0o644)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "modules.softdep"), []byte("softdep mymod pre: crc32\n"), 0o644)).To(Succeed())
		Expect(os.Symlink(outside, filepath.Join(root, "host"))).To(Succeed())

		got := parseOK(baseRequest(root))

		Expect(got.InTreeModules).To(Equal([]string{"crc32"}))
	})

	DescribeTable("matches an unsuffixed moduleNames entry and strips the output suffix",
		func(fileName string) {
			root := writeFiles(map[string]string{
				"modules.dep":       "extra/" + fileName + ": host/kernel/crypto/crc32" + koSuffix(fileName) + "\n",
				"extra/" + fileName: "",
			})
			req := baseRequest(root)
			req.ModuleNames = []string{"mymod"}

			got := parseOK(req)

			Expect(got.InTreeModules).To(Equal([]string{"crc32"}))
		},
		Entry(".ko", "mymod.ko"),
		Entry(".ko.xz", "mymod.ko.xz"),
		Entry(".ko.gz", "mymod.ko.gz"),
	)

	It("returns the design example JSON", func() {
		root := writeFiles(map[string]string{
			"modules.dep":       "extra/mymod.ko.xz: host/kernel/crypto/crc32.ko.xz\n",
			"modules.softdep":   "softdep mymod pre: crc32c\n",
			"extra/mymod.ko.xz": "",
		})
		got := parseOK(baseRequest(root))

		Expect(jsonText(got)).To(Equal(`{"inTreeModules":["crc32","crc32c"]}`))
	})
})

func baseRequest(root string) Request {
	return Request{
		ModuleNames: []string{"mymod"},
		ModulesRoot: root,
	}
}

func parseOK(req Request) Success {
	got, err := NewParser().Parse(req)
	Expect(err).NotTo(HaveOccurred())
	return got
}

func jsonText(v any) string {
	body, err := json.Marshal(v)
	Expect(err).NotTo(HaveOccurred())
	return string(body)
}

func koSuffix(fileName string) string {
	switch {
	case strings.HasSuffix(fileName, ".ko.xz"):
		return ".ko.xz"
	case strings.HasSuffix(fileName, ".ko.gz"):
		return ".ko.gz"
	default:
		return ".ko"
	}
}

func writeFiles(entries map[string]string) string {
	root := GinkgoT().TempDir()
	for rel, body := range entries {
		path := filepath.Join(root, rel)
		Expect(os.MkdirAll(filepath.Dir(path), 0o755)).To(Succeed())
		Expect(os.WriteFile(path, []byte(body), 0o644)).To(Succeed())
	}
	return root
}
