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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras/test/e2e/internal/testdata/artifact"
	"oras.land/oras/test/e2e/internal/testdata/feature"
	"oras.land/oras/test/e2e/internal/testdata/foobar"
	. "oras.land/oras/test/e2e/internal/utils"
	"oras.land/oras/test/e2e/internal/utils/match"
)

var _ = Describe("ORAS beginners:", func() {
	When("running push command", func() {
		It("should show help description with feature flags", func() {
			out := ORAS("push", "--help").MatchKeyWords(ExampleDesc).Exec().Out
			gomega.Expect(out).Should(gbytes.Say("--image-spec string\\s+%s", regexp.QuoteMeta(feature.Preview.Mark)))
			gomega.Expect(out).Should(gbytes.Say("--oci-layout-path string\\s+%s", regexp.QuoteMeta(feature.Experimental.Mark)))
		})

		It("should not show --verbose in help doc", func() {
			out := ORAS("push", "--help").MatchKeyWords(ExampleDesc).Exec().Out
			gomega.Expect(out).ShouldNot(gbytes.Say("--verbose"))
		})

		It("should show text as default format type in help doc", func() {
			MatchDefaultFlagValue("format", "text", "push")
		})

		It("should show deprecation message and print unnamed status output for --verbose", func() {
			repo := pushTestRepo("test-verbose")
			tag := "e2e"
			tempDir := PrepareTempFiles()
			stateKeys := []match.StateKey{
				artifact.DefaultConfigStateKey,
			}

			ORAS("push", RegistryRef(ZOTHost, repo, tag), "--verbose").
				WithWorkDir(tempDir).
				MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).
				MatchStatus(stateKeys, true, len(stateKeys)).
				Exec()
		})

		It("should show deprecation message and should NOT print unnamed status output for --verbose=false", func() {
			repo := pushTestRepo("test-verbose-false")
			tag := "e2e"
			tempDir := PrepareTempFiles()

			out := ORAS("push", RegistryRef(ZOTHost, repo, tag), "--verbose=false").
				WithWorkDir(tempDir).
				MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).
				Exec().Out
			// should not print status output for unnamed blobs
			gomega.Expect(out).ShouldNot(gbytes.Say("application/vnd.oci.empty.v1+json"))
			gomega.Expect(out).ShouldNot(gbytes.Say("application/vnd.oci.image.manifest.v1+json"))
		})

		It("should fail and show detailed error description if no argument provided", func() {
			err := ORAS("push").ExpectFailure().Exec().Err
			gomega.Expect(err).Should(gbytes.Say("Error"))
			gomega.Expect(err).Should(gbytes.Say("\nUsage: oras push"))
			gomega.Expect(err).Should(gbytes.Say("\n"))
			gomega.Expect(err).Should(gbytes.Say(`Run "oras push -h"`))
		})

		It("should fail if the provided reference is not valid", func() {
			err := ORAS("push", "/oras").ExpectFailure().Exec().Err
			gomega.Expect(err).Should(gbytes.Say(`Error: "/oras": invalid reference: invalid registry ""`))
			gomega.Expect(err).Should(gbytes.Say(regexp.QuoteMeta("Please make sure the provided reference is in the form of <registry>/<repo>[:tag|@digest]")))
		})

		It("should fail if the to-be-pushed file is not found", func() {
			tempDir := GinkgoT().TempDir()
			notFoundFilePath := "file/not/found"
			err := ORAS("push", RegistryRef(ZOTHost, pushTestRepo("file-not-found"), ""), notFoundFilePath).
				WithWorkDir(tempDir).
				ExpectFailure().Exec().Err
			gomega.Expect(err).Should(gbytes.Say(filepath.Join(tempDir, notFoundFilePath)))
			gomega.Expect(err).Should(gbytes.Say("no such file or directory"))
		})

		It("should fail to use --config and --artifact-type at the same time for OCI spec v1.0 registry", func() {
			tempDir := PrepareTempFiles()
			repo := pushTestRepo("no-mediatype")
			ref := RegistryRef(ZOTHost, repo, "")

			ORAS("push", ref, "--config", foobar.FileConfigName, "--artifact-type", "test/artifact+json", "--image-spec", "v1.0").ExpectFailure().WithWorkDir(tempDir).Exec()
		})

		It("should fail to use --artifact-platform and --config at the same time", func() {
			tempDir := PrepareTempFiles()
			repo := pushTestRepo("no-mediatype")
			ref := RegistryRef(ZOTHost, repo, "")

			ORAS("push", ref, "--artifact-platform", "linux/amd64", "--config", foobar.FileConfigName).ExpectFailure().WithWorkDir(tempDir).Exec()
		})

		It("should fail if image spec is not valid", func() {
			testRepo := attachTestRepo("invalid-image-spec")
			subjectRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)
			invalidFlag := "???"
			ORAS("push", subjectRef, Flags.ImageSpec, invalidFlag).
				ExpectFailure().
				MatchErrKeyWords("Error:", invalidFlag, "Available options: v1.1, v1.0").
				Exec()
		})

		It("should fail if image spec is not valid", func() {
			testRepo := attachTestRepo("invalid-image-spec")
			subjectRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)
			invalidFlag := "???"
			ORAS("push", subjectRef, Flags.ImageSpec, invalidFlag).
				ExpectFailure().
				MatchErrKeyWords("Error:", invalidFlag, "Available options: v1.1, v1.0").
				Exec()
		})

		It("should fail if image spec is not valid", func() {
			testRepo := attachTestRepo("invalid-image-spec")
			subjectRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)
			invalidFlag := "???"
			ORAS("push", subjectRef, Flags.ImageSpec, invalidFlag).
				ExpectFailure().
				MatchErrKeyWords("Error:", invalidFlag, "Available options: v1.1, v1.0").
				Exec()
		})

		It("should fail if image spec v1.1 is used, with --config and without --artifactType", func() {
			testRepo := pushTestRepo("v1-1/no-artifact-type")
			subjectRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)
			imageSpecFlag := "v1.1"
			ORAS("push", subjectRef, "--config", foobar.FileConfigName, Flags.ImageSpec, imageSpecFlag).
				ExpectFailure().
				MatchErrKeyWords("missing artifact type for OCI image-spec v1.1 artifacts").
				Exec()
		})
	})
})

func pushTestRepo(text string) string {
	return fmt.Sprintf("command/push/%d/%s", GinkgoRandomSeed(), text)
}

var _ = Describe("Remote registry users:", func() {
	tag := "e2e"
	When("pushing to OCI spec v1.0 registries", func() {
		statusKeys := []match.StateKey{
			foobar.ImageConfigStateKey("application/vnd.oci.empty.v1+json"),
			foobar.FileBarStateKey,
		}
		It("should push files without customized media types", func() {
			repo := pushTestRepo("no-mediatype")
			tempDir := PrepareTempFiles()
			ref := RegistryRef(ZOTHost, repo, tag)

			ORAS("push", ref, foobar.FileBarName).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
		})

		It("should push files with path validation disabled", func() {
			repo := pushTestRepo("disable-path-validation")
			ref := RegistryRef(ZOTHost, repo, tag)
			absBarName := filepath.Join(PrepareTempFiles(), foobar.FileBarName)

			ORAS("push", ref, absBarName, "--disable-path-validation").
				Exec()

			// validate
			fetched := ORAS("manifest", "fetch", ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(ocispec.Descriptor{
				MediaType: "application/vnd.oci.image.layer.v1.tar",
				Digest:    digest.Digest(foobar.BarBlobDigest),
				Size:      3,
				Annotations: map[string]string{
					"org.opencontainers.image.title": absBarName,
				},
			}))
		})

		It("should fail path validation when pushing file with absolute path", func() {
			repo := pushTestRepo("path-validation")
			ref := RegistryRef(ZOTHost, repo, tag)
			absBarName := filepath.Join(PrepareTempFiles(), foobar.FileBarName)
			// test
			ORAS("push", ref, absBarName).
				MatchErrKeyWords("--disable-path-validation").
				ExpectFailure().
				Exec()
		})

		It("should push files and tag", func() {
			repo := pushTestRepo("multi-tag")
			tempDir := PrepareTempFiles()
			extraTag := "2e2"

			ORAS("push", fmt.Sprintf("%s,%s", RegistryRef(ZOTHost, repo, tag), extraTag), foobar.FileBarName, "--format", "go-template={{range .referenceAsTags}}{{println .}}{{end}}").
				MatchContent(fmt.Sprintf("%s\n%s\n", RegistryRef(ZOTHost, repo, extraTag), RegistryRef(ZOTHost, repo, tag))).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))

			fetched = ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, extraTag)).Exec().Out.Contents()
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
		})

		It("should push and tag and hide tag logs", func() {
			repo := pushTestRepo("no-tag-log")
			tempDir := PrepareTempFiles()
			extraTag := "2e2"

			out := ORAS("push", fmt.Sprintf("%s,%s", RegistryRef(ZOTHost, repo, tag), extraTag), foobar.FileBarName, "--format", "json").
				WithWorkDir(tempDir).
				Exec().Out.Contents()
			Expect(json.Unmarshal(out, &struct{}{})).ShouldNot(HaveOccurred())
		})

		It("should push files with customized media types", func() {
			repo := pushTestRepo("layer-mediatype")
			layerType := "layer/type"
			tempDir := PrepareTempFiles()
			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName+":"+layerType).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor(layerType)))
		})

		It("should push files with manifest exported", func() {
			repo := pushTestRepo("export-manifest")
			layerType := "layer/type"
			tempDir := PrepareTempFiles()
			exportPath := "packed.json"
			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName+":"+layerType, "--export-manifest", exportPath).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			MatchFile(filepath.Join(tempDir, exportPath), string(fetched), DefaultTimeout)
		})

		It("should push files with customized config file", func() {
			repo := pushTestRepo("config")
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), "--config", foobar.FileConfigName, foobar.FileBarName).
				MatchStatus([]match.StateKey{
					foobar.FileConfigStateKey,
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: "application/vnd.unknown.config.v1+json",
				Size:      int64(foobar.FileConfigSize),
				Digest:    foobar.FileConfigDigest,
			}))
		})

		It("should pack with image spec v1.0 when --config is used, --artifact-type is not used, and --image-spec set to auto", func() {
			repo := pushTestRepo("config/without/artifact/type")
			configType := "my/config+type"
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), "--config", fmt.Sprintf("%s:%s", foobar.FileConfigName, configType), foobar.FileBarName).
				MatchStatus([]match.StateKey{
					{Digest: foobar.FileConfigStateKey.Digest, Name: configType},
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: configType,
				Size:      int64(foobar.FileConfigSize),
				Digest:    foobar.FileConfigDigest,
			}))
			Expect(manifest.ArtifactType).Should(Equal(""))
		})

		It("should push files with customized config file and mediatype", func() {
			repo := pushTestRepo("config/mediatype")
			configType := "config/type"
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), "--config", fmt.Sprintf("%s:%s", foobar.FileConfigName, configType), foobar.FileBarName).
				MatchStatus([]match.StateKey{
					{Digest: foobar.FileConfigStateKey.Digest, Name: configType},
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: configType,
				Size:      int64(foobar.FileConfigSize),
				Digest:    foobar.FileConfigDigest,
			}))
		})

		It("should push files with customized manifest annotation", func() {
			repo := pushTestRepo("manifest-annotation")
			key := "image-anno-key"
			value := "image-anno-value"
			tempDir := PrepareTempFiles()
			// test
			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--annotation", fmt.Sprintf("%s=%s", key, value)).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Annotations[key]).To(Equal(value))
		})

		It("should push files with customized file annotation", func() {
			repo := pushTestRepo("file-annotation")
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--annotation-file", "foobar/annotation.json", "--config", foobar.FileConfigName).
				MatchStatus(statusKeys, true, 1).
				WithWorkDir(tempDir).Exec()

			// validate
			// see testdata\files\foobar\annotation.json
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Annotations["hi"]).To(Equal("manifest"))
			Expect(manifest.Config.Annotations["hello"]).To(Equal("config"))
			Expect(len(manifest.Layers)).To(Equal(1))
			Expect(manifest.Layers[0].Annotations["foo"]).To(Equal("bar"))
		})

		It("should allow renaming files using customized file annotation", func() {
			repo := pushTestRepo("file-annotation-rename")
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--annotation-file", "foobar/annotation-rename.json", "--config", foobar.FileConfigName).
				WithWorkDir(tempDir).Exec()

			// validate
			// see testdata\files\foobar\annotation-rename.json
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers[0].Annotations["org.opencontainers.image.title"]).To(Equal("foobar/baz"))
		})
	})

	When("pushing to OCI spec v1.1 registries", func() {
		It("should push artifact without layer", func() {
			repo := pushTestRepo("artifact-no-layer")
			tempDir := PrepareTempFiles()
			artifactType := "test/artifact+json"
			annotationKey := "key"
			annotationValue := "value"

			// test
			ORAS("push", RegistryRef(ZOTHost, repo, tag), "-a", fmt.Sprintf("%s=%s", annotationKey, annotationValue), "--artifact-type", artifactType).
				MatchStatus([]match.StateKey{artifact.DefaultConfigStateKey}, true, 1).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.ArtifactType).Should(Equal(artifactType))
			Expect(manifest.Layers).Should(HaveLen(1))
			Expect(manifest.Layers[0]).Should(Equal(artifact.EmptyLayerJSON))
			Expect(manifest.Config).Should(Equal(artifact.EmptyLayerJSON))
			Expect(manifest.Annotations).NotTo(BeNil())
			Expect(manifest.Annotations[annotationKey]).Should(Equal(annotationValue))
		})

		It("should push artifact and format reference", func() {
			repo := pushTestRepo("format-go-template")
			tempDir := PrepareTempFiles()
			annotationKey := "key"
			annotationValue := "value"

			// test
			out := ORAS("push", RegistryRef(ZOTHost, repo, tag), "-a", fmt.Sprintf("%s=%s", annotationKey, annotationValue), "--format", "go-template={{.reference}}").
				WithWorkDir(tempDir).Exec().Out

			// validate
			ref := string(out.Contents())
			fetched := ORAS("manifest", "fetch", ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(HaveLen(1))
			Expect(manifest.Layers[0]).Should(Equal(artifact.EmptyLayerJSON))
			Expect(manifest.Config).Should(Equal(artifact.EmptyLayerJSON))
			Expect(manifest.Annotations).NotTo(BeNil())
			Expect(manifest.Annotations[annotationKey]).Should(Equal(annotationValue))
		})

		It("should push artifact and format json", func() {
			repo := pushTestRepo("format-json")
			tempDir := PrepareTempFiles()
			artifactType := "test/artifact+json"
			annotationKey := "key"
			annotationValue := "value"

			// test
			out := ORAS("push", RegistryRef(ZOTHost, repo, tag), "-a", fmt.Sprintf("%s=%s", annotationKey, annotationValue), "--format", "json", "--artifact-type", artifactType).
				WithWorkDir(tempDir).Exec().Out

			// validate
			Expect(out).To(gbytes.Say(RegistryRef(ZOTHost, repo, "")))
			Expect(out).To(gbytes.Say(regexp.QuoteMeta(fmt.Sprintf(`"artifactType": "%s"`, artifactType))))
		})

		It("should push files", func() {
			repo := pushTestRepo("artifact-with-blob")
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName).
				MatchStatus([]match.StateKey{foobar.FileBarStateKey, artifact.DefaultConfigStateKey}, true, 2).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.ArtifactType).Should(Equal("application/vnd.unknown.artifact.v1"))
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
			Expect(manifest.Config).Should(Equal(artifact.EmptyLayerJSON))
		})

		It("should output artifact type when push is complete for image-spec v1.1", func() {
			repo := pushTestRepo("print-artifact-type-v1-1")
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--image-spec", "v1.1").
				MatchKeyWords("ArtifactType: ", "application/vnd.unknown.artifact.v1").
				WithWorkDir(tempDir).Exec()
		})

		It("should output artifact type when push is complete for image-spec v1.0 when --config is used", func() {
			repo := pushTestRepo("print-artifact-type-v1-0-config")
			configType := "config/type"
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), "--config", fmt.Sprintf("%s:%s", foobar.FileConfigName, configType), foobar.FileBarName, "--image-spec", "v1.0").
				MatchKeyWords("ArtifactType: ", configType).
				WithWorkDir(tempDir).Exec()
		})

		It("should push v1.1-rc.4 artifact", func() {
			repo := pushTestRepo("v1.1-artifact")
			tempDir := PrepareTempFiles()

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--image-spec", "v1.1").
				MatchStatus([]match.StateKey{foobar.FileBarStateKey, artifact.DefaultConfigStateKey}, true, 2).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.ArtifactType).Should(Equal("application/vnd.unknown.artifact.v1"))
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
			Expect(manifest.Config).Should(Equal(artifact.EmptyLayerJSON))
		})

		It("should push artifact with config", func() {
			repo := pushTestRepo("artifact-with-config")
			tempDir := PrepareTempFiles()
			configType := "test/config+json"

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--config", fmt.Sprintf("%s:%s", foobar.FileConfigName, configType)).
				MatchStatus([]match.StateKey{
					foobar.FileBarStateKey,
					{Digest: foobar.FileConfigStateKey.Digest, Name: configType},
					artifact.DefaultConfigStateKey}, true, 2).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.ArtifactType).Should(Equal(""))
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
			Expect(manifest.Config.MediaType).Should(Equal(configType))
			Expect(manifest.Config.Digest).Should(Equal(foobar.FileConfigDigest))
		})

		It("should push artifact with artifact type and config data", func() {
			repo := pushTestRepo("artifact-type-and-config")
			tempDir := PrepareTempFiles()
			artifactType := "test/artifact+json"
			configType := "test/config+json"

			ORAS("push", RegistryRef(ZOTHost, repo, tag), foobar.FileBarName, "--artifact-type", artifactType, "--config", fmt.Sprintf("%s:%s", foobar.FileConfigName, configType)).
				MatchStatus([]match.StateKey{
					foobar.FileBarStateKey,
					{Digest: foobar.FileConfigStateKey.Digest, Name: configType},
					artifact.DefaultConfigStateKey}, true, 2).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", RegistryRef(ZOTHost, repo, tag)).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.ArtifactType).Should(Equal(artifactType))
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
			Expect(manifest.Config.MediaType).Should(Equal(configType))
			Expect(manifest.Config.Digest).Should(Equal(foobar.FileConfigDigest))
		})
	})
})

var _ = Describe("OCI image layout users:", func() {
	tag := "e2e"
	When("pushing to registry without OCI artifact support", func() {
		statusKeys := []match.StateKey{
			foobar.ImageConfigStateKey("application/vnd.oci.empty.v1+json"),
			foobar.FileBarStateKey,
		}

		It("should push files without customized media types", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			// test
			ORAS("push", Flags.Layout, ref, foobar.FileBarName).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
		})

		It("should push files and tag", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			extraTag := "2e2"

			ORAS("push", Flags.Layout, fmt.Sprintf("%s,%s", ref, extraTag), foobar.FileBarName).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()

			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))

			fetched = ORAS("manifest", "fetch", Flags.Layout, LayoutRef(tempDir, extraTag)).Exec().Out.Contents()
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor("application/vnd.oci.image.layer.v1.tar")))
		})

		It("should push files with customized media types", func() {
			layerType := "layer.type"
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			ORAS("push", Flags.Layout, ref, foobar.FileBarName+":"+layerType).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers).Should(ContainElements(foobar.BlobBarDescriptor(layerType)))
		})

		It("should push files with manifest exported", func() {
			tempDir := PrepareTempFiles()
			layerType := "layer.type"
			exportPath := "packed.json"
			ref := LayoutRef(tempDir, tag)
			ORAS("push", ref, Flags.Layout, foobar.FileBarName+":"+layerType, "--export-manifest", exportPath).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", ref, Flags.Layout).Exec().Out.Contents()
			MatchFile(filepath.Join(tempDir, exportPath), string(fetched), DefaultTimeout)
		})

		It("should push files with customized config file", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			ORAS("push", Flags.Layout, ref, "--config", foobar.FileConfigName, foobar.FileBarName).
				MatchStatus([]match.StateKey{
					foobar.FileConfigStateKey,
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: "application/vnd.unknown.config.v1+json",
				Size:      int64(foobar.FileConfigSize),
				Digest:    foobar.FileConfigDigest,
			}))
		})

		It("should push files with customized config file and mediatype", func() {
			configType := "config/type"
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			ORAS("push", Flags.Layout, ref, "--config", fmt.Sprintf("%s:%s", foobar.FileConfigName, configType), foobar.FileBarName).
				MatchStatus([]match.StateKey{
					{Digest: foobar.FileConfigStateKey.Digest, Name: configType},
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: configType,
				Size:      int64(foobar.FileConfigSize),
				Digest:    foobar.FileConfigDigest,
			}))
		})

		It("should push files with platform", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			ORAS("push", Flags.Layout, ref, "--artifact-platform", "darwin/arm64", foobar.FileBarName).
				MatchStatus([]match.StateKey{
					foobar.PlatformConfigStateKey,
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: foobar.PlatformConfigStateKey.Name,
				Size:      int64(foobar.PlatformConfigSize),
				Digest:    foobar.PlatformConfigDigest,
			}))
			ORAS("pull", "--platform", "darwin/arm64", Flags.Layout, ref).MatchStatus([]match.StateKey{
				foobar.FileBarStateKey,
			}, true, 1).Exec()

		})

		It("should fail to customize config mediaType when baking config blob with platform for v1.0", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			ORAS("push", Flags.Layout, ref, "--image-spec", "v1.0", "--artifact-type", "test/artifact+json", "--artifact-platform", "darwin/arm64", foobar.FileBarName).
				ExpectFailure().
				Exec()
		})

		It("should push files with platform with no artifactType for v1.0", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			ORAS("push", Flags.Layout, ref, "--image-spec", "v1.0", "--artifact-platform", "darwin/arm64", foobar.FileBarName).
				MatchStatus([]match.StateKey{
					foobar.PlatformV1DEfaultConfigStateKey,
					foobar.FileBarStateKey,
				}, true, 2).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", Flags.Layout, ref).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Config).Should(Equal(ocispec.Descriptor{
				MediaType: "application/vnd.oci.image.config.v1+json",
				Size:      int64(foobar.PlatformV10ConfigSize),
				Digest:    foobar.PlatformV10ConfigDigest,
			}))
		})

		It("should push files with customized manifest annotation", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			key := "image-anno-key"
			value := "image-anno-value"
			// test
			ORAS("push", Flags.Layout, ref, foobar.FileBarName, "--annotation", fmt.Sprintf("%s=%s", key, value)).
				MatchStatus(statusKeys, true, len(statusKeys)).
				WithWorkDir(tempDir).Exec()
			// validate
			fetched := ORAS("manifest", "fetch", ref, Flags.Layout).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Annotations[key]).To(Equal(value))
		})

		It("should push files with customized file annotation", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			// test
			ORAS("push", ref, Flags.Layout, foobar.FileBarName, "--annotation-file", "foobar/annotation.json", "--config", foobar.FileConfigName).
				MatchStatus(statusKeys, true, 1).
				WithWorkDir(tempDir).Exec()

			// validate
			// see testdata\files\foobar\annotation.json
			fetched := ORAS("manifest", "fetch", ref, Flags.Layout).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Annotations["hi"]).To(Equal("manifest"))
			Expect(manifest.Config.Annotations["hello"]).To(Equal("config"))
			Expect(len(manifest.Layers)).To(Equal(1))
			Expect(manifest.Layers[0].Annotations["foo"]).To(Equal("bar"))
		})

		It("should allow renaming files using customized file annotation", func() {
			tempDir := PrepareTempFiles()
			ref := LayoutRef(tempDir, tag)
			// test
			ORAS("push", ref, Flags.Layout, foobar.FileBarName, "--annotation-file", "foobar/annotation-rename.json", "--config", foobar.FileConfigName).
				WithWorkDir(tempDir).Exec()

			// validate
			// see testdata\files\foobar\annotation-rename.json
			fetched := ORAS("manifest", "fetch", ref, Flags.Layout).Exec().Out.Contents()
			var manifest ocispec.Manifest
			Expect(json.Unmarshal(fetched, &manifest)).ShouldNot(HaveOccurred())
			Expect(manifest.Layers[0].Annotations["org.opencontainers.image.title"]).To(Equal("foobar/baz"))
		})
	})
})

var _ = Describe("Remote registry users:", func() {
	When("pushing recursively with --recursive flag", func() {
		tag := "recursive-e2e"

		// buildRecursiveDir creates a temp directory with a nested structure and
		// returns the path. The structure is:
		//   <tmp>/
		//     file1.txt
		//     file2.txt
		//     subdir/
		//       file3.txt
		//       nested/
		//         file4.txt
		buildRecursiveDir := func() (string, map[string]string) {
			srcDir := GinkgoT().TempDir()
			files := map[string]string{
				"file1.txt":               "root file one\n",
				"file2.txt":               "root file two\n",
				"subdir/file3.txt":        "subdir file three\n",
				"subdir/nested/file4.txt": "nested file four\n",
			}
			for rel, content := range files {
				full := filepath.Join(srcDir, rel)
				Expect(os.MkdirAll(filepath.Dir(full), 0755)).To(Succeed())
				Expect(os.WriteFile(full, []byte(content), 0644)).To(Succeed())
			}
			return srcDir, files
		}

		It("should push recursively and pull back with correct file contents", func() {
			// Regression test for https://github.com/oras-project/oras/pull/1951
			// where oras pull failed with "open <path>: is a directory" because
			// Image Index descriptors carried AnnotationTitle = directory name.
			repo := pushTestRepo("recursive-roundtrip")
			ref := RegistryRef(ZOTHost, repo, tag)

			srcDir, files := buildRecursiveDir()

			ORAS("push", "--recursive", ref, srcDir).
				WithDescription("recursive push of nested directory").
				Exec()

			dstDir := GinkgoT().TempDir()
			// The untitled child manifests must not be reported as skipped
			// layers; the pull must complete with the "Pulled" summary.
			ORAS("pull", ref).
				WithWorkDir(dstDir).
				WithDescription("pull after recursive push").
				MatchKeyWords("Pulled ", "Digest: ").
				Exec()

			for rel, want := range files {
				full := filepath.Join(dstDir, rel)
				got, err := os.ReadFile(full)
				Expect(err).NotTo(HaveOccurred(), "expected pulled file %q to exist", rel)
				Expect(string(got)).To(Equal(want), "content mismatch for %q", rel)
			}
		})

		It("should round-trip a flat directory without extra files", func() {
			// Regression test: the root manifest descriptor used to carry
			// AnnotationTitle = <directory basename>, so pulling a flat
			// directory wrote the manifest JSON as an extra file named after
			// the source directory.
			repo := pushTestRepo("recursive-flat-roundtrip")
			ref := RegistryRef(ZOTHost, repo, tag)

			srcDir := filepath.Join(GinkgoT().TempDir(), "src")
			Expect(os.MkdirAll(srcDir, 0755)).To(Succeed())
			// "src" collides with the source directory basename
			for _, name := range []string{"a", "src"} {
				Expect(os.WriteFile(filepath.Join(srcDir, name), []byte(name), 0644)).To(Succeed())
			}

			ORAS("push", "--recursive", ref, srcDir).
				WithDescription("recursive push of flat directory").
				Exec()

			dstDir := GinkgoT().TempDir()
			ORAS("pull", ref).
				WithWorkDir(dstDir).
				WithDescription("pull after flat recursive push").
				Exec()

			entries, err := os.ReadDir(dstDir)
			Expect(err).NotTo(HaveOccurred())
			var names []string
			for _, e := range entries {
				names = append(names, e.Name())
			}
			Expect(names).To(ConsistOf("a", "src"), "pull must restore exactly the pushed files")
			for _, name := range []string{"a", "src"} {
				got, err := os.ReadFile(filepath.Join(dstDir, name))
				Expect(err).NotTo(HaveOccurred())
				Expect(string(got)).To(Equal(name), "content mismatch for %q", name)
			}
		})

		It("should round-trip empty directories with --preserve-empty-dirs", func() {
			repo := pushTestRepo("recursive-empty-dirs")
			ref := RegistryRef(ZOTHost, repo, tag)

			srcDir := GinkgoT().TempDir()
			Expect(os.WriteFile(filepath.Join(srcDir, "a.txt"), []byte("a"), 0644)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(srcDir, "sub", "empty"), 0755)).To(Succeed())

			ORAS("push", "--recursive", "--preserve-empty-dirs", ref, srcDir).
				WithDescription("recursive push preserving empty directories").
				Exec()

			dstDir := GinkgoT().TempDir()
			ORAS("pull", ref).
				WithWorkDir(dstDir).
				WithDescription("pull after recursive push with empty directories").
				Exec()

			info, err := os.Stat(filepath.Join(dstDir, "sub", "empty"))
			Expect(err).NotTo(HaveOccurred(), "expected empty directory to be restored")
			Expect(info.IsDir()).To(BeTrue())
		})
	})
})
