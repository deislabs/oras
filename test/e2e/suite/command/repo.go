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
	"regexp"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	"github.com/onsi/gomega"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"oras.land/oras/test/e2e/internal/testdata/feature"
	"oras.land/oras/test/e2e/internal/testdata/foobar"
	"oras.land/oras/test/e2e/internal/testdata/multi_arch"
	. "oras.land/oras/test/e2e/internal/utils"
)

var _ = Describe("ORAS beginners:", func() {
	When("running repo command", func() {
		When("running `repo ls`", func() {
			It("should show help description", func() {
				ORAS("repo", "ls", "--help").MatchKeyWords(ExampleDesc).Exec()
			})

			It("should call sub-commands with aliases", func() {
				ORAS("repository", "list", "--help").MatchKeyWords(ExampleDesc).Exec()
			})

			It("should fail listing repositories if wrong registry provided", func() {
				ORAS("repo", "ls", RegistryRef(ZOTHost, ImageRepo, "some-tag")).ExpectFailure().MatchErrKeyWords("Error:").Exec()
			})

			It("should fail and show detailed error description if no argument provided", func() {
				err := ORAS("repo", "ls").ExpectFailure().Exec().Err
				gomega.Expect(err).Should(gbytes.Say("Error"))
				gomega.Expect(err).Should(gbytes.Say("\nUsage: oras repo ls"))
				gomega.Expect(err).Should(gbytes.Say("\n"))
				gomega.Expect(err).Should(gbytes.Say(`Run "oras repo ls -h"`))
			})

			It("should fail if password is wrong with registry error prefix", func() {
				ORAS("repo", "ls", ZOTHost, "-u", Username, "-p", "???").
					MatchErrKeyWords(RegistryErrorPrefix).ExpectFailure().Exec()
			})

			It("should show format help in command output", func() {
				help := ORAS("repo", "ls", "--help").Exec().Out.Contents()
				helpStr := string(help)

				// Check for format flag info in help
				Expect(helpStr).To(ContainSubstring("--format"))
				Expect(helpStr).To(ContainSubstring("json"))
				Expect(helpStr).To(ContainSubstring("text"))
				Expect(helpStr).To(ContainSubstring("go-template"))

				// Check for example in help text
				Expect(helpStr).To(ContainSubstring("oras repo ls"))
				Expect(helpStr).To(ContainSubstring("--format json"))
				Expect(helpStr).To(ContainSubstring("--format go-template"))
			})
		})
		When("running `repo tags`", func() {
			It("should show help description with feature flags", func() {
				out := ORAS("repo", "tags", "--help").MatchKeyWords(ExampleDesc).Exec().Out
				Expect(out).Should(gbytes.Say("--exclude-digest-tags(\\[=true\\|false\\])?\\s+%s", regexp.QuoteMeta(feature.Preview.Mark)))
			})

			It("should call sub-commands with aliases", func() {
				ORAS("repository", "show-tags", "--help").MatchKeyWords(ExampleDesc).Exec()
			})

			It("should fail listing repositories if wrong reference provided", func() {
				ORAS("repo", "tags").ExpectFailure().MatchErrKeyWords("Error:").Exec()
				ORAS("repo", "tags", ZOTHost).ExpectFailure().MatchErrKeyWords("Error:").Exec()
				ORAS("repo", "tags", RegistryRef(ZOTHost, ImageRepo, "some-tag")).ExpectFailure().MatchErrKeyWords(RegistryErrorPrefix).Exec()
			})

			It("should fail and show detailed error description if no argument provided", func() {
				err := ORAS("repo", "tags").ExpectFailure().Exec().Err
				gomega.Expect(err).Should(gbytes.Say("Error"))
				gomega.Expect(err).Should(gbytes.Say("\nUsage: oras repo tags"))
				gomega.Expect(err).Should(gbytes.Say("\n"))
				gomega.Expect(err).Should(gbytes.Say(`Run "oras repo tags -h"`))
			})

			It("should show format help in command output", func() {
				help := ORAS("repo", "tags", "--help").Exec().Out.Contents()
				helpStr := string(help)

				// Check for format flag info in help
				Expect(helpStr).To(ContainSubstring("--format"))
				Expect(helpStr).To(ContainSubstring("json"))
				Expect(helpStr).To(ContainSubstring("text"))
				Expect(helpStr).To(ContainSubstring("go-template"))

				// Check for example in help text
				Expect(helpStr).To(ContainSubstring("oras repo tags"))
				Expect(helpStr).To(ContainSubstring("--format json"))
				Expect(helpStr).To(ContainSubstring("--format go-template"))
			})
		})
	})
})

var _ = Describe("1.1 registry users:", func() {
	When("running `repo ls`", func() {
		It("should list repositories", func() {
			ORAS("repository", "list", ZOTHost).MatchKeyWords(ImageRepo).Exec()
		})
		It("should list repositories under provided namespace", func() {
			ORAS("repo", "ls", RegistryRef(ZOTHost, Namespace, "")).MatchKeyWords(ImageRepo[len(Namespace)+1:]).Exec()
		})
		It("should show deprecation message when running with --verbose flag", func() {
			ORAS("repository", "list", ZOTHost, "--verbose").MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).Exec()
		})
		It("should not list repositories without a fully matched namespace", func() {
			repo := "command-draft/images"
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, foobar.Tag)).
				WithDescription("prepare destination repo: " + repo).
				Exec()
			ORAS("repo", "ls", ZOTHost).MatchKeyWords(ImageRepo, repo).Exec()
			session := ORAS("repo", "ls", RegistryRef(ZOTHost, Namespace, "")).MatchKeyWords(ImageRepo[len(Namespace)+1:]).Exec()
			Expect(session.Out).ShouldNot(gbytes.Say(repo[len(Namespace)+1:]))
		})
		It("should list repositories via short command", func() {
			ORAS("repo", "ls", ZOTHost).MatchKeyWords(ImageRepo).Exec()
		})
	})

	When("running `repo ls` with JSON format", func() {
		It("should list repositories in JSON format", func() {
			bytes := ORAS("repo", "ls", ZOTHost, "--format", "json").
				WithDescription("get repos in JSON format").
				MatchKeyWords(`"repositories"`).
				Exec().Out.Contents()

			// Parse the JSON output
			var result struct {
				Registry     string   `json:"registry"`
				Repositories []string `json:"repositories"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())

			// Verify registry is in the output
			Expect(result.Registry).To(Equal(ZOTHost))
			// Verify repositories are in the output
			Expect(result.Repositories).Should(ContainElement(ImageRepo))
		})

		It("should list repositories under provided namespace in JSON format", func() {
			bytes := ORAS("repo", "ls", RegistryRef(ZOTHost, Namespace, ""), "--format", "json").
				WithDescription("get repos in JSON format under namespace").
				MatchKeyWords(`"repositories"`).
				Exec().Out.Contents()

			// Parse the JSON output
			var result struct {
				Registry     string   `json:"registry"`
				Repositories []string `json:"repositories"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())

			// Verify registry is in the output
			Expect(result.Registry).To(Equal(ZOTHost))
			// Verify repositories are in the output
			Expect(result.Repositories).Should(ContainElement(ImageRepo))
		})

		It("should not list repositories without a fully matched namespace in JSON format", func() {
			repo := "command-draft/images"
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, foobar.Tag)).
				WithDescription("prepare destination repo: " + repo).
				Exec()
			bytes := ORAS("repo", "ls", RegistryRef(ZOTHost, Namespace, ""), "--format", "json").
				WithDescription("get repos in JSON format under namespace").
				Exec().Out.Contents()

			// Parse the JSON output
			var result struct {
				Registry     string   `json:"registry"`
				Repositories []string `json:"repositories"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())
			// Verify registry is in the output
			Expect(result.Registry).To(Equal(ZOTHost))
			// Verify repositories are in the output
			Expect(result.Repositories).Should(ContainElement(ImageRepo))
			// Ensure the other repo is not listed
			Expect(result.Repositories).ShouldNot(ContainElement(repo))
		})
	})

	When("running `repo ls` with go-template format", func() {
		It("should list repositories in go-template format", func() {
			template := `"Registry: {{.registry}}{{println}}{{range .repositories}}{{println .}}{{end}}"`
			output := ORAS("repo", "ls", ZOTHost, "--format", "go-template="+template).
				WithDescription("get repos in go-template format").
				Exec().Out.Contents()

			outputString := string(output)
			// Verify registry is in the output
			Expect(outputString).To(ContainSubstring("Registry: " + ZOTHost))
			// Verify repositories are in the output
			Expect(outputString).To(ContainSubstring(ImageRepo))
		})

		It("should list repositories under provided namespace in go-template format", func() {
			// Template that prints registry and then each repository
			template := `"Registry: {{.registry}}{{println}}{{range .repositories}}{{println .}}{{end}}"`
			output := ORAS("repo", "ls", RegistryRef(ZOTHost, Namespace, ""), "--format", "go-template="+template).
				WithDescription("get repos in go-template format under namespace").
				Exec().Out.Contents()

			outputString := string(output)
			// Verify registry is in the output
			Expect(outputString).To(ContainSubstring("Registry: " + ZOTHost))
			// Verify repositories are in the output
			Expect(outputString).To(ContainSubstring(ImageRepo))
		})

		It("should not list repositories without a fully matched namespace in go-template format", func() {
			repo := "command-draft/images"
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, foobar.Tag)).
				WithDescription("prepare destination repo: " + repo).
				Exec()
			template := `"Registry: {{.registry}}{{println}}{{range .repositories}}{{println .}}{{end}}"`
			output := ORAS("repo", "ls", RegistryRef(ZOTHost, Namespace, ""), "--format", "go-template="+template).
				WithDescription("get repos in go-template format under namespace").
				Exec().Out.Contents()

			outputString := string(output)
			// Verify registry is in the output
			Expect(outputString).To(ContainSubstring("Registry: " + ZOTHost))
			// Verify repositories are in the output
			Expect(outputString).To(ContainSubstring(ImageRepo))
			// Ensure the other repo is not listed
			Expect(outputString).ShouldNot(ContainSubstring(repo))
		})
	})

	When("running `repo tags`", func() {
		repoWithName := func(name string) string {
			return fmt.Sprintf("command/images/repo/tags/%d/%s", GinkgoRandomSeed(), name)
		}
		repoRef := RegistryRef(ZOTHost, ImageRepo, "")
		It("should list tags", func() {
			ORAS("repository", "show-tags", repoRef).MatchKeyWords(multi_arch.Tag, foobar.Tag).Exec()
		})
		It("should list tags via short command", func() {
			ORAS("repo", "tags", repoRef).MatchKeyWords(multi_arch.Tag, foobar.Tag).Exec()
		})
		It("should show deprecation message when running with --verbose flag", func() {
			ORAS("repo", "tags", repoRef, "--verbose").MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).Exec()
		})

		It("Should list out tags associated to the provided reference", func() {
			// prepare
			repo := repoWithName("filter-tag")
			tags := []string{foobar.Tag, "bax", "bay", "baz"}
			refWithTags := fmt.Sprintf("%s:%s", RegistryRef(ZOTHost, repo, ""), strings.Join(tags, ","))
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), refWithTags).
				WithDescription("prepare: copy and create multiple tags to " + refWithTags).
				Exec()
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, multi_arch.Tag), RegistryRef(ZOTHost, ImageRepo, "")).
				WithDescription("prepare: copy tag with different digest").
				Exec()
			// test
			viaTag := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, foobar.Tag)).
				MatchKeyWords(tags...).
				MatchErrKeyWords(feature.Experimental.Mark, foobar.Digest).Exec().Out
			Expect(viaTag).ShouldNot(gbytes.Say(multi_arch.Tag))

			viaDigest := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, foobar.Digest)).
				MatchKeyWords(tags...).
				MatchErrKeyWords(feature.Experimental.Mark, foobar.Digest).Exec().Out
			Expect(viaDigest).ShouldNot(gbytes.Say(multi_arch.Tag))
		})

	})

	When("running `repo tags` with JSON format", func() {
		repoWithName := func(name string) string {
			return fmt.Sprintf("command/images/repo/tags/%d/%s", GinkgoRandomSeed(), name)
		}
		repoRef := RegistryRef(ZOTHost, ImageRepo, "")

		It("should list tags in JSON format", func() {
			ORAS("repo", "tags", repoRef, "--format", "json").
				WithDescription("get repo tags in JSON format").
				MatchKeyWords(`"tags"`).
				Exec()

			// Parse the output to validate JSON structure
			bytes := ORAS("repo", "tags", repoRef, "--format", "json").Exec().Out.Contents()
			var result struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())

			// Verify tags are in the output (using the known tags from the standard repository)
			Expect(result.Tags).Should(ContainElement(multi_arch.Tag))
			Expect(result.Tags).Should(ContainElement(foobar.Tag))
		})

		It("should handle digest exclusion with JSON format", func() {
			// Prepare a repository with a digest-like tag
			repo := repoWithName("json-digest")
			normalTag := "normal-tag"
			digestLikeTag := "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

			// Create repository with tags
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, normalTag)).
				WithDescription("prepare test repo with normal tag").Exec()
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, digestLikeTag)).
				WithDescription("prepare test repo with digest-like tag").Exec()

			// Run repo tags with JSON format and exclude digest tags
			bytes := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, ""), "--format", "json", "--exclude-digest-tags").Exec().Out.Contents()

			// Validate the JSON output
			var result struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())

			// Verify only normal tag is in the output
			Expect(result.Tags).Should(ContainElement(normalTag))
			Expect(result.Tags).ShouldNot(ContainElement(digestLikeTag))
			Expect(result.Tags).Should(HaveLen(1))
		})

		It("Should list out tags associated to reference in JSON format", func() {
			// prepare
			repo := repoWithName("filter-tag-json")
			tags := []string{foobar.Tag, "json1", "json2", "json3"}
			refWithTags := fmt.Sprintf("%s:%s", RegistryRef(ZOTHost, repo, ""), strings.Join(tags, ","))
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), refWithTags).
				WithDescription("prepare: copy and create multiple tags").
				Exec()

			// Test via tag with JSON format
			bytesViaTag := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, foobar.Tag), "--format", "json").
				MatchErrKeyWords(feature.Experimental.Mark).
				WithDescription("get JSON format via tag").
				Exec().Out.Contents()

			// Parse and verify JSON output
			var resultViaTag struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytesViaTag, &resultViaTag)).ShouldNot(HaveOccurred())

			// Check each tag individually
			for _, tag := range tags {
				Expect(resultViaTag.Tags).Should(ContainElement(tag))
			}

			// Test via digest with JSON format
			bytesViaDigest := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, foobar.Digest), "--format", "json").
				MatchErrKeyWords(feature.Experimental.Mark).
				WithDescription("get JSON format via digest").
				Exec().Out.Contents()

			// Parse and verify JSON output
			var resultViaDigest struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytesViaDigest, &resultViaDigest)).ShouldNot(HaveOccurred())

			// Check each tag individually
			for _, tag := range tags {
				Expect(resultViaDigest.Tags).Should(ContainElement(tag))
			}
		})
	})

	When("running `repo tags` with go-template format", func() {
		repoWithName := func(name string) string {
			return fmt.Sprintf("command/images/repo/tags/%d/%s", GinkgoRandomSeed(), name)
		}
		repoRef := RegistryRef(ZOTHost, ImageRepo, "")

		It("should output tags using Go template format", func() {
			// Run repo tags with Go template format
			template := "{{range .tags}}{{println .}}{{end}}"
			output := ORAS("repo", "tags", repoRef, "--format", "go-template="+template).Exec().Out.Contents()

			// Verify tags are in the output - should be one tag per line
			outputString := string(output)
			Expect(outputString).To(ContainSubstring(foobar.Tag))
			Expect(outputString).To(ContainSubstring(multi_arch.Tag))
		})

		It("should handle digest exclusion with go-template format", func() {
			// Prepare a repository with a digest-like tag
			repo := repoWithName("template-digest")
			normalTag := "normal-tag"
			digestLikeTag := "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

			// Create repository with tags
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, normalTag)).
				WithDescription("prepare test repo with normal tag").Exec()
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), RegistryRef(ZOTHost, repo, digestLikeTag)).
				WithDescription("prepare test repo with digest-like tag").Exec()

			// Run with go-template and exclude digest tags
			template := "{{range .tags}}{{println .}}{{end}}"
			output := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, ""), "--format", "go-template="+template, "--exclude-digest-tags").Exec().Out.Contents()

			// Verify only normal tag is in the output
			outputString := string(output)
			Expect(outputString).To(ContainSubstring(normalTag))
			Expect(outputString).NotTo(ContainSubstring(digestLikeTag))
		})

		It("Should list out tags associated to reference in go-template format", func() {
			// prepare
			repo := repoWithName("filter-tag-template")
			tags := []string{foobar.Tag, "template1", "template2", "template3"}
			refWithTags := fmt.Sprintf("%s:%s", RegistryRef(ZOTHost, repo, ""), strings.Join(tags, ","))
			ORAS("cp", RegistryRef(ZOTHost, ImageRepo, foobar.Tag), refWithTags).
				WithDescription("prepare: copy and create multiple tags").
				Exec()

			// Define template to list all tags one per line
			template := "{{range .tags}}{{println .}}{{end}}"

			// Test via tag with go-template format
			outputViaTag := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, foobar.Tag), "--format", "go-template="+template).
				MatchErrKeyWords(feature.Experimental.Mark).
				WithDescription("get go-template format via tag").
				Exec().Out.Contents()

			// Check each tag is in the output
			outputTagStr := string(outputViaTag)
			for _, tag := range tags {
				Expect(outputTagStr).To(ContainSubstring(tag))
			}

			// Test via digest with go-template format
			outputViaDigest := ORAS("repo", "tags", RegistryRef(ZOTHost, repo, foobar.Digest), "--format", "go-template="+template).
				MatchErrKeyWords(feature.Experimental.Mark).
				WithDescription("get go-template format via digest").
				Exec().Out.Contents()

			// Check each tag is in the output
			outputDigestStr := string(outputViaDigest)
			for _, tag := range tags {
				Expect(outputDigestStr).To(ContainSubstring(tag))
			}
		})
	})
})

var _ = Describe("1.0 registry users:", func() {
	When("running `repo ls`", func() {
		It("should list partial repositories via `last` flag", func() {
			session := ORAS("repo", "ls", FallbackHost, "--last", ArtifactRepo).Exec()
			repoRegex := regexp.QuoteMeta(ImageRepo + "\n")
			Expect(session.Out).ShouldNot(gbytes.Say(repoRegex))
		})
	})
})

var _ = Describe("OCI image layout users:", func() {
	When("running `repo tags`", func() {
		prepare := func(repo string, fromTag string, toTags ...string) string {
			root := PrepareTempOCI(repo)
			args := []string{"tag", LayoutRef(root, fromTag), Flags.Layout}
			args = append(args, toTags...)
			ORAS(args...).WithDescription("prepare in OCI layout").Exec()
			return root
		}
		tagOutput := foobar.Tag + "\n"
		It("should list tags", func() {
			root := PrepareTempOCI(ImageRepo)
			ORAS("repository", "show-tags", root, Flags.Layout).MatchKeyWords(tagOutput).Exec()
		})
		It("should list tags via short command", func() {
			root := PrepareTempOCI(ImageRepo)
			ORAS("repository", "tags", root, Flags.Layout).MatchKeyWords(tagOutput).Exec()
		})
		It("should list partial tags via `last` flag", func() {
			// prepare
			extra := "zzz"
			root := prepare(ImageRepo, foobar.Tag, extra)
			// test
			session := ORAS("repository", "tags", root, "--last", foobar.Tag, Flags.Layout).MatchKeyWords(extra).Exec()
			Expect(session.Out).ShouldNot(gbytes.Say(regexp.QuoteMeta(tagOutput)))
		})
		It("should list out tags associated to the provided reference", func() {
			// prepare
			tags := []string{foobar.Tag, "bax", "bay", "baz"}
			root := prepare(ImageRepo, foobar.Tag, tags...)
			// test
			viaTag := ORAS("repo", "tags", LayoutRef(root, foobar.Tag), Flags.Layout).
				WithDescription("via tag").
				MatchKeyWords(tags...).
				MatchErrKeyWords(feature.Experimental.Mark, foobar.Digest).Exec().Out
			Expect(viaTag).ShouldNot(gbytes.Say(multi_arch.Tag))
			viaDigest := ORAS("repo", "tags", LayoutRef(root, foobar.Digest), Flags.Layout).
				WithDescription("via digest").
				MatchKeyWords(tags...).
				MatchErrKeyWords(feature.Experimental.Mark, foobar.Digest).Exec().Out
			Expect(viaDigest).ShouldNot(gbytes.Say(multi_arch.Tag))
		})
	})

	When("running `repo tags` with JSON format", func() {
		prepare := func(repo string, fromTag string, toTags ...string) string {
			root := PrepareTempOCI(repo)
			args := []string{"tag", LayoutRef(root, fromTag), Flags.Layout}
			args = append(args, toTags...)
			ORAS(args...).WithDescription("prepare in OCI layout").Exec()
			return root
		}

		It("should list tags in JSON format for OCI layout", func() {
			// Use existing layout
			root := PrepareTempOCI(ImageRepo)

			// Run repo tags with JSON format
			bytes := ORAS("repository", "tags", root, "--format", "json", Flags.Layout).Exec().Out.Contents()

			// Parse the JSON output
			var result struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())

			Expect(result.Tags).Should(ContainElements(foobar.Tag))
		})

		It("should handle digest exclusion with JSON format", func() {
			// Prepare layout with both normal and digest-like tags
			digestLikeTag := "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
			root := prepare(ImageRepo, foobar.Tag, digestLikeTag)

			// Verify both tags are included without exclusion
			plainBytes := ORAS("repo", "tags", root, "--format", "json", Flags.Layout).Exec().Out.Contents()
			var plainResult struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(plainBytes, &plainResult)).ShouldNot(HaveOccurred())
			Expect(plainResult.Tags).Should(ContainElements(digestLikeTag, foobar.Tag))

			// Test with digest exclusion
			bytes := ORAS("repo", "tags", root, "--format", "json", "--exclude-digest-tags", Flags.Layout).Exec().Out.Contents()

			// Validate the JSON output
			var result struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytes, &result)).ShouldNot(HaveOccurred())

			// Verify digest tag is excluded
			Expect(result.Tags).Should(ContainElements(foobar.Tag))
			Expect(result.Tags).ShouldNot(ContainElement(digestLikeTag))
		})

		It("should list out tags associated to the provided reference in JSON format", func() {
			// prepare
			tags := []string{foobar.Tag, "bax", "bay", "baz"}
			root := prepare(ImageRepo, foobar.Tag, tags...)

			// Test via tag with JSON format
			bytesViaTag := ORAS("repo", "tags", LayoutRef(root, foobar.Tag), "--format", "json", Flags.Layout).
				WithDescription("via tag JSON").
				MatchErrKeyWords(feature.Experimental.Mark).
				Exec().Out.Contents()

			// Parse and verify JSON output
			var resultViaTag struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytesViaTag, &resultViaTag)).ShouldNot(HaveOccurred())

			// Check each tag individually
			for _, tag := range tags {
				Expect(resultViaTag.Tags).Should(ContainElement(tag))
			}

			// Test via digest with JSON format
			bytesViaDigest := ORAS("repo", "tags", LayoutRef(root, foobar.Digest), "--format", "json", Flags.Layout).
				WithDescription("via digest JSON").
				MatchErrKeyWords(feature.Experimental.Mark).
				Exec().Out.Contents()

			// Parse and verify JSON output
			var resultViaDigest struct {
				Tags []string `json:"tags"`
			}
			Expect(json.Unmarshal(bytesViaDigest, &resultViaDigest)).ShouldNot(HaveOccurred())

			// Check each tag individually
			for _, tag := range tags {
				Expect(resultViaDigest.Tags).Should(ContainElement(tag))
			}
		})
	})

	When("running `repo tags` with go-template format", func() {
		prepare := func(repo string, fromTag string, toTags ...string) string {
			root := PrepareTempOCI(repo)
			args := []string{"tag", LayoutRef(root, fromTag), Flags.Layout}
			args = append(args, toTags...)
			ORAS(args...).WithDescription("prepare in OCI layout").Exec()
			return root
		}

		It("should list tags in go-template format for OCI layout", func() {
			// Use existing layout
			root := PrepareTempOCI(ImageRepo)

			// Simple template to list tags
			template := "{{range .tags}}{{println .}}{{end}}"

			// Run repo tags with go-template format
			output := ORAS("repository", "tags", root, "--format", "go-template="+template, Flags.Layout).
				Exec().Out.Contents()

			// Verify output contains the expected tag
			outputString := string(output)
			Expect(outputString).To(ContainSubstring(foobar.Tag))
		})

		It("should handle digest exclusion with go-template format in OCI layout", func() {
			// Prepare layout with both normal and digest-like tags
			normalTag := "normal-tag"
			digestLikeTag := "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
			root := prepare(ImageRepo, foobar.Tag, normalTag, digestLikeTag)

			// Template to list all tags
			template := "{{range .tags}}{{println .}}{{end}}"

			// Verify output with digest exclusion
			output := ORAS("repo", "tags", root, "--format", "go-template="+template, "--exclude-digest-tags", Flags.Layout).
				Exec().Out.Contents()

			// Check results
			outputString := string(output)
			Expect(outputString).To(ContainSubstring(normalTag))
			Expect(outputString).NotTo(ContainSubstring(digestLikeTag))
		})

		It("should list out tags associated to the provided reference in go-template format", func() {
			// prepare
			tags := []string{foobar.Tag, "bax", "bay", "baz"}
			root := prepare(ImageRepo, foobar.Tag, tags...)

			// Define template to list tags one per line
			template := "{{range .tags}}{{println .}}{{end}}"

			// Test via tag with go-template format
			outputViaTag := ORAS("repo", "tags", LayoutRef(root, foobar.Tag), "--format", "go-template="+template, Flags.Layout).
				WithDescription("via tag with go-template").
				MatchErrKeyWords(feature.Experimental.Mark).
				Exec().Out.Contents()

			// Verify each tag is in the output
			outputTagStr := string(outputViaTag)
			for _, tag := range tags {
				Expect(outputTagStr).To(ContainSubstring(tag))
			}

			// Test via digest with go-template format
			outputViaDigest := ORAS("repo", "tags", LayoutRef(root, foobar.Digest), "--format", "go-template="+template, Flags.Layout).
				WithDescription("via digest with go-template").
				MatchErrKeyWords(feature.Experimental.Mark).
				Exec().Out.Contents()

			// Verify each tag is in the output
			outputDigestStr := string(outputViaDigest)
			for _, tag := range tags {
				Expect(outputDigestStr).To(ContainSubstring(tag))
			}
		})
	})

	When("showing tags of a specific repository using `--oci-layout-path` flag", func() {
		prepare := func(repo string, fromTag string, toTags ...string) string {
			root := PrepareTempOCI(repo)
			args := []string{"tag", LayoutRef(root, fromTag), Flags.Layout}
			args = append(args, toTags...)
			ORAS(args...).WithDescription("prepare in OCI layout").Exec()
			return root
		}

		It("should show tags of the repo example.registry.com/foo", func() {
			// prepare
			root := prepare(ImageRepo, foobar.Tag, "example.registry.com/foo:latest", "example.registry.com/foo:v1.2.6", "test.com/bar:v1")
			// test
			session := ORAS("repo", "tags", "--oci-layout-path", root, "example.registry.com/foo").MatchKeyWords("latest\n", "v1.2.6\n").Exec()
			Expect(session.Out).ShouldNot(gbytes.Say(foobar.Tag, "example.registry.com/foo:latest", "example.registry.com/foo:v1.2.6", "test.com/bar:v1"))
		})

		It("should still find associated tags if a full reference is provided", func() {
			// prepare
			root := prepare(ImageRepo, foobar.Tag, "example.registry.com/foo:latest", "example.registry.com/foo:v1.2.6", "test.com/bar:v1")
			// test
			ORAS("repo", "tags", "--oci-layout-path", root, "example.registry.com/foo:latest").MatchKeyWords(foobar.Tag, "example.registry.com/foo:latest", "example.registry.com/foo:v1.2.6", "test.com/bar:v1").Exec()
		})

		It("should not show digest tags if --exclude-digest-tags is used", func() {
			// prepare
			root := prepare(ImageRepo, foobar.Tag, "example.registry.com/foo:latest", "example.registry.com/foo:v1.2.6", "test.com/bar:v1", "example.registry.com/foo:sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855")
			// test
			session := ORAS("repo", "tags", "--oci-layout-path", root, "example.registry.com/foo", "--exclude-digest-tags").MatchKeyWords("latest\n", "v1.2.6\n").Exec()
			Expect(session.Out).ShouldNot(gbytes.Say(foobar.Tag, "example.registry.com/foo:latest", "example.registry.com/foo:v1.2.6", "test.com/bar:v1", "sha256-e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"))
		})
	})
})
