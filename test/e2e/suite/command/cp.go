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
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/oras-project/oras-go/v3"
	"oras.land/oras/test/e2e/internal/testdata/artifact/blob"
	"oras.land/oras/test/e2e/internal/testdata/artifact/config"
	"oras.land/oras/test/e2e/internal/testdata/artifact/index"
	"oras.land/oras/test/e2e/internal/testdata/feature"
	"oras.land/oras/test/e2e/internal/testdata/foobar"
	ma "oras.land/oras/test/e2e/internal/testdata/multi_arch"
	. "oras.land/oras/test/e2e/internal/utils"
	"oras.land/oras/test/e2e/internal/utils/match"
)

func cpTestRepo(text string) string {
	return fmt.Sprintf("command/copy/%d/%s", GinkgoRandomSeed(), text)
}

var _ = Describe("ORAS beginners:", func() {
	When("running cp command", func() {
		It("should show help doc with feature flags", func() {
			out := ORAS("cp", "--help").MatchKeyWords("Copy", ExampleDesc).Exec().Out
			Expect(out).Should(gbytes.Say("--from-distribution-spec string\\s+%s", regexp.QuoteMeta(feature.Preview.Mark)))
			Expect(out).Should(gbytes.Say("--from-oci-layout-path string\\s+%s", regexp.QuoteMeta(feature.Experimental.Mark)))
			Expect(out).Should(gbytes.Say("-r, --recursive\\s+%s", regexp.QuoteMeta(feature.Preview.Mark)))
			Expect(out).Should(gbytes.Say("--to-distribution-spec string\\s+%s", regexp.QuoteMeta(feature.Preview.Mark)))
			Expect(out).Should(gbytes.Say("--to-oci-layout-path string\\s+%s", regexp.QuoteMeta(feature.Experimental.Mark)))
		})

		It("should not show --verbose in help doc", func() {
			out := ORAS("push", "--help").MatchKeyWords(ExampleDesc).Exec().Out
			gomega.Expect(out).ShouldNot(gbytes.Say("--verbose"))
		})

		It("should show deprecation message and print unnamed status output for --verbose", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, blob.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("test-verbose"), "copied")
			ORAS("cp", src, dst, "--verbose").
				MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).
				MatchStatus(blob.StateKeys, true, len(blob.StateKeys)).
				Exec()
			CompareRef(src, dst)
		})

		It("should show deprecation message and should NOT print unnamed status output for --verbose=false", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, blob.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("test-verbose-false"), "copied")
			stateKeys := []match.StateKey{
				{Digest: "2ef548696ac7", Name: "hello.tar"},
			}
			out := ORAS("cp", src, dst, "--verbose=false").
				MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).
				MatchStatus(stateKeys, false, len(stateKeys)).
				Exec().Out
			CompareRef(src, dst)
			// should not print status output for unnamed blobs
			gomega.Expect(out).ShouldNot(gbytes.Say("application/vnd.oci.empty.v1+json"))
			gomega.Expect(out).ShouldNot(gbytes.Say("application/vnd.oci.image.manifest.v1+json"))
		})

		It("should fail when no reference provided", func() {
			ORAS("cp").ExpectFailure().MatchErrKeyWords("Error:").Exec()
		})

		It("should fail when no destination reference provided", func() {
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag)).ExpectFailure().MatchErrKeyWords("Error:").Exec()
		})

		It("should fail when no tag or digest is provided for source target", func() {
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, ""), RegistryRef(ZOTHost, ImageRepo, "dst")).ExpectFailure().MatchErrKeyWords("Error:", "no tag or digest specified", "oras cp").Exec()
		})

		It("should fail when source doesn't exist", func() {
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, InvalidTag), RegistryRef(ZOTHost, cpTestRepo("nonexistent-source"), "")).ExpectFailure().MatchErrKeyWords(InvalidTag).Exec()
		})

		It("should fail and show detailed error description if no argument provided", func() {
			err := ORAS("cp").ExpectFailure().Exec().Err
			Expect(err).Should(gbytes.Say("Error"))
			Expect(err).Should(gbytes.Say("\nUsage: oras cp"))
			Expect(err).Should(gbytes.Say("\n"))
			Expect(err).Should(gbytes.Say(`Run "oras cp -h"`))
		})

		It("should fail and show detailed error description if more than 2 arguments are provided", func() {
			err := ORAS("cp", "foo", "bar", "buz").ExpectFailure().Exec().Err
			Expect(err).Should(gbytes.Say("Error"))
			Expect(err).Should(gbytes.Say("\nUsage: oras cp"))
			Expect(err).Should(gbytes.Say("\n"))
			Expect(err).Should(gbytes.Say(`Run "oras cp -h"`))
		})

		It("should fail and show registry error prefix if source not found", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, InvalidTag)
			dst := GinkgoT().TempDir()
			ORAS("cp", src, Flags.ToLayout, dst).MatchErrKeyWords(RegistryErrorPrefix).ExpectFailure().Exec()
		})

		It("should fail and show registry error prefix if destination registry is not logged in", func() {
			src := PrepareTempOCI(ArtifactRepo)
			dst := RegistryRef(ZOTHost, cpTestRepo("dest-not-logged-in"), "")
			ORAS("cp", Flags.FromLayout, LayoutRef(src, foobar.Tag), dst, "--to-username", Username, "--to-password", Password+"?").
				MatchErrKeyWords(fmt.Sprintf("Error from destination registry for %q", dst)).
				ExpectFailure().Exec()
		})

		It("should fail and show registry error prefix if source registry is not logged in", func() {
			src := RegistryRef(ZOTHost, cpTestRepo("src-not-logged-in"), foobar.Tag)
			dst := RegistryRef(ZOTHost, ArtifactRepo, "")
			ORAS("cp", src, dst, "--from-username", Username, "--from-password", Password+"?").
				MatchErrKeyWords(RegistryErrorPrefix).ExpectFailure().Exec()
		})

		It("should fail if basic auth flag is used with identity token flag", func() {
			src := RegistryRef(ZOTHost, cpTestRepo("conflicted-flags"), foobar.Tag)
			dst := RegistryRef(ZOTHost, ArtifactRepo, "")
			ORAS("cp", src, dst, "--from-username", Username, "--from-identity-token", "test-token").ExpectFailure().Exec()
		})
	})
})

var foobarStates = append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageConfigStateKey(oras.MediaTypeUnknownConfig))

func CompareRef(src, dst string) {
	srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
	dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
	Expect(srcManifest).To(Equal(dstManifest))
}

var _ = Describe("1.1 registry users:", func() {
	When("running `cp`", func() {
		It("should copy an artifact with blob", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, blob.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("artifact-with-blob"), "copied")
			ORAS("cp", src, dst).MatchStatus(blob.StateKeys, true, len(blob.StateKeys)).Exec()
			CompareRef(src, dst)
		})

		It("should copy and show logs with deduplicated id", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, blob.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("debug-log"), "copied")
			session := ORAS("cp", src, dst, "-d").Exec()
			Expect(session.Err).To(gbytes.Say("Request #0"))
			Expect(session.Err).NotTo(gbytes.Say("Request #0"))
			CompareRef(src, dst)
		})

		It("should copy an artifact with config", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, config.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("artifact-with-config"), "copied")
			ORAS("cp", src, dst).MatchStatus(config.StateKeys, true, len(config.StateKeys)).Exec()
		})

		It("should copy index and its subject", func() {
			stateKeys := append(ma.IndexStateKeys, index.ManifestStatusKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, index.ManifestDigest)
			dst := RegistryRef(ZOTHost, cpTestRepo("index-with-subject"), "")
			ORAS("cp", src, dst).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
		})

		It("should copy an image to a new repository via tag", func() {
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("tag"), "copied")
			ORAS("cp", src, dst).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			CompareRef(src, dst)
		})

		It("should copy an image to a new repository via digest", func() {
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Digest)
			dst := RegistryRef(ZOTHost, cpTestRepo("digest"), "copiedTag")
			ORAS("cp", src, dst).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			CompareRef(src, dst)
		})

		It("should copy an image to a new repository via tag without tagging", func() {
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("no-tagging"), foobar.Digest)
			ORAS("cp", src, dst).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			CompareRef(src, dst)
		})

		It("should copy an image and its referrers to a new repository", func() {
			stateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			src := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("referrers"), foobar.Digest)
			ORAS("cp", "-r", src, dst).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
			CompareRef(src, dst)
		})

		It("should copy a multi-arch image and its referrers to a new repository via tag", func() {
			stateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			dstRepo := cpTestRepo("index-referrers")
			dst := RegistryRef(ZOTHost, dstRepo, "copiedTag")
			ORAS("cp", src, dst, "-r").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.Digest).
				Exec()
			// validate
			CompareRef(RegistryRef(ZOTHost, ImageRepo, ma.Digest), dst)

			digests := ORAS("discover", dst, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for _, digest := range strings.Split(strings.TrimSpace(string(digests)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), RegistryRef(ZOTHost, dstRepo, digest))
			}
		})

		It("should copy a multi-arch image and its referrers without concurrency limitation", func() {
			stateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			dstRepo := cpTestRepo("index-referrers-concurrent")
			dst := RegistryRef(ZOTHost, dstRepo, "copiedTag")
			// test
			ORAS("cp", src, dst, "-r", "--concurrency", "0").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.Digest).
				Exec()
			// validate
			CompareRef(RegistryRef(ZOTHost, ImageRepo, ma.Digest), dst)
			digests := ORAS("discover", dst, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for _, digest := range strings.Split(strings.TrimSpace(string(digests)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), RegistryRef(ZOTHost, dstRepo, digest))
			}
		})

		It("should copy a multi-arch image, child images and referrers of the child images", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, "v1.3.8")
			dstRepo := cpTestRepo("index-without-referrers")
			dst := RegistryRef(ZOTHost, dstRepo, "copiedTag")
			stateKeys := []match.StateKey{
				{Digest: "44136fa355b3", Name: "application/vnd.oci.empty.v1+json"},
				{Digest: "01fa0c3558d5", Name: "arm64"},
				{Digest: "2960eae76dd7", Name: "amd64"},
				{Digest: "ab01d6e284e8", Name: "application/vnd.oci.image.manifest.v1+json"},
				{Digest: "6aa11331ce0c", Name: "application/vnd.oci.image.manifest.v1+json"},
				{Digest: "58e0d01dbd27", Name: "signature"},
				{Digest: "ecbd32686867", Name: "referrerimage"},
				{Digest: "02746a135c9e", Name: "sbom"},
				{Digest: "553c18eccc8b", Name: "application/vnd.oci.image.index.v1+json"},
			}
			// test
			ORAS("cp", src, dst, "-r").
				MatchStatus(stateKeys, true, len(stateKeys)).
				Exec()
			// validate that the index is copied
			CompareRef(src, dst)
			// validate that the child images are copied
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, "sha256:ab01d6e284e843d51fb5e753904a540f507a62361a5fd7e434e4f27b285ca5c9"), RegistryRef(ZOTHost, dstRepo, "sha256:ab01d6e284e843d51fb5e753904a540f507a62361a5fd7e434e4f27b285ca5c9"))
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, "sha256:6aa11331ce0c766d6333b60dac98d584d98eea45fa93bbfc9b5bdb915ce3a43f"), RegistryRef(ZOTHost, dstRepo, "sha256:6aa11331ce0c766d6333b60dac98d584d98eea45fa93bbfc9b5bdb915ce3a43f"))
			// validate that the referrers of the child images are copied
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, "sha256:359bac7f6a262e0f36e83b6b78ee3cc7a0bb8813e04d330328ca7ca9785e1e0b"), RegistryRef(ZOTHost, dstRepo, "sha256:359bac7f6a262e0f36e83b6b78ee3cc7a0bb8813e04d330328ca7ca9785e1e0b"))
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, "sha256:938419ae89a9947476bbed93abc5eb7abf7d5708be69679fe6cc4b22afe8fdd5"), RegistryRef(ZOTHost, dstRepo, "sha256:938419ae89a9947476bbed93abc5eb7abf7d5708be69679fe6cc4b22afe8fdd5"))
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, "sha256:20e7d3a6ce087c54238c18a3428853b50cdaf4478a9d00caa8304119b58ae8a9"), RegistryRef(ZOTHost, dstRepo, "sha256:20e7d3a6ce087c54238c18a3428853b50cdaf4478a9d00caa8304119b58ae8a9"))
		})

		It("should copy an empty index", func() {
			src := RegistryRef(ZOTHost, ImageRepo, ma.EmptyTag)
			dstRepo := cpTestRepo("empty-index")
			dst := RegistryRef(ZOTHost, dstRepo, "copiedTag")
			// test
			ORAS("cp", src, dst, "-r", "--concurrency", "0").Exec()
			// validate
			CompareRef(RegistryRef(ZOTHost, ImageRepo, ma.EmptyTag), dst)
		})

		It("should copy a multi-arch image and its referrers to a new repository via digest", func() {
			stateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			dstRepo := cpTestRepo("index-referrers-digest")
			dst := RegistryRef(ZOTHost, dstRepo, ma.Digest)
			ORAS("cp", src, dst, "-r").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.Digest).
				Exec()
			// validate
			CompareRef(RegistryRef(ZOTHost, ImageRepo, ma.Digest), dst)
			digests := ORAS("discover", dst, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for _, digest := range strings.Split(strings.TrimSpace(string(digests)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), RegistryRef(ZOTHost, dstRepo, digest))
			}
		})

		It("should copy a certain platform of image to a new repository via tag", func() {
			src := RegistryRef(ZOTHost, ImageRepo, ma.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("platform-tag"), "copiedTag")

			ORAS("cp", src, dst, "--platform", "linux/amd64").
				MatchStatus(ma.LinuxAMD64StateKeys, true, len(ma.LinuxAMD64StateKeys)).
				MatchKeyWords("Digest: " + ma.LinuxAMD64.Digest.String()).
				Exec()
			CompareRef(RegistryRef(ZOTHost, ImageRepo, ma.LinuxAMD64.Digest.String()), dst)
		})

		It("should copy a certain platform of image to a new repository via digest", func() {
			src := RegistryRef(ZOTHost, ImageRepo, ma.Digest)
			dstRepo := cpTestRepo("platform-digest")
			dst := RegistryRef(ZOTHost, dstRepo, "")
			ORAS("cp", src, dst, "--platform", "linux/amd64").
				MatchStatus(ma.LinuxAMD64StateKeys, true, len(ma.LinuxAMD64StateKeys)).
				MatchKeyWords("Digest: " + ma.LinuxAMD64.Digest.String()).
				Exec()
			CompareRef(RegistryRef(ZOTHost, ImageRepo, ma.LinuxAMD64.Digest.String()), RegistryRef(ZOTHost, dstRepo, ma.LinuxAMD64.Digest.String()))
		})

		It("should copy a certain platform of image and its referrers to a new repository with tag", func() {
			stateKeys := append(ma.LinuxAMD64StateKeys, ma.LinuxAMD64ReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			dstRepo := cpTestRepo("platform-referrers")
			dst := RegistryRef(ZOTHost, dstRepo, "copiedTag")
			digest := ma.LinuxAMD64.Digest.String()
			ORAS("cp", src, dst, "-r", "--platform", "linux/amd64").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + digest).
				Exec()
			// validate
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), dst)
			digests := ORAS("discover", dst, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for _, digest := range strings.Split(strings.TrimSpace(string(digests)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), RegistryRef(ZOTHost, dstRepo, digest))
			}
		})

		It("should copy a certain platform of image and its referrers to a new repository without tagging", func() {
			stateKeys := append(ma.LinuxAMD64StateKeys, ma.LinuxAMD64ReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			dstRepo := cpTestRepo("platform-referrers-no-tag")
			dst := RegistryRef(ZOTHost, dstRepo, "")
			digest := ma.LinuxAMD64.Digest.String()
			ORAS("cp", src, dst, "-r", "--platform", "linux/amd64").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + digest).
				Exec()
			// validate
			CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), RegistryRef(ZOTHost, dstRepo, digest))
			digests := ORAS("discover", RegistryRef(ZOTHost, dstRepo, digest), "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for _, digest := range strings.Split(strings.TrimSpace(string(digests)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, digest), RegistryRef(ZOTHost, dstRepo, digest))
			}
		})

		It("should copy an image to a new repository with multiple tagging", func() {
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Digest)
			tags := []string{"tag1", "tag2", "tag3"}
			dstRepo := cpTestRepo("multi-tagging")
			dst := RegistryRef(ZOTHost, dstRepo, "")
			ORAS("cp", src, dst+":"+strings.Join(tags, ",")).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			for _, tag := range tags {
				dst := RegistryRef(ZOTHost, dstRepo, tag)
				CompareRef(src, dst)
			}
		})

		It("should copy a multi-arch image to a partially populated destination with --force", func() {
			// Reproduce the pull-through-cache failure mode: the
			// destination already advertises the index manifest as
			// present (Exists(index)=true) while one or more referenced
			// platform manifests are actually missing. In that state the
			// default copy short-circuits the sub-DAG and the missing
			// platforms are never pushed. With --force, cp must walk
			// every referenced manifest and push the missing ones before
			// updating the tag.
			//
			// Conformant registries reject indices whose children aren't
			// present, so the partial state can't be produced over the
			// distribution API. Use an OCI image layout as the
			// destination instead: it stores manifests as plain files,
			// which lets us delete two child manifests on disk and mint
			// the exact "index known, children missing" state that
			// pull-through caches expose. The --force code path under
			// test (wrapping the destination in a
			// contentutil.TraversingTarget) is identical for layout and
			// registry targets.
			src := RegistryRef(ZOTHost, ImageRepo, ma.Tag)
			dstDir := GinkgoT().TempDir()
			tag := "copiedTag"
			dst := LayoutRef(dstDir, tag)

			// Fully populate the layout, then physically remove two
			// child manifest blobs so the index advertises them while
			// they are absent on disk.
			ORAS("cp", src, dst, Flags.ToLayout).Exec()
			for _, child := range []string{ma.LinuxARM64.Digest.String(), ma.LinuxARMV7.Digest.String()} {
				algo, hex, ok := strings.Cut(child, ":")
				Expect(ok).To(BeTrue())
				Expect(os.Remove(filepath.Join(dstDir, "blobs", algo, hex))).To(Succeed())
			}
			missingARM64 := LayoutRef(dstDir, ma.LinuxARM64.Digest.String())
			missingARMv7 := LayoutRef(dstDir, ma.LinuxARMV7.Digest.String())
			ORAS("manifest", "fetch", Flags.Layout, missingARM64).ExpectFailure().Exec()
			ORAS("manifest", "fetch", Flags.Layout, missingARMv7).ExpectFailure().Exec()

			// Negative control: without --force, cp short-circuits on
			// Exists(index)=true, skips the sub-DAG, and leaves the
			// missing platforms absent.
			ORAS("cp", src, dst, Flags.ToLayout).Exec()
			ORAS("manifest", "fetch", Flags.Layout, missingARM64).ExpectFailure().Exec()
			ORAS("manifest", "fetch", Flags.Layout, missingARMv7).ExpectFailure().Exec()

			// With --force, cp performs a deep traversal so the missing
			// platforms get pushed before the tag is updated.
			ORAS("cp", src, dst, Flags.ToLayout, "--force").Exec()

			// The index in the layout must match the source.
			srcIndex := ORAS("manifest", "fetch", RegistryRef(ZOTHost, ImageRepo, ma.Digest)).Exec().Out.Contents()
			dstIndex := ORAS("manifest", "fetch", Flags.Layout, dst).Exec().Out.Contents()
			Expect(srcIndex).To(Equal(dstIndex))
			// The previously-missing platform manifests must be present.
			srcARM64 := ORAS("manifest", "fetch", RegistryRef(ZOTHost, ImageRepo, ma.LinuxARM64.Digest.String())).Exec().Out.Contents()
			dstARM64 := ORAS("manifest", "fetch", Flags.Layout, missingARM64).Exec().Out.Contents()
			Expect(srcARM64).To(Equal(dstARM64))
			srcARMv7 := ORAS("manifest", "fetch", RegistryRef(ZOTHost, ImageRepo, ma.LinuxARMV7.Digest.String())).Exec().Out.Contents()
			dstARMv7 := ORAS("manifest", "fetch", Flags.Layout, missingARMv7).Exec().Out.Contents()
			Expect(srcARMv7).To(Equal(dstARMv7))
		})
	})
})

var _ = Describe("OCI spec 1.0 registry users:", func() {
	When("running `cp`", func() {
		It("should copy an image artifact with mounting", func() {
			repo := cpTestRepo("1.0-mount")
			src := RegistryRef(FallbackHost, ArtifactRepo, foobar.Tag)
			dst := RegistryRef(FallbackHost, repo, "")
			out := ORAS("cp", src, dst).Exec()
			Expect(out).Should(gbytes.Say("Mounted fcde2b2edba5 bar"))
			CompareRef(src, RegistryRef(FallbackHost, repo, foobar.Digest))
		})

		It("should copy an image artifact and its referrers from a registry to a fallback registry", func() {
			repo := cpTestRepo("to-fallback")
			stateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			src := RegistryRef(ZOTHost, ArtifactRepo, foobar.SignatureImageReferrer.Digest.String())
			dst := RegistryRef(FallbackHost, repo, "")
			ORAS("cp", "-r", src, dst).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
			CompareRef(src, RegistryRef(FallbackHost, repo, foobar.SignatureImageReferrer.Digest.String()))
			ORAS("discover", "-o", "tree", RegistryRef(FallbackHost, repo, foobar.Digest)).
				WithDescription("discover referrer via subject").MatchKeyWords(foobar.SignatureImageReferrer.Digest.String(), foobar.SBOMImageReferrer.Digest.String()).Exec()
		})
		It("should copy an image artifact and its referrers from a fallback registry to a registry", func() {
			repo := cpTestRepo("from-fallback")
			stateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			src := RegistryRef(FallbackHost, ArtifactRepo, foobar.SBOMImageReferrer.Digest.String())
			dst := RegistryRef(ZOTHost, repo, "")
			ORAS("cp", "-r", src, dst).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
			CompareRef(src, RegistryRef(ZOTHost, repo, foobar.SBOMImageReferrer.Digest.String()))
			ORAS("discover", "-o", "tree", RegistryRef(ZOTHost, repo, foobar.Digest)).
				WithDescription("discover referrer via subject").MatchKeyWords(foobar.SignatureImageReferrer.Digest.String(), foobar.SBOMImageReferrer.Digest.String()).Exec()
		})

		It("should copy an image from a fallback registry to an OCI image layout via digest", func() {
			dstDir := GinkgoT().TempDir()
			src := RegistryRef(FallbackHost, ArtifactRepo, foobar.Tag)
			ORAS("cp", src, dstDir, Flags.ToLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", LayoutRef(dstDir, foobar.Digest), Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from an OCI image layout to a fallback registry via digest", func() {
			layoutDir := GinkgoT().TempDir()
			src := LayoutRef(layoutDir, foobar.Digest)
			dst := RegistryRef(FallbackHost, cpTestRepo("from-layout-digest"), "copied")
			// prepare
			ORAS("cp", RegistryRef(FallbackHost, ArtifactRepo, foobar.Tag), layoutDir, Flags.ToLayout).Exec()
			// test
			ORAS("cp", src, dst, Flags.FromLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy a certain platform of image and its referrers from an OCI image layout to a fallback registry", func() {
			type discover struct {
				ocispec.Descriptor
				Referrers []ocispec.Descriptor
			}
			stateKeys := append(ma.LinuxAMD64StateKeys, ma.LinuxAMD64ReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			fromDir := GinkgoT().TempDir()
			src := LayoutRef(fromDir, ma.Tag)
			dstRepo := cpTestRepo("platform-referrer-from-layout")
			dst := RegistryRef(FallbackHost, dstRepo, "copied")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, ma.Tag), src, Flags.ToLayout, "-r").Exec()
			ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, ma.Tag), src, Flags.ToLayout, "-r", "--platform", "linux/amd64").Exec()
			// test
			ORAS("cp", src, Flags.FromLayout, dst, "-r", "--platform", "linux/amd64").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.LinuxAMD64.Digest.String()).
				Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout, "--platform", "linux/amd64").WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
			ORAS("manifest", "fetch", RegistryRef(FallbackHost, dstRepo, ma.Digest)).WithDescription("not copy index").ExpectFailure().Exec()
			var disv discover
			bytes := ORAS("discover", dst, "-o", "json").
				MatchKeyWords(ma.LinuxAMD64Referrer.Digest.String()).
				WithDescription("copy image referrer").
				Exec().Out.Contents()
			Expect(json.Unmarshal(bytes, &disv)).ShouldNot(HaveOccurred())
			Expect(len(disv.Referrers)).To(Equal(1))
			Expect(disv.Referrers[0].Digest.String()).To(Equal(ma.LinuxAMD64Referrer.Digest.String()))
		})
	})
})

var _ = Describe("OCI layout users:", func() {
	When("running `cp`", func() {
		type discover struct {
			ocispec.Descriptor
			Referrers []ocispec.Descriptor
		}
		It("should copy an image from a registry to an OCI image layout via tag", func() {
			dst := LayoutRef(GinkgoT().TempDir(), "copied")
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)
			ORAS("cp", src, dst, Flags.ToLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from an OCI image layout to a registry via tag", func() {
			layoutDir := GinkgoT().TempDir()
			src := LayoutRef(layoutDir, "copied")
			dst := RegistryRef(ZOTHost, cpTestRepo("from-layout-tag"), foobar.Tag)
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), src, Flags.ToLayout).Exec()
			// test
			ORAS("cp", src, dst, Flags.FromLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image between OCI image layouts via tag", func() {
			srcDir := GinkgoT().TempDir()
			toDir := GinkgoT().TempDir()
			src := LayoutRef(srcDir, "from")
			dst := LayoutRef(toDir, "to")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), src, Flags.ToLayout).Exec()
			// test
			ORAS("cp", src, dst, Flags.FromLayout, Flags.ToLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from a registry to an OCI image layout via digest", func() {
			dstDir := GinkgoT().TempDir()
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Digest)
			ORAS("cp", src, dstDir, Flags.ToLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", LayoutRef(dstDir, foobar.Digest), Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from an OCI image layout to a registry via digest", func() {
			layoutDir := GinkgoT().TempDir()
			src := LayoutRef(layoutDir, foobar.Digest)
			dst := RegistryRef(ZOTHost, cpTestRepo("from-layout-digest"), "copied")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), layoutDir, Flags.ToLayout).Exec()
			// test
			ORAS("cp", src, dst, Flags.FromLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image between OCI image layouts via digest", func() {
			srcDir := GinkgoT().TempDir()
			toDir := GinkgoT().TempDir()
			src := LayoutRef(srcDir, foobar.Digest)
			dst := LayoutRef(toDir, foobar.Digest)
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), srcDir, Flags.ToLayout).Exec()
			// test
			ORAS("cp", src, toDir, Flags.FromLayout, Flags.ToLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from a registry to an OCI image layout with multiple tagging", func() {
			dstDir := GinkgoT().TempDir()
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)
			tags := []string{"tag1", "tag2", "tag3"}
			// test
			ORAS("cp", src, dstDir+":"+strings.Join(tags, ","), Flags.ToLayout).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			for _, tag := range tags {
				dstManifest := ORAS("manifest", "fetch", LayoutRef(dstDir, tag), Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
				Expect(srcManifest).To(Equal(dstManifest))
			}
		})

		It("should copy a tagged image and its referrers from a registry to an OCI image layout", func() {
			stateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			dst := LayoutRef(GinkgoT().TempDir(), "copied")
			src := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)
			// test
			ORAS("cp", "-r", src, dst, Flags.ToLayout).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy a image and its referrers from a registry to an OCI image layout via digest", func() {
			stateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			toDir := GinkgoT().TempDir()
			src := RegistryRef(ZOTHost, ArtifactRepo, foobar.Digest)
			// test
			ORAS("cp", "-r", src, toDir, Flags.ToLayout).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", LayoutRef(toDir, foobar.Digest), Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy a multi-arch image and its referrers from a registry to an OCI image layout a via tag", func() {
			stateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			toDir := GinkgoT().TempDir()
			dst := LayoutRef(toDir, "copied")
			// test
			ORAS("cp", src, Flags.ToLayout, dst, "-r").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.Digest).
				Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
			var disv discover
			bytes := ORAS("discover", dst, "-o", "json", Flags.Layout, "--artifact-type", ma.IndexReferrerConfigStateKey.Name).
				// MatchKeyWords(ma.IndexReferrerDigest).
				WithDescription("copy image referrer").
				Exec().Out.Contents()
			Expect(json.Unmarshal(bytes, &disv)).ShouldNot(HaveOccurred())
			Expect(len(disv.Referrers)).To(Equal(1))
			Expect(disv.Referrers[0].Digest.String()).To(Equal(ma.IndexReferrerDigest))
			ORAS("manifest", "fetch", Flags.Layout, LayoutRef(toDir, ma.LinuxAMD64Referrer.Digest.String())).
				WithDescription("copy referrer of successor").
				Exec()
		})

		It("should copy a multi-arch image and its referrers from an OCI image layout to a registry via digest", func() {
			stateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			fromDir := GinkgoT().TempDir()
			src := LayoutRef(fromDir, ma.Tag)
			dst := RegistryRef(ZOTHost, cpTestRepo("recursive-from-layout"), "copied")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, ma.Tag), src, Flags.ToLayout, "-r").Exec()
			// test
			ORAS("cp", src, Flags.FromLayout, dst, "-r").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.Digest).
				Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
			var disv discover
			bytes := ORAS("discover", dst, "-o", "json", "--artifact-type", ma.IndexReferrerConfigStateKey.Name).
				MatchKeyWords(ma.IndexReferrerDigest).
				WithDescription("copy image referrer").
				Exec().Out.Contents()
			Expect(json.Unmarshal(bytes, &disv)).ShouldNot(HaveOccurred())
			Expect(len(disv.Referrers)).To(Equal(1))
			Expect(disv.Referrers[0].Digest.String()).To(Equal(ma.IndexReferrerDigest))
			ORAS("manifest", "fetch", dst).
				WithDescription("copy referrer of successor").
				Exec()
		})

		It("should copy a certain platform of image and its referrers from an OCI image layout to a registry", func() {
			stateKeys := append(ma.LinuxAMD64StateKeys, ma.LinuxAMD64ReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			fromDir := GinkgoT().TempDir()
			src := LayoutRef(fromDir, ma.Tag)
			dstRepo := cpTestRepo("platform-referrer-from-layout")
			dst := RegistryRef(ZOTHost, dstRepo, "copied")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, ma.Tag), src, Flags.ToLayout, "-r").Exec()
			ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, ma.Tag), src, Flags.ToLayout, "-r", "--platform", "linux/amd64").Exec()
			// test
			ORAS("cp", src, Flags.FromLayout, dst, "-r", "--platform", "linux/amd64").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.LinuxAMD64.Digest.String()).
				Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout, "--platform", "linux/amd64").WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
			ORAS("manifest", "fetch", RegistryRef(ZOTHost, dstRepo, ma.Digest)).WithDescription("not copy index").ExpectFailure().Exec()
			var disv discover
			bytes := ORAS("discover", dst, "-o", "json").
				MatchKeyWords(ma.LinuxAMD64Referrer.Digest.String()).
				WithDescription("copy image referrer").
				Exec().Out.Contents()
			Expect(json.Unmarshal(bytes, &disv)).ShouldNot(HaveOccurred())
			Expect(len(disv.Referrers)).To(Equal(1))
			Expect(disv.Referrers[0].Digest.String()).To(Equal(ma.LinuxAMD64Referrer.Digest.String()))
		})

		It("should copy a multi-arch image, child images and referrers of the child images from an OCI layout", func() {
			fromDir := GinkgoT().TempDir()
			toDir := GinkgoT().TempDir()
			src := LayoutRef(fromDir, ma.Tag)
			dst := LayoutRef(toDir, "copiedIndex")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, ma.Tag), src, Flags.ToLayout, "-r").Exec()
			// test
			ORAS("cp", src, Flags.FromLayout, dst, Flags.ToLayout, "-r").Exec()
			// validate
			// verify that the index "multi" is copied
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
			// verify that "multi"'s referrers are copied
			ORAS("discover", dst, Flags.Layout).MatchKeyWords("sha256:d37baf66300b9006b0f4c7102075d56b970fbf910be5c6bca07fdbb000dfa383", "sha256:7679bc22c33b87aa345c6950a993db98a6df7a6cc77a35c388908a3a50be6bad").Exec()
			// verify that the child images are copied
			ORAS("manifest", "fetch", Flags.Layout, LayoutRef(toDir, "sha256:9d84a5716c66a1d1b9c13f8ed157ba7d1edfe7f9b8766728b8a1f25c0d9c14c1")).Exec()
			ORAS("manifest", "fetch", Flags.Layout, LayoutRef(toDir, "sha256:4f93460061882467e6fb3b772dc6ab72130d9ac1906aed2fc7589a5cd145433c")).Exec()
			ORAS("manifest", "fetch", Flags.Layout, LayoutRef(toDir, "sha256:58efe73e78fe043ca31b89007a025c594ce12aa7e6da27d21c7b14b50112e255")).Exec()
			// verify that the referrers of the child images are copied
			ORAS("discover", Flags.Layout, LayoutRef(toDir, "sha256:9d84a5716c66a1d1b9c13f8ed157ba7d1edfe7f9b8766728b8a1f25c0d9c14c1")).MatchKeyWords("c5e00045954a70e3fd28307dd543d4cc158946117943700b8f520f72ddca031f").Exec()
		})

		It("should copy a certain platform of image and its referrers from a registry to an OCI image layout", func() {
			stateKeys := append(ma.LinuxAMD64StateKeys, ma.LinuxAMD64ReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			src := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)
			toDir := GinkgoT().TempDir()
			dst := LayoutRef(toDir, "copied")
			// test
			ORAS("cp", src, Flags.ToLayout, dst, "-r", "--platform", "linux/amd64").
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Digest: " + ma.LinuxAMD64.Digest.String()).
				Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, "--platform", "linux/amd64").WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
			ORAS("manifest", "fetch", LayoutRef(toDir, ma.Digest)).WithDescription("not copy index").ExpectFailure().Exec()
			var disv discover
			bytes := ORAS("discover", dst, "-o", "json", Flags.Layout).
				MatchKeyWords(ma.LinuxAMD64Referrer.Digest.String()).
				WithDescription("copy image referrer").
				Exec().Out.Contents()
			Expect(json.Unmarshal(bytes, &disv)).ShouldNot(HaveOccurred())
			Expect(len(disv.Referrers)).To(Equal(1))
			Expect(disv.Referrers[0].Digest.String()).To(Equal(ma.LinuxAMD64Referrer.Digest.String()))
		})

		// oci-layout-path tests

		It("should copy an image from a registry to an OCI image layout via tag using --oci-layout-path", func() {
			layoutDir := GinkgoT().TempDir()
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)
			ref := "copied"
			dst := LayoutRef(layoutDir, ref)
			ORAS("cp", src, ref, Flags.ToLayoutPath, layoutDir).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from an OCI image layout to a registry via tag using --oci-layout-path", func() {
			layoutDir := GinkgoT().TempDir()
			ref := "copied"
			src := LayoutRef(layoutDir, ref)
			dst := RegistryRef(ZOTHost, cpTestRepo("from-layout-tag-path"), foobar.Tag)
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), src, Flags.ToLayout).Exec()
			// test
			ORAS("cp", ref, dst, Flags.FromLayoutPath, layoutDir).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image between OCI image layouts via tag using --oci-layout-path", func() {
			srcDir := GinkgoT().TempDir()
			toDir := GinkgoT().TempDir()
			srcRef := "from"
			dstRef := "to"
			src := LayoutRef(srcDir, srcRef)
			dst := LayoutRef(toDir, dstRef)
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), src, Flags.ToLayout).Exec()
			// test
			ORAS("cp", srcRef, dstRef, Flags.FromLayoutPath, srcDir, Flags.ToLayoutPath, toDir).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from a registry to an OCI image layout via digest using --oci-layout-path", func() {
			dstDir := GinkgoT().TempDir()
			src := RegistryRef(ZOTHost, ImageRepo, foobar.Digest)
			ORAS("cp", src, foobar.Digest, Flags.ToLayoutPath, dstDir).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", LayoutRef(dstDir, foobar.Digest), Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image from an OCI image layout to a registry via digest using --oci-layout-path", func() {
			layoutDir := GinkgoT().TempDir()
			src := LayoutRef(layoutDir, foobar.Digest)
			dst := RegistryRef(ZOTHost, cpTestRepo("from-layout-digest-path"), "copied")
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), layoutDir, Flags.ToLayout).Exec()
			// test
			ORAS("cp", foobar.Digest, dst, Flags.FromLayoutPath, layoutDir).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})

		It("should copy an image between OCI image layouts via digest", func() {
			srcDir := GinkgoT().TempDir()
			toDir := GinkgoT().TempDir()
			src := LayoutRef(srcDir, foobar.Digest)
			dst := LayoutRef(toDir, foobar.Digest)
			// prepare
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), srcDir, Flags.ToLayout).Exec()
			// test
			ORAS("cp", foobar.Digest, foobar.Digest, Flags.FromLayoutPath, srcDir, Flags.ToLayoutPath, toDir).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
			// validate
			srcManifest := ORAS("manifest", "fetch", src, Flags.Layout).WithDescription("fetch from source to validate").Exec().Out.Contents()
			dstManifest := ORAS("manifest", "fetch", dst, Flags.Layout).WithDescription("fetch from destination to validate").Exec().Out.Contents()
			Expect(srcManifest).To(Equal(dstManifest))
		})
	})
})

var _ = Describe("OCI image spec v1.1.0-rc2 artifact users:", func() {
	It("should copy an image and its referrers to a new repository", func() {
		stateKeys := append(foobarStates, foobar.ArtifactReferrerStateKeys...)
		digest := foobar.SBOMArtifactReferrer.Digest.String()
		src := RegistryRef(Host, ArtifactRepo, digest)
		dst := RegistryRef(Host, cpTestRepo("referrers"), digest)
		ORAS("cp", "-r", src, dst).MatchStatus(stateKeys, true, len(stateKeys)).Exec()
		CompareRef(src, dst)
	})
})
