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
	"path/filepath"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"oras.land/oras/test/e2e/internal/testdata/feature"
	"oras.land/oras/test/e2e/internal/testdata/foobar"
	. "oras.land/oras/test/e2e/internal/utils"
	"oras.land/oras/test/e2e/internal/utils/match"
)

const (
	pushContent   = "test-blob"
	pushDigest    = "sha256:e1ca41574914ba00e8ed5c8fc78ec8efdfd48941c7e48ad74dad8ada7f2066d8"
	invalidDigest = "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	pushDescFmt   = `{"mediaType":"%s","digest":"sha256:e1ca41574914ba00e8ed5c8fc78ec8efdfd48941c7e48ad74dad8ada7f2066d8","size":9}`
)

var _ = Describe("ORAS beginners:", func() {
	repoFmt := fmt.Sprintf("command/blob/%%s/%d/%%s", GinkgoRandomSeed())
	When("running blob command", func() {
		When("running `blob push`", func() {
			It("should not show --verbose in help doc", func() {
				out := ORAS("push", "--help").MatchKeyWords(ExampleDesc).Exec().Out
				gomega.Expect(out).ShouldNot(gbytes.Say("--verbose"))
			})

			It("should show deprecation message and print unnamed status output for --verbose", func() {
				repo := fmt.Sprintf(repoFmt, "push", "test-verbose")
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, pushDigest), "-", "--size", strconv.Itoa(len(pushContent)), "--verbose").
					WithInput(strings.NewReader(pushContent)).
					MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).
					MatchStatus([]match.StateKey{{Digest: "e1ca41574914", Name: "application/vnd.oci.image.layer.v1.tar"}}, true, 1).
					Exec()
			})

			It("should show deprecation message and should NOT print unnamed status output for --verbose=false", func() {
				repo := fmt.Sprintf(repoFmt, "push", "test-verbose-false")
				out := ORAS("blob", "push", RegistryRef(ZOTHost, repo, pushDigest), "-", "--size", strconv.Itoa(len(pushContent)), "--verbose=false").
					WithInput(strings.NewReader(pushContent)).
					MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).
					Exec().Out
				// should not print status output for unnamed blobs
				gomega.Expect(out).ShouldNot(gbytes.Say("application/vnd.oci.image.layer.v1.tar"))
			})

			It("should fail to read blob content and password from stdin at the same time", func() {
				repo := fmt.Sprintf(repoFmt, "push", "password-stdin")
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, ""), "--password-stdin", "-").
					ExpectFailure().
					MatchErrKeyWords("Error: `-` read file from input and `--password-stdin` read password from input cannot be both used").Exec()
			})

			It("should fail to read blob content and identity token from stdin at the same time", func() {
				repo := fmt.Sprintf(repoFmt, "push", "password-stdin")
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, ""), "--identity-token-stdin", "-").
					ExpectFailure().
					MatchErrKeyWords("Error: `-` read file from input and `--identity-token-stdin` read identity token from input cannot be both used").Exec()
			})

			It("should fail to push a blob from stdin but no blob size provided", func() {
				repo := fmt.Sprintf(repoFmt, "push", "no-size")
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, pushDigest), "-").
					WithInput(strings.NewReader(pushContent)).
					ExpectFailure().
					MatchErrKeyWords("Error: `--size` must be provided if the blob is read from stdin").Exec()
			})

			It("should fail to push a blob from stdin if invalid blob size provided", func() {
				content := "another-test"
				digest := "sha256:c897eff15c4586525388034f8246346681cb48d75a619039c566c4939a18102e"
				repo := fmt.Sprintf(repoFmt, "push", "invalid-stdin-size")
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, digest), "-", "--size", "3").
					WithInput(strings.NewReader(content)).ExpectFailure().
					Exec()
			})

			It("should fail to push a blob from stdin if invalid digest provided", func() {
				repo := fmt.Sprintf(repoFmt, "push", "invalid-stdin-digest")
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, invalidDigest), "-", "--size", strconv.Itoa(len(pushContent))).
					WithInput(strings.NewReader(pushContent)).ExpectFailure().
					Exec()
			})

			It("should fail to push a blob from file if invalid blob size provided", func() {
				repo := fmt.Sprintf(repoFmt, "push", "invalid-file-digest")
				blobPath := WriteTempFile("blob", pushContent)
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, pushDigest), blobPath, "--size", "3").
					ExpectFailure().
					Exec()
			})

			It("should fail to push a blob from file if invalid digest provided", func() {
				repo := fmt.Sprintf(repoFmt, "push", "invalid-stdin-size")
				blobPath := WriteTempFile("blob", pushContent)
				ORAS("blob", "push", RegistryRef(ZOTHost, repo, invalidDigest), blobPath, "--size", strconv.Itoa(len(pushContent))).
					WithInput(strings.NewReader(pushContent)).ExpectFailure().
					Exec()
			})

			It("should fail if no reference is provided", func() {
				ORAS("blob", "push").ExpectFailure().Exec()
			})
		})

		When("running `blob fetch`", func() {
			It("should call sub-commands with aliases", func() {
				ORAS("blob", "get", "--help").
					MatchKeyWords(ExampleDesc).
					Exec()
			})
			It("should have flag for prettifying JSON output", func() {
				ORAS("blob", "get", "--help").
					MatchKeyWords("--pretty", "prettify JSON").
					Exec()
			})

			It("should fail if neither output path nor descriptor flag are not provided", func() {
				ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae")).
					ExpectFailure().Exec()
			})

			It("should fail and show detailed error description if no argument provided", func() {
				err := ORAS("blob", "fetch").ExpectFailure().Exec().Err
				gomega.Expect(err).Should(gbytes.Say("Error"))
				gomega.Expect(err).Should(gbytes.Say("\nUsage: oras blob fetch"))
				gomega.Expect(err).Should(gbytes.Say("\n"))
				gomega.Expect(err).Should(gbytes.Say(`Run "oras blob fetch -h"`))
			})

			It("should fail if no digest is provided", func() {
				ORAS("blob", "fetch", "--descriptor", RegistryRef(ZOTHost, ImageRepo, "")).
					ExpectFailure().MatchErrKeyWords("Error", "no digest specified", "oras blob fetch").Exec()
			})

			It("should fail if provided digest doesn't exist", func() {
				ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, "sha256:2aaa2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a2a"), "-o", "/dev/null").
					ExpectFailure().Exec()
			})

			It("should fail if output path points to stdout and descriptor flag is provided", func() {
				ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, ""), "--descriptor", "--output", "-").
					ExpectFailure().Exec()
			})

			It("should fail if no reference is provided", func() {
				ORAS("blob", "fetch").ExpectFailure().Exec()
			})
		})

		When("running `blob delete`", func() {
			It("should fail and show detailed error description if no argument provided", func() {
				err := ORAS("blob", "delete").ExpectFailure().Exec().Err
				gomega.Expect(err).Should(gbytes.Say("Error"))
				gomega.Expect(err).Should(gbytes.Say("\nUsage: oras blob delete"))
				gomega.Expect(err).Should(gbytes.Say("\n"))
				gomega.Expect(err).Should(gbytes.Say(`Run "oras blob delete -h"`))
			})
		})
	})

	When("running `blob delete`", func() {
		It("should fail if no blob reference is provided", func() {
			dstRepo := fmt.Sprintf(repoFmt, "delete", "no-ref")
			CopyZOTRepo(ImageRepo, dstRepo)
			ORAS("blob", "delete").ExpectFailure().Exec()
			ORAS("blob", "fetch", RegistryRef(ZOTHost, dstRepo, foobar.FooBlobDigest), "--output", "-").MatchContent(foobar.FooBlobContent).Exec()
		})

		It("should fail if no force flag and descriptor flag is provided", func() {
			dstRepo := fmt.Sprintf(repoFmt, "delete", "no-confirm")
			CopyZOTRepo(ImageRepo, dstRepo)
			ORAS("blob", "delete", RegistryRef(ZOTHost, dstRepo, foobar.FooBlobDigest), "--descriptor").ExpectFailure().Exec()
			ORAS("blob", "fetch", RegistryRef(ZOTHost, dstRepo, foobar.FooBlobDigest), "--output", "-").MatchContent(foobar.FooBlobContent).Exec()
		})

		It("should fail if the blob reference is not in the form of <name@digest>", func() {
			dstRepo := fmt.Sprintf(repoFmt, "delete", "wrong-ref-form")
			ORAS("blob", "delete", fmt.Sprintf("%s/%s:%s", ZOTHost, dstRepo, "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"), "--descriptor", "--force").ExpectFailure().Exec()
			ORAS("blob", "delete", fmt.Sprintf("%s/%s:%s", ZOTHost, dstRepo, "test"), "--descriptor", "--force").ExpectFailure().Exec()
			ORAS("blob", "delete", fmt.Sprintf("%s/%s@%s", ZOTHost, dstRepo, "test"), "--descriptor", "--force").ExpectFailure().Exec()
		})

		It("should fail if no digest is provided", func() {
			ORAS("blob", "delete", RegistryRef(ZOTHost, ImageRepo, "")).
				ExpectFailure().MatchErrKeyWords("Error", "no digest specified", "oras blob delete").Exec()
		})

		It("should fail to delete a non-existent blob without force flag set", func() {
			toDeleteRef := RegistryRef(ZOTHost, ImageRepo, invalidDigest)
			ORAS("blob", "delete", toDeleteRef).
				ExpectFailure().
				MatchErrKeyWords(toDeleteRef, "the specified blob does not exist").
				Exec()
		})

		It("should fail to delete a non-existent blob and output descriptor, with force flag set", func() {
			toDeleteRef := RegistryRef(ZOTHost, ImageRepo, invalidDigest)
			ORAS("blob", "delete", toDeleteRef, "--force", "--descriptor").
				ExpectFailure().
				MatchErrKeyWords(toDeleteRef, "the specified blob does not exist").
				Exec()
		})
	})
})

var _ = Describe("1.1 registry users:", func() {
	repoFmt := fmt.Sprintf("command/blob/%%s/%d/%%s", GinkgoRandomSeed())
	When("running `blob delete`", func() {
		It("should delete a blob with interactive confirmation", func() {
			dstRepo := fmt.Sprintf(repoFmt, "delete", "prompt-confirmation")
			CopyZOTRepo(BlobRepo, dstRepo)
			toDeleteRef := RegistryRef(ZOTHost, dstRepo, foobar.FooBlobDigest)
			ORAS("blob", "delete", toDeleteRef).
				WithInput(strings.NewReader("y")).
				MatchKeyWords("Deleted", toDeleteRef).Exec()
			// cannot verify the whether blob is deleted since zot will cache the blob
			// https://github.com/project-zot/zot/issues/1733
		})

		It("should delete a blob with force flag and output descriptor", func() {
			dstRepo := fmt.Sprintf(repoFmt, "delete", "flag-confirmation")
			CopyZOTRepo(BlobRepo, dstRepo)
			toDeleteRef := RegistryRef(ZOTHost, dstRepo, foobar.FooBlobDigest)
			ORAS("blob", "delete", toDeleteRef, "--force", "--descriptor").MatchContent(foobar.FooBlobDescriptor).Exec()
		})
		It("should return success when deleting a non-existent blob with force flag set", func() {
			toDeleteRef := RegistryRef(ZOTHost, ImageRepo, invalidDigest)
			ORAS("blob", "delete", toDeleteRef, "--force").
				MatchKeyWords("Missing", toDeleteRef).
				Exec()
		})
		It("should show deprecation message when running with --verbose flag", func() {
			dstRepo := fmt.Sprintf(repoFmt, "delete", "blob-delete-verbose")
			CopyZOTRepo(BlobRepo, dstRepo)
			toDeleteRef := RegistryRef(ZOTHost, dstRepo, foobar.FooBlobDigest)
			ORAS("blob", "delete", toDeleteRef, "--verbose").MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).Exec()
		})
	})
	When("running `blob push`", func() {
		It("should push a blob from a file and output the descriptor with specific media-type", func() {
			mediaType := "test.media"
			repo := fmt.Sprintf(repoFmt, "push", "blob-file-media-type")
			blobPath := WriteTempFile("blob", pushContent)
			ORAS("blob", "push", RegistryRef(ZOTHost, repo, ""), blobPath, "--media-type", mediaType, "--descriptor").
				MatchContent(fmt.Sprintf(pushDescFmt, mediaType)).Exec()
			ORAS("blob", "fetch", RegistryRef(ZOTHost, repo, pushDigest), "--output", "-").MatchContent(pushContent).Exec()

			ORAS("blob", "push", RegistryRef(ZOTHost, repo, ""), blobPath).
				WithDescription("skip the pushing if the blob already exists in the target repo").
				MatchStatus([]match.StateKey{{Digest: "e1ca41574914", Name: "application/vnd.oci.image.layer.v1.tar"}}, true, 1).
				MatchKeyWords("Exists").
				Exec()
		})

		It("should push a blob from a stdin and output the descriptor with specific media-type", func() {
			mediaType := "test.media"
			repo := fmt.Sprintf(repoFmt, "push", "blob-file-media-type")
			ORAS("blob", "push", RegistryRef(ZOTHost, repo, pushDigest), "-", "--media-type", mediaType, "--descriptor", "--size", strconv.Itoa(len(pushContent))).
				WithInput(strings.NewReader(pushContent)).
				MatchContent(fmt.Sprintf(pushDescFmt, mediaType)).Exec()
			ORAS("blob", "fetch", RegistryRef(ZOTHost, repo, pushDigest), "--output", "-").MatchContent(pushContent).Exec()
		})
	})

	When("running `blob fetch`", func() {
		It("should fetch blob descriptor ", func() {
			ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, foobar.FooBlobDigest), "--descriptor").
				MatchContent(foobar.FooBlobDescriptor).Exec()
		})
		It("should fetch blob content and output to stdout", func() {
			ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, foobar.FooBlobDigest), "--output", "-").
				MatchContent(foobar.FooBlobContent).Exec()
		})
		It("should show deprecation message when running with --verbose flag", func() {
			ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, foobar.FooBlobDigest), "--descriptor", "--verbose").
				MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).Exec()
		})
		It("should fetch blob content and output to a file", func() {
			tempDir := GinkgoT().TempDir()
			contentPath := filepath.Join(tempDir, "fetched")
			ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, foobar.FooBlobDigest), "--output", contentPath).
				WithWorkDir(tempDir).Exec()
			MatchFile(contentPath, foobar.FooBlobContent, DefaultTimeout)
		})
		It("should fetch blob descriptor and output content to a file", func() {
			contentPath := filepath.Join(GinkgoT().TempDir(), "fetched")
			ORAS("blob", "fetch", RegistryRef(ZOTHost, ImageRepo, foobar.FooBlobDigest), "--output", contentPath, "--descriptor").
				MatchContent(foobar.FooBlobDescriptor).Exec()
			MatchFile(contentPath, foobar.FooBlobContent, DefaultTimeout)
		})
	})
})

var _ = Describe("OCI image layout users:", func() {
	When("running `blob delete`", func() {
		It("should delete a blob with interactive confirmation", func() {
			// prepare
			toDeleteRef := LayoutRef(PrepareTempOCI(ImageRepo), foobar.FooBlobDigest)
			// test
			ORAS("blob", "delete", Flags.Layout, toDeleteRef).
				WithInput(strings.NewReader("y")).
				MatchKeyWords("Deleted", toDeleteRef).Exec()
			// validate
			ORAS("blob", "fetch", toDeleteRef, Flags.Layout, "--output", "-").ExpectFailure().Exec()
		})

		It("should delete a blob with force flag and output descriptor", func() {
			// prepare
			toDeleteRef := LayoutRef(PrepareTempOCI(ImageRepo), foobar.FooBlobDigest)
			// test
			ORAS("blob", "delete", Flags.Layout, toDeleteRef, "--force", "--descriptor").MatchContent(foobar.FooBlobDescriptor).Exec()
			// validate
			ORAS("blob", "fetch", Flags.Layout, toDeleteRef, "--output", "-").ExpectFailure().Exec()
		})

		It("should return success when deleting a non-existent blob with force flag set", func() {
			// prepare
			toDeleteRef := RegistryRef(ZOTHost, ImageRepo, invalidDigest)
			// test
			ORAS("blob", "delete", Flags.Layout, toDeleteRef, "--force").
				MatchKeyWords("Missing", toDeleteRef).
				Exec()
		})
		It("should fail to delete a manifest when a tag is provided instead of a digest", func() {
			root := PrepareTempOCI(ImageRepo)
			ref := LayoutRef(root, foobar.Tag)

			ORAS("blob", "delete", Flags.Layout, ref, "--force").
				ExpectFailure().
				MatchErrKeyWords("no digest specified").
				Exec()

			ORAS("manifest", "fetch", Flags.Layout, ref).Exec()
		})
	})

	When("running `blob fetch`", func() {
		It("should fetch blob descriptor", func() {
			root := PrepareTempOCI(ImageRepo)
			ORAS("blob", "fetch", Flags.Layout, LayoutRef(root, foobar.FooBlobDigest), "--descriptor").
				MatchContent(foobar.FooBlobDescriptor).Exec()
		})
		It("should fetch blob content and output to stdout", func() {
			root := PrepareTempOCI(ImageRepo)
			ORAS("blob", "fetch", Flags.Layout, LayoutRef(root, foobar.FooBlobDigest), "--output", "-").
				MatchContent(foobar.FooBlobContent).Exec()
		})
		It("should fail if a tag is provided instead of a digest", func() {
			root := PrepareTempOCI(ImageRepo)
			ref := LayoutRef(root, foobar.Tag)

			ORAS("blob", "fetch", Flags.Layout, ref, "--output", "-").
				ExpectFailure().
				MatchErrKeyWords("no digest specified").
				Exec()
		})
		It("should fetch blob content and output to a file", func() {
			root := PrepareTempOCI(ImageRepo)
			tempDir := GinkgoT().TempDir()
			contentPath := filepath.Join(tempDir, "fetched")
			ORAS("blob", "fetch", Flags.Layout, LayoutRef(root, foobar.FooBlobDigest), "--output", contentPath).
				WithWorkDir(tempDir).Exec()
			MatchFile(contentPath, foobar.FooBlobContent, DefaultTimeout)
		})
		It("should fetch blob descriptor and output content to a file", func() {
			root := PrepareTempOCI(ImageRepo)
			tempDir := GinkgoT().TempDir()
			contentPath := filepath.Join(tempDir, "fetched")
			ORAS("blob", "fetch", Flags.Layout, LayoutRef(root, foobar.FooBlobDigest), "--output", contentPath, "--descriptor").
				MatchContent(foobar.FooBlobDescriptor).
				WithWorkDir(tempDir).Exec()
			MatchFile(contentPath, foobar.FooBlobContent, DefaultTimeout)
		})
	})

	When("running `blob push`", func() {
		It("should push a blob from a file and output the descriptor with specific media-type", func() {
			// prepare
			tmpRoot := GinkgoT().TempDir()
			mediaType := "test.media"
			blobPath := WriteTempFile("blob", pushContent)
			// test
			ORAS("blob", "push", Flags.Layout, LayoutRef(tmpRoot, pushDigest), blobPath, "--media-type", mediaType, "--descriptor").
				MatchContent(fmt.Sprintf(pushDescFmt, mediaType)).Exec()
			ORAS("blob", "push", Flags.Layout, LayoutRef(tmpRoot, pushDigest), blobPath).
				WithDescription("skip pushing if the blob already exists in the target repo").
				MatchKeyWords("Exists").Exec()
			// validate
			ORAS("blob", "fetch", LayoutRef(tmpRoot, pushDigest), Flags.Layout, "--output", "-").MatchContent(pushContent).Exec()
		})

		It("should push a blob from a stdin and output the descriptor with specific media-type", func() {
			// prepare
			tmpRoot := GinkgoT().TempDir()
			// test
			mediaType := "test.media"
			ORAS("blob", "push", Flags.Layout, LayoutRef(tmpRoot, pushDigest), "-", "--media-type", mediaType, "--descriptor", "--size", strconv.Itoa(len(pushContent))).
				WithInput(strings.NewReader(pushContent)).
				MatchContent(fmt.Sprintf(pushDescFmt, mediaType)).Exec()
			// validate
			ORAS("blob", "fetch", LayoutRef(tmpRoot, pushDigest), Flags.Layout, "--output", "-").MatchContent(pushContent).Exec()
		})
	})
})
