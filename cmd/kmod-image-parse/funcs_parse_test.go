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

package main

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rh-ecosystem-edge/kernel-module-management/internal/kmodparser"
	"github.com/spf13/cobra"
	"go.uber.org/mock/gomock"
)

var _ = Describe("parseFunc", func() {
	var mp *kmodparser.MockParser

	BeforeEach(func() {
		ctrl := gomock.NewController(GinkgoT())
		mp = kmodparser.NewMockParser(ctrl)
		kmodParser = mp
	})

	AfterEach(func() {
		kmodParser = kmodparser.NewParser()
		terminationMessagePath = "/dev/termination-log"
	})

	It("applies flag defaults", func() {
		cmd := newCommand()

		Expect(cmd.ParseFlags(nil)).To(Succeed())
		Expect(cmd.Flags().GetString("modules-root")).To(Equal("/modules"))
	})

	It("collects repeated modules-names and omit flags", func() {
		cmd := newCommand()

		Expect(cmd.ParseFlags([]string{
			"--modules-names", "mymod",
			"--modules-names", "other",
			"--omit", "crc32",
			"--omit", "crc32c",
		})).To(Succeed())
		Expect(cmd.Flags().GetStringArray("modules-names")).To(Equal([]string{"mymod", "other"}))
		Expect(cmd.Flags().GetStringArray("omit")).To(Equal([]string{"crc32", "crc32c"}))
	})

	It("writes compact success JSON", func() {
		logPath := filepath.Join(GinkgoT().TempDir(), "termination-log")
		terminationMessagePath = logPath
		req := kmodparser.Request{
			ModuleNames: []string{"mymod"},
			Omit:        []string{"crc32"},
			ModulesRoot: "/modules",
		}
		mp.EXPECT().Parse(req).Return(kmodparser.Success{InTreeModules: []string{"crc32", "crc32c"}}, nil)
		cmd := newCommand()
		cmd.SetArgs([]string{
			"--modules-root", req.ModulesRoot,
			"--modules-names", "mymod",
			"--omit", "crc32",
		})

		Expect(cmd.Execute()).To(Succeed())
		body, err := os.ReadFile(logPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal(`{"inTreeModules":["crc32","crc32c"]}`))
	})

	It("writes missing_modules.dep and returns that error", func() {
		logPath := filepath.Join(GinkgoT().TempDir(), "termination-log")
		terminationMessagePath = logPath
		req := kmodparser.Request{
			ModuleNames: []string{},
			Omit:        []string{},
			ModulesRoot: "/modules",
		}
		mp.EXPECT().Parse(req).Return(kmodparser.Success{}, kmodparser.ErrMissingModulesDep)
		cmd := newCommand()
		cmd.SetArgs([]string{
			"--modules-root", req.ModulesRoot,
		})

		Expect(cmd.Execute()).To(MatchError(kmodparser.ErrMissingModulesDep))
		body, err := os.ReadFile(logPath)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(body)).To(Equal(`{"error":"missing_modules.dep"}`))
	})
})

func newCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "kmod-image-parse",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          parseFunc,
	}
	cmd.SetOut(GinkgoWriter)
	cmd.SetErr(GinkgoWriter)
	setCommandsFlags(cmd)
	return cmd
}
