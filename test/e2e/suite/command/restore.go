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
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"

	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/oras-project/oras-go/v3"
	"oras.land/oras/test/e2e/internal/testdata/feature"
	"oras.land/oras/test/e2e/internal/testdata/foobar"
	ma "oras.land/oras/test/e2e/internal/testdata/multi_arch"
	. "oras.land/oras/test/e2e/internal/utils"
	"oras.land/oras/test/e2e/internal/utils/match"
)

func restoreTestRepo(text string) string {
	return fmt.Sprintf("command/restore/%d/%s", GinkgoRandomSeed(), text)
}

var _ = Describe("ORAS beginners:", func() {
	When("running restore command", func() {
		It("should show help description with experimental flag", func() {
			out := ORAS("restore", "--help").MatchKeyWords(ExampleDesc).Exec().Out
			gomega.Expect(out).Should(gbytes.Say(regexp.QuoteMeta(feature.Experimental.Mark)))
		})

		It("should fail when no reference provided", func() {
			ORAS("restore").ExpectFailure().MatchErrKeyWords("Error:").Exec()
		})

		It("should fail when no input path provided", func() {
			ORAS("restore", RegistryRef(ZOTHost, ImageRepo, foobar.Tag)).ExpectFailure().
				MatchErrKeyWords("Error:", `required flag(s) "input" not set`).Exec()
		})

		It("should fail when the input path is empty", func() {
			ORAS("restore", "--input", "", RegistryRef(ZOTHost, ImageRepo, foobar.Tag)).ExpectFailure().
				MatchErrKeyWords("Error:", `the input path cannot be empty`).Exec()
		})

		It("should fail when input doesn't exist", func() {
			nonExistentPath := filepath.Join(GinkgoT().TempDir(), "non-existent-path")
			ORAS("restore", "--input", nonExistentPath, RegistryRef(ZOTHost, ImageRepo, foobar.Tag)).ExpectFailure().
				MatchErrKeyWords("Error:").Exec()
		})

		It("should fail with appropriate error when digest is provided", func() {
			tmpDir := GinkgoT().TempDir()
			ORAS("restore", "--input", tmpDir, "invalid/format@sha256:digest").ExpectFailure().
				MatchErrKeyWords("Error:", "digest references are not supported").Exec()
		})

		It("should fail with appropriate error when invalid tag format provided", func() {
			tmpDir := GinkgoT().TempDir()
			ORAS("restore", "--input", tmpDir, "localhost:5000/repo:invalid+tag").ExpectFailure().
				MatchErrKeyWords("Error:").Exec()
		})

		It("should fail with appropriate error when tag is provided with digest", func() {
			tmpDir := GinkgoT().TempDir()
			ORAS("restore", "--input", tmpDir, "invalid/format:v1,@sha256:123abc").ExpectFailure().
				MatchErrKeyWords("Error:", "digest references are not supported").Exec()
		})
	})
})

var _ = Describe("ORAS users:", func() {
	When("restoring a single tag", func() {
		It("should successfully restore an image without referrers from a directory", func() {
			// Prepare a backup to restore from
			tmpDir := GinkgoT().TempDir()
			srcRef := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)
			backupDir := filepath.Join(tmpDir, "backup-single-tag")

			ORAS("backup", "--output", backupDir, srcRef).Exec()

			// Prepare target repo for restore
			testRepo := restoreTestRepo("restore-single-tag-dir")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			// Restore from backup directory
			foobarStates := append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageConfigStateKey(oras.MediaTypeUnknownConfig))
			ORAS("restore", "--input", backupDir, Flags.ExcludeReferrers, dstRef).
				MatchStatus(foobarStates, true, len(foobarStates)).
				MatchKeyWords("0 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)
		})

		It("should restore an artifact with its referrers from a directory", func() {
			// Create a backup with referrers
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-with-referrers")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-with-referrers")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			// Restore from backup with referrers
			foobarStates := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)

			ORAS("restore", "--input", backupDir, dstRef).
				MatchStatus(foobarStates, true, len(foobarStates)).
				MatchKeyWords("2 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)

			// Verify referrers were restored
			referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, referrerDgst), RegistryRef(ZOTHost, testRepo, referrerDgst))
			}
		})

		It("should successfully restore a multi-arch artifact without referrers from a directory", func() {
			// Create a backup to restore from
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-multi-arch")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)

			ORAS("backup", "--output", backupDir, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-multi-arch")
			dstRef := RegistryRef(ZOTHost, testRepo, ma.Tag)

			// Restore from backup directory
			stateKeys := ma.IndexStateKeys

			ORAS("restore", "--input", backupDir, Flags.ExcludeReferrers, dstRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("0 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)
		})

		It("should successfully restore a multi-arch artifact with referrers from a directory", func() {
			// Create a backup with multi-arch and referrers
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-multi-arch-referrers")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-multi-arch-referrers")
			dstRef := RegistryRef(ZOTHost, testRepo, ma.Tag)

			// Restore multi-arch with referrers
			stateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)

			ORAS("restore", "--input", backupDir, dstRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("3 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)

			// Verify referrers were restored
			referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, referrerDgst), RegistryRef(ZOTHost, testRepo, referrerDgst))
			}
		})

		It("should restore a multi-arch image with child images and referrers of the child images", func() {
			// Create a backup with a multi-arch image that has no direct referrers
			// but has child images with referrers
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-multi-arch-child-referrers")
			tag := "v1.3.8"
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, tag)

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-multi-arch-child-referrers")
			dstRef := RegistryRef(ZOTHost, testRepo, tag)

			// Restore from backup directory
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

			ORAS("restore", "--input", backupDir, dstRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("3 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			// validate that the index is restored correctly
			CompareRef(srcRef, dstRef)

			// validate that the child images are restored correctly
			CompareRef(
				RegistryRef(ZOTHost, ArtifactRepo, "sha256:ab01d6e284e843d51fb5e753904a540f507a62361a5fd7e434e4f27b285ca5c9"),
				RegistryRef(ZOTHost, testRepo, "sha256:ab01d6e284e843d51fb5e753904a540f507a62361a5fd7e434e4f27b285ca5c9"),
			)
			CompareRef(
				RegistryRef(ZOTHost, ArtifactRepo, "sha256:6aa11331ce0c766d6333b60dac98d584d98eea45fa93bbfc9b5bdb915ce3a43f"),
				RegistryRef(ZOTHost, testRepo, "sha256:6aa11331ce0c766d6333b60dac98d584d98eea45fa93bbfc9b5bdb915ce3a43f"),
			)

			// validate that the referrers of the child images are restored correctly
			CompareRef(
				RegistryRef(ZOTHost, ArtifactRepo, "sha256:359bac7f6a262e0f36e83b6b78ee3cc7a0bb8813e04d330328ca7ca9785e1e0b"),
				RegistryRef(ZOTHost, testRepo, "sha256:359bac7f6a262e0f36e83b6b78ee3cc7a0bb8813e04d330328ca7ca9785e1e0b"),
			)
			CompareRef(
				RegistryRef(ZOTHost, ArtifactRepo, "sha256:938419ae89a9947476bbed93abc5eb7abf7d5708be69679fe6cc4b22afe8fdd5"),
				RegistryRef(ZOTHost, testRepo, "sha256:938419ae89a9947476bbed93abc5eb7abf7d5708be69679fe6cc4b22afe8fdd5"),
			)
			CompareRef(
				RegistryRef(ZOTHost, ArtifactRepo, "sha256:20e7d3a6ce087c54238c18a3428853b50cdaf4478a9d00caa8304119b58ae8a9"),
				RegistryRef(ZOTHost, testRepo, "sha256:20e7d3a6ce087c54238c18a3428853b50cdaf4478a9d00caa8304119b58ae8a9"),
			)
		})

		It("should successfully restore an image without referrers from a tar file", func() {
			// Create a backup tar file
			tmpDir := GinkgoT().TempDir()
			backupTar := filepath.Join(tmpDir, "backup-single-tag.tar")
			srcRef := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)

			ORAS("backup", "--output", backupTar, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-single-tag-tar")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			// Restore from backup tar
			foobarStates := append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageConfigStateKey(oras.MediaTypeUnknownConfig))

			ORAS("restore", "--input", backupTar, Flags.ExcludeReferrers, dstRef).
				MatchStatus(foobarStates, true, len(foobarStates)).
				MatchKeyWords("0 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)
		})

		It("should restore an artifact with its referrers from a tar file", func() {
			// Create a backup tar with referrers
			tmpDir := GinkgoT().TempDir()
			backupTar := filepath.Join(tmpDir, "backup-with-referrers.tar")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)

			ORAS("backup", "--output", backupTar, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-with-referrers-tar")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			// Restore from backup tar with referrers
			foobarStates := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)

			ORAS("restore", "--input", backupTar, dstRef).
				MatchStatus(foobarStates, true, len(foobarStates)).
				MatchKeyWords("2 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)

			// Verify referrers were restored
			referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, referrerDgst), RegistryRef(ZOTHost, testRepo, referrerDgst))
			}
		})

		It("should successfully restore a multi-arch artifact without referrers from a tar file", func() {
			// Create a backup tar
			tmpDir := GinkgoT().TempDir()
			backupTar := filepath.Join(tmpDir, "backup-multi-arch.tar")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, ma.Tag)

			ORAS("backup", "--output", backupTar, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-multi-arch-tar")
			dstRef := RegistryRef(ZOTHost, testRepo, ma.Tag)

			// Restore from backup tar
			stateKeys := ma.IndexStateKeys

			ORAS("restore", "--input", backupTar, Flags.ExcludeReferrers, dstRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("0 referrer(s)").
				MatchKeyWords("Successfully restored 1 tag(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)
		})
	})

	When("restoring multiple tags", func() {
		It("should restore multiple specified tags without referrers from a directory", func() {
			// Create a backup with multiple tags
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-multiple-tags")
			srcTags := []string{foobar.Tag, ma.Tag}
			srcRefs := fmt.Sprintf("%s/%s:%s", ZOTHost, ArtifactRepo, strings.Join(srcTags, ","))

			ORAS("backup", "--output", backupDir, srcRefs).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-multiple-tags")
			dstRefs := fmt.Sprintf("%s/%s:%s", ZOTHost, testRepo, strings.Join(srcTags, ","))

			// Restore multiple tags
			stateKeys := append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey)
			stateKeys = append(stateKeys, ma.IndexStateKeys...)

			ORAS("restore", "--input", backupDir, Flags.ExcludeReferrers, dstRefs).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("0 referrer(s)").
				MatchKeyWords("Successfully restored 2 tag(s)").
				Exec()

			// Verify restored content
			for _, tag := range srcTags {
				CompareRef(
					RegistryRef(ZOTHost, ArtifactRepo, tag),
					RegistryRef(ZOTHost, testRepo, tag),
				)
			}
		})

		It("should restore multiple specified tags with referrers from a directory", func() {
			// Create a backup with multiple tags and referrers
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-multiple-tags-referrers")
			srcTags := []string{foobar.Tag, ma.Tag}
			srcRefs := fmt.Sprintf("%s/%s:%s", ZOTHost, ArtifactRepo, strings.Join(srcTags, ","))

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRefs).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-multiple-tags-referrers")
			dstRefs := fmt.Sprintf("%s/%s:%s", ZOTHost, testRepo, strings.Join(srcTags, ","))

			// Restore multiple tags with referrers
			// foobar state keys with referrers
			foobarStateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			// ma state keys with referrers
			maStateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			// combined state keys
			stateKeys := append(foobarStateKeys, maStateKeys...)

			ORAS("restore", "--input", backupDir, dstRefs).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Successfully restored 2 tag(s)").
				Exec()

			// Verify restored content
			for _, tag := range srcTags {
				srcRef := RegistryRef(ZOTHost, ArtifactRepo, tag)
				dstRef := RegistryRef(ZOTHost, testRepo, tag)

				CompareRef(srcRef, dstRef)

				// Verify referrers were restored for each tag
				referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
				for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
					CompareRef(RegistryRef(ZOTHost, ArtifactRepo, referrerDgst), RegistryRef(ZOTHost, testRepo, referrerDgst))
				}
			}
		})

		It("should restore all tags without referrers when no specific tags are provided", func() {
			// Create a backup with multiple tags
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-all-tags")
			srcTags := []string{foobar.Tag, ma.Tag}

			// Prepare test repo with multiple tags for backup
			testSrcRepo := restoreTestRepo("backup-all-tags-src")
			for _, tag := range srcTags {
				prepare(RegistryRef(ZOTHost, ArtifactRepo, tag), RegistryRef(ZOTHost, testSrcRepo, tag))
			}

			// Backup all tags from source repo
			srcRepoRef := fmt.Sprintf("%s/%s", ZOTHost, testSrcRepo)
			ORAS("backup", "--output", backupDir, srcRepoRef).Exec()

			// Create target repo for restore
			testDstRepo := restoreTestRepo("restore-all-tags")
			dstRepoRef := fmt.Sprintf("%s/%s", ZOTHost, testDstRepo)

			// Restore all tags from backup
			stateKeys := append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey)
			stateKeys = append(stateKeys, ma.IndexStateKeys...)

			ORAS("restore", "--input", backupDir, Flags.ExcludeReferrers, dstRepoRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("0 referrer(s)").
				MatchKeyWords("Successfully restored 2 tag(s)").
				Exec()

			// Verify all tags were restored
			for _, tag := range srcTags {
				CompareRef(
					RegistryRef(ZOTHost, testSrcRepo, tag),
					RegistryRef(ZOTHost, testDstRepo, tag),
				)
			}
		})

		It("should restore all tags with referrers when no specific tags are provided", func() {
			// Create a backup with multiple tags and their referrers
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-all-tags-referrers")
			srcTags := []string{foobar.Tag, ma.Tag}

			// Prepare test repo with multiple tags for backup
			testSrcRepo := restoreTestRepo("backup-all-tags-referrers-src")
			for _, tag := range srcTags {
				ORAS("cp", RegistryRef(ZOTHost, ArtifactRepo, tag), RegistryRef(ZOTHost, testSrcRepo, tag), "-r").
					WithDescription("copying tag to test repo").
					Exec()
			}

			// Backup all tags and referrers from source repo
			srcRepoRef := fmt.Sprintf("%s/%s", ZOTHost, testSrcRepo)
			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRepoRef).Exec()

			// Create target repo for restore
			testDstRepo := restoreTestRepo("restore-all-tags-referrers")
			dstRepoRef := fmt.Sprintf("%s/%s", ZOTHost, testDstRepo)

			// Restore all tags and their referrers from backup
			// foobar state keys with referrers
			foobarStateKeys := append(append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageReferrerConfigStateKeys[0]), foobar.ImageReferrersStateKeys...)
			// ma state keys with referrers
			maStateKeys := append(ma.IndexStateKeys, ma.IndexZOTReferrerStateKey, ma.LinuxAMD64ReferrerConfigStateKey)
			// combined state keys
			stateKeys := append(foobarStateKeys, maStateKeys...)

			ORAS("restore", "--input", backupDir, dstRepoRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				MatchKeyWords("Successfully restored 2 tag(s)").
				Exec()

			// Verify all tags and their referrers were restored
			for _, tag := range srcTags {
				srcRef := RegistryRef(ZOTHost, testSrcRepo, tag)
				dstRef := RegistryRef(ZOTHost, testDstRepo, tag)

				CompareRef(srcRef, dstRef)

				// Verify referrers were restored for each tag
				referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
				for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
					CompareRef(RegistryRef(ZOTHost, testSrcRepo, referrerDgst), RegistryRef(ZOTHost, testDstRepo, referrerDgst))
				}
			}
		})
	})

	When("using --dry-run", func() {
		It("should simulate restore without referrers", func() {
			// Create a backup
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-dry-run")
			srcRef := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-dry-run")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			ORAS("restore", "--input", backupDir, Flags.ExcludeReferrers, "--dry-run", dstRef).
				MatchKeyWords("would push tag", "0 referrer(s)").
				MatchKeyWords("Dry run complete", "1 tag(s) would be restored").
				Exec()

			// Verify nothing was actually pushed
			ORAS("manifest", "fetch", dstRef).
				ExpectFailure().
				MatchErrKeyWords("Error response from registry:", "not found").
				Exec()
		})

		It("should simulate restore with referrers", func() {
			// Create a backup with referrers
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-dry-run-referrers")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-dry-run-referrers")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			ORAS("restore", "--input", backupDir, "--dry-run", dstRef).
				MatchKeyWords("would push tag", "2 referrer(s)").
				MatchKeyWords("Dry run complete", "1 tag(s) would be restored").
				Exec()

			// Verify nothing was actually pushed
			ORAS("manifest", "fetch", dstRef).
				ExpectFailure().
				MatchErrKeyWords("Error response from registry:", "not found").
				Exec()
		})
	})

	When("using --concurrency", func() {
		It("should successfully restore with custom concurrency level when --concurrency flag is used", func() {
			// Create a backup
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-concurrency")
			srcRef := RegistryRef(ZOTHost, ImageRepo, ma.Tag)

			ORAS("backup", "--output", backupDir, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-concurrency")
			dstRef := RegistryRef(ZOTHost, testRepo, ma.Tag)

			stateKeys := ma.IndexStateKeys
			ORAS("restore", "--input", backupDir, "--concurrency", "1", dstRef).
				MatchStatus(stateKeys, true, len(stateKeys)).
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)
		})
	})

	When("using --distribution-spec options", func() {

		It("should restore using referrers API with --distribution-spec v1.1-referrers-api", func() {
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-referrers-api")
			srcRef := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-referrers-api")
			dstRef := RegistryRef(ZOTHost, testRepo, foobar.Tag)

			ORAS("restore", "--input", backupDir, Flags.DistributionSpec, "v1.1-referrers-api", dstRef).
				MatchKeyWords("2 referrer(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)

			// Verify referrers were restored
			referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
				CompareRef(RegistryRef(ZOTHost, ArtifactRepo, referrerDgst), RegistryRef(ZOTHost, testRepo, referrerDgst))
			}
		})

		It("should restore using tag schema with --distribution-spec v1.1-referrers-tag", func() {
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-referrers-tag")
			srcRef := RegistryRef(FallbackHost, ArtifactRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, Flags.IncludeReferrers, srcRef).Exec()

			// Create target repo for restore
			testRepo := restoreTestRepo("restore-referrers-tag")
			dstRef := RegistryRef(FallbackHost, testRepo, foobar.Tag)

			ORAS("restore", "--input", backupDir, Flags.DistributionSpec, "v1.1-referrers-tag", dstRef).
				MatchKeyWords("2 referrer(s)").
				Exec()

			// Verify restored content
			CompareRef(srcRef, dstRef)

			// Verify referrers were restored
			referrers := ORAS("discover", dstRef, "--format", "go-template={{range .referrers}}{{println .digest}}{{end}}").Exec().Out.Contents()
			for referrerDgst := range strings.SplitSeq(strings.TrimSpace(string(referrers)), "\n") {
				CompareRef(RegistryRef(FallbackHost, ArtifactRepo, referrerDgst), RegistryRef(FallbackHost, testRepo, referrerDgst))
			}
		})
	})

	When("handling error cases", func() {
		It("should fail when restoring from a non-existent backup directory", func() {
			// Create a non-existent backup directory
			nonExistentBackupDir := filepath.Join(GinkgoT().TempDir(), "non-existent-backup")

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-non-existent-backup"))
			// Attempt to restore from non-existent backup directory
			ORAS("restore", "--input", nonExistentBackupDir, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "no such file or directory").
				Exec()
		})

		It("should fail when restoring from an empty backup directory", func() {
			// Create an invalid backup directory (empty)
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "empty-backup")
			err := os.MkdirAll(backupDir, 0755)
			Expect(err).ToNot(HaveOccurred())

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-empty-backup"))
			// Attempt to restore from empty backup directory
			ORAS("restore", "--input", backupDir, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "no tags found").
				MatchErrKeyWords("oras repo tags --oci-layout"). // test recommendation
				Exec()
		})

		It("should fail when restoring from an invalid backup directory", func() {
			// Create an invalid backup directory (not a valid OCI layout)
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "invalid-backup")
			err := os.MkdirAll(backupDir, 0755)
			Expect(err).ToNot(HaveOccurred())
			err = os.WriteFile(filepath.Join(backupDir, "invalid-file.txt"), []byte("not a valid layout"), 0644)
			Expect(err).ToNot(HaveOccurred())

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-invalid-backup"))
			// Attempt to restore from invalid backup directory
			ORAS("restore", "--input", backupDir, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "no tags found").
				Exec()
		})

		It("should fail when restoring from a non-existent backup tar file", func() {
			// Create a non-existent backup tar file
			nonExistentBackupTar := filepath.Join(GinkgoT().TempDir(), "non-existent-backup.tar")

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-non-existent-backup-tar"))
			// Attempt to restore from non-existent backup tar file
			ORAS("restore", "--input", nonExistentBackupTar, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "no such file or directory").
				Exec()
		})

		It("should fail when restoring from an invalid backup tar file", func() {
			// Create an invalid backup tar file (empty)
			tmpDir := GinkgoT().TempDir()
			backupTar := filepath.Join(tmpDir, "invalid-backup.tar")

			// Create an empty file to simulate an invalid tar
			err := os.WriteFile(backupTar, []byte{}, 0644)
			Expect(err).ToNot(HaveOccurred())

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-invalid-backup-tar"))
			// Attempt to restore from invalid backup tar file
			ORAS("restore", "--input", backupTar, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "invalid OCI Image Layout").
				Exec()
		})

		It("should fail when restoring from a non-tar file", func() {
			// Create a non-tar backup file
			tmpDir := GinkgoT().TempDir()
			backupFile := filepath.Join(tmpDir, "not-tar.txt")

			// Create an empty non-tar file
			err := os.WriteFile(backupFile, []byte{}, 0644)
			Expect(err).ToNot(HaveOccurred())

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-non-tar-file"))
			// Attempt to restore from invalid backup tar file
			ORAS("restore", "--input", backupFile, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:").
				Exec()
		})

		It("should fail when no tags are found in the backup", func() {
			// Create an OCI layout with no tags
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-no-tags")
			ORAS("push", Flags.Layout, backupDir).Exec()

			dstRef := fmt.Sprintf("%s/%s", ZOTHost, restoreTestRepo("restore-no-tags"))
			// Attempt to restore from empty backup
			ORAS("restore", "--input", backupDir, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "no tags found").
				MatchErrKeyWords("oras repo tags --oci-layout"). // test recommendation
				Exec()
		})

		It("should fail when the specified tag doesn't exist in the backup", func() {
			// Create a backup
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-missing-tag")
			srcRef := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, srcRef).Exec()

			// Create target repo for restore with a non-existent tag
			testRepo := restoreTestRepo("restore-missing-tag")
			dstRef := RegistryRef(ZOTHost, testRepo, "non-existent-tag")

			// Attempt to restore a non-existent tag
			ORAS("restore", "--input", backupDir, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:", "non-existent-tag").Exec()
		})

		It("should fail when trying to restore to a non-existent registry", func() {
			// Create a backup
			tmpDir := GinkgoT().TempDir()
			backupDir := filepath.Join(tmpDir, "backup-nonexistent-registry")
			srcRef := RegistryRef(ZOTHost, ImageRepo, foobar.Tag)

			ORAS("backup", "--output", backupDir, srcRef).Exec()

			// Try to restore to a non-existent registry
			nonExistentRegistry := fmt.Sprintf("non-existent-repo-%d-%d.example.com", GinkgoRandomSeed(), time.Now().UnixNano())
			dstRef := RegistryRef(nonExistentRegistry, restoreTestRepo("restore-test"), foobar.Tag)

			// Attempt to restore to a non-existent registry
			ORAS("restore", "--input", backupDir, dstRef).ExpectFailure().
				MatchErrKeyWords("Error:").
				Exec()
		})
	})
})
