/*
Copyright The ORAS Authors.
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

package command

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"oras.land/oras/test/e2e/internal/utils"
)

var _ = ginkgo.Describe("OCI image layout users:", func() {
	ginkgo.When("pulling to an output archive", func() {
		prepare := func() (string, string) {
			root := ginkgo.GinkgoT().TempDir()
			gomega.Expect(os.MkdirAll(filepath.Join(root, "src", "empty"), 0755)).To(gomega.Succeed())
			gomega.Expect(os.WriteFile(filepath.Join(root, "README.txt"), []byte("source package\n"), 0644)).To(gomega.Succeed())
			gomega.Expect(os.WriteFile(filepath.Join(root, "src", "main.txt"), []byte("source content\n"), 0644)).To(gomega.Succeed())
			ref := utils.LayoutRef(filepath.Join(root, "layout"), "archive")
			utils.ORAS("push", utils.Flags.Layout, ref, "README.txt", "src").WithWorkDir(root).Exec()
			return root, ref
		}
		readArchive := func(filename string) map[string]string {
			archive, err := zip.OpenReader(filename)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			defer archive.Close()
			entries := make(map[string]string)
			for _, entry := range archive.File {
				rc, err := entry.Open()
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				data, err := io.ReadAll(rc)
				gomega.Expect(err).NotTo(gomega.HaveOccurred())
				gomega.Expect(rc.Close()).To(gomega.Succeed())
				entries[entry.Name] = string(data)
			}
			return entries
		}

		ginkgo.It("should archive directory layers reproducibly without extracting files", func() {
			root, ref := prepare()
			output := ginkgo.GinkgoT().TempDir()
			first := filepath.Join(output, "first.zip")
			second := filepath.Join(output, "second.zip")
			utils.ORAS("pull", utils.Flags.Layout, ref, "--output-archive", first).WithWorkDir(output).Exec()
			utils.ORAS("pull", utils.Flags.Layout, ref, "--output-archive", second).WithWorkDir(output).Exec()
			entries := readArchive(first)
			gomega.Expect(entries).To(gomega.HaveKeyWithValue("README.txt", "source package\n"))
			gomega.Expect(entries).To(gomega.HaveKeyWithValue("src/main.txt", "source content\n"))
			gomega.Expect(entries).To(gomega.HaveKey("src/empty/"))
			a, err := os.ReadFile(first)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			b, err := os.ReadFile(second)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(a).To(gomega.Equal(b))
			files, err := os.ReadDir(output)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(files).To(gomega.HaveLen(2))
			gomega.Expect(filepath.Join(output, "src")).NotTo(gomega.BeADirectory())
			gomega.Expect(filepath.Join(root, "src")).To(gomega.BeADirectory())
		})

		ginkgo.It("should preserve an existing output archive with keep-old-files", func() {
			root, ref := prepare()
			output := filepath.Join(root, "sources.zip")
			utils.ORAS("pull", utils.Flags.Layout, ref, "--output-archive", output).WithWorkDir(root).Exec()
			original, err := os.ReadFile(output)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			utils.ORAS("pull", utils.Flags.Layout, ref, "--output-archive", output, "--keep-old-files").WithWorkDir(root).ExpectFailure().Exec()
			current, err := os.ReadFile(output)
			gomega.Expect(err).NotTo(gomega.HaveOccurred())
			gomega.Expect(current).To(gomega.Equal(original))
		})

		ginkgo.It("should create a new output archive with keep-old-files", func() {
			root, ref := prepare()
			output := filepath.Join(root, "sources.zip")
			utils.ORAS("pull", utils.Flags.Layout, ref, "--output-archive", output, "--keep-old-files").WithWorkDir(root).Exec()
			gomega.Expect(readArchive(output)).To(gomega.HaveKeyWithValue("src/main.txt", "source content\n"))
		})
	})
})
