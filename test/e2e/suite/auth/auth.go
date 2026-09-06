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

package scenario

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/oras-project/oras-go/v3"
	"oras.land/oras/test/e2e/internal/testdata/feature"
	"oras.land/oras/test/e2e/internal/testdata/foobar"
	. "oras.land/oras/test/e2e/internal/utils"
)

var _ = Describe("Common registry user", func() {
	When("not logged in", func() {
		It("should run commands without logging in", func() {
			authConfigPath := filepath.Join(GinkgoT().TempDir(), "auth.config")
			RunWithoutLogin("attach", ZOTHost+"/repo:tag", "-a", "test=true", "--artifact-type", "doc/example", "--registry-config", authConfigPath)
			RunWithoutLogin("discover", ZOTHost+"/repo:tag", "--registry-config", authConfigPath)
			RunWithoutLogin("push", "-a", "key=value", ZOTHost+"/repo:tag", "--registry-config", authConfigPath)
			RunWithoutLogin("pull", ZOTHost+"/repo:tag", "--registry-config", authConfigPath)
			RunWithoutLogin("manifest", "fetch", ZOTHost+"/repo:tag", "--registry-config", authConfigPath)
			RunWithoutLogin("blob", "delete", ZOTHost+"/repo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "--registry-config", authConfigPath)
			RunWithoutLogin("blob", "push", ZOTHost+"/repo", WriteTempFile("blob", "test"), "--registry-config", authConfigPath)
			RunWithoutLogin("tag", ZOTHost+"/repo:tag", "tag1", "--registry-config", authConfigPath)
			RunWithoutLogin("resolve", ZOTHost+"/repo:tag", "--registry-config", authConfigPath)
			RunWithoutLogin("repo", "ls", ZOTHost, "--registry-config", authConfigPath)
			RunWithoutLogin("repo", "tags", RegistryRef(ZOTHost, "repo", ""), "--registry-config", authConfigPath)
			RunWithoutLogin("manifest", "fetch-config", ZOTHost+"/repo:tag", "--registry-config", authConfigPath)
		})

		It("should fail with modified error prefix when running commands without logging in", func() {
			authConfigPath := filepath.Join(GinkgoT().TempDir(), "auth.config")
			ORAS("copy", ZOTHost+"/repo:from", ZOTHost+"/repo:to", "--from-registry-config", authConfigPath, "--to-registry-config", authConfigPath).
				ExpectFailure().
				MatchErrKeyWords(fmt.Sprintf("Error from source registry for %q", ZOTHost+"/repo:from"), "basic credential not found").
				WithDescription("fail with modified error prefix when not logged in").
				Exec()
		})
	})

	When("credential is invalid", func() {
		It("should fail with registry error", func() {
			RunWithInvalidCreds("attach", ZOTHost+"/repo:tag", "-a", "test=true", "--artifact-type", "doc/example")
			RunWithInvalidCreds("discover", ZOTHost+"/repo:tag")
			RunWithInvalidCreds("manifest", "fetch", ZOTHost+"/repo:tag")
			RunWithInvalidCreds("blob", "delete", ZOTHost+"/repo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			RunWithInvalidCreds("blob", "push", ZOTHost+"/repo", WriteTempFile("blob", "test"))
			RunWithInvalidCreds("tag", ZOTHost+"/repo:tag", "tag1")
			RunWithInvalidCreds("resolve", ZOTHost+"/repo:tag")
			RunWithInvalidCreds("repo", "ls", ZOTHost)
			RunWithInvalidCreds("repo", "tags", RegistryRef(ZOTHost, "repo", ""))
			RunWithInvalidCreds("manifest", "fetch-config", ZOTHost+"/repo:tag")
		})
	})

	When("credential for basic auth not found in the config file", func() {
		It("should fail with registry error", func() {
			RunWithEmptyRegistryConfig("attach", ZOTHost+"/repo:tag", "-a", "test=true", "--artifact-type", "doc/example")
			RunWithEmptyRegistryConfig("discover", ZOTHost+"/repo:tag")
			RunWithEmptyRegistryConfig("push", "-a", "key=value", ZOTHost+"/repo:tag")
			RunWithEmptyRegistryConfig("pull", ZOTHost+"/repo:tag")
			RunWithEmptyRegistryConfig("manifest", "fetch", ZOTHost+"/repo:tag")
			RunWithEmptyRegistryConfig("blob", "delete", ZOTHost+"/repo@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
			RunWithEmptyRegistryConfig("blob", "push", ZOTHost+"/repo", WriteTempFile("blob", "test"))
			RunWithEmptyRegistryConfig("tag", ZOTHost+"/repo:tag", "tag1")
			RunWithEmptyRegistryConfig("resolve", ZOTHost+"/repo:tag")
			RunWithEmptyRegistryConfig("repo", "ls", ZOTHost)
			RunWithEmptyRegistryConfig("repo", "tags", RegistryRef(ZOTHost, "repo", ""))
			RunWithEmptyRegistryConfig("manifest", "fetch-config", ZOTHost+"/repo:tag")
		})
	})

	When("logging in", func() {
		tmpConfigName := "test.config"
		It("should succeed to use basic auth", func() {
			ORAS("login", ZOTHost, "-u", Username, "-p", Password, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				MatchContent("Login Succeeded\n").
				MatchErrKeyWords("WARNING", "Using --password via the CLI is insecure", "Use --password-stdin").Exec()
		})

		It("should show deprecation message when running with --verbose flag", func() {
			ORAS("login", ZOTHost, "-u", Username, "-p", Password, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName), "--verbose").
				MatchErrKeyWords(feature.DeprecationMessageVerboseFlag).Exec()
		})

		It("should show detailed error description if no argument provided", func() {
			err := ORAS("login").ExpectFailure().Exec().Err
			Expect(err).Should(gbytes.Say("Error"))
			Expect(err).Should(gbytes.Say("\nUsage: oras login"))
			Expect(err).Should(gbytes.Say("\n"))
			Expect(err).Should(gbytes.Say(`Run "oras login -h"`))
		})

		It("should fail if username is used with identity token", func() {
			ORAS("login", ZOTHost, "-u", Username, "--identity-token", Password).ExpectFailure().Exec()
		})

		It("should fail if password is used with identity token", func() {
			ORAS("login", ZOTHost, "-p", Password, "--identity-token", Password).ExpectFailure().Exec()
		})

		It("should fail if no username input", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20 * time.Second).
				WithInput(strings.NewReader("")).
				MatchKeyWords("username:").
				ExpectFailure().
				Exec()
		})

		It("should fail if no password input", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				MatchKeyWords("Username: ", "Password: ").
				WithInput(strings.NewReader(fmt.Sprintf("%s\n", Username))).ExpectFailure().Exec()
		})

		It("should fail if password is empty", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				MatchKeyWords("Username: ", "Password: ").
				MatchErrKeyWords("Error: password required").
				WithInput(strings.NewReader(fmt.Sprintf("%s\n\n", Username))).ExpectFailure().Exec()
		})

		It("should fail if password is wrong with registry error prefix", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				MatchKeyWords("Username: ", "Password: ").
				MatchErrKeyWords(RegistryErrorPrefix).
				WithInput(strings.NewReader(fmt.Sprintf("%s\n???\n", Username))).ExpectFailure().Exec()
		})

		It("should fail if no token input", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				MatchKeyWords("Username: ", "Token: ").
				WithInput(strings.NewReader("\n")).ExpectFailure().Exec()
		})

		It("should fail if token is empty", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				MatchKeyWords("Username: ", "Token: ").
				MatchErrKeyWords("Error: token required").
				WithInput(strings.NewReader("\n\n")).ExpectFailure().Exec()
		})

		It("should succeed to use prompted input", func() {
			ORAS("login", ZOTHost, "--registry-config", filepath.Join(GinkgoT().TempDir(), tmpConfigName)).
				WithTimeOut(20*time.Second).
				WithInput(strings.NewReader(fmt.Sprintf("%s\n%s\n", Username, Password))).
				MatchKeyWords("Username: ", "Password: ", "Login Succeeded\n").Exec()
		})

		It("should fail as the test server doesn't support token service", func() {
			ORAS("login", ZOTHost, "--identity-token", Password).
				MatchErrKeyWords("WARNING", "Using --identity-token via the CLI is insecure", "Use --identity-token-stdin").ExpectFailure().Exec()
		})

		It("should fail if --cert-file is not used with --key-file with correct error message", func() {
			ORAS("login", ZOTHost, "--cert-file", "test").
				MatchErrKeyWords("--cert-file", "in conjunction with", "--key-file").ExpectFailure().Exec()
		})
	})

	When("using legacy config", func() {
		var LegacyConfigPath = filepath.Join(TestDataRoot, LegacyConfigName)
		It("should succeed to copy", func() {
			src := RegistryRef(ZOTHost, ArtifactRepo, foobar.Tag)
			dst := RegistryRef(ZOTHost, fmt.Sprintf("command/auth/%d/copy", GinkgoRandomSeed()), foobar.Tag)
			foobarStates := append(foobar.ImageLayerStateKeys, foobar.ManifestStateKey, foobar.ImageConfigStateKey(oras.MediaTypeUnknownConfig))
			ORAS("cp", src, dst, "--from-registry-config", LegacyConfigPath, "--to-registry-config", LegacyConfigPath).MatchStatus(foobarStates, true, len(foobarStates)).Exec()
		})
	})
})

func RunWithoutLogin(args ...string) {
	ORAS(args...).ExpectFailure().
		MatchErrKeyWords("Error:", "basic credential not found").
		WithDescription("fail without logging in").Exec()
}

func RunWithInvalidCreds(args ...string) {
	ORAS(append(args, "-u", Username, "-p", Password+"1")...).ExpectFailure().
		MatchErrKeyWords(RegistryErrorPrefix).
		WithDescription("fail with invalid credentials").Exec()
}

func RunWithEmptyRegistryConfig(args ...string) {
	ORAS(append(args, "--registry-config", EmptyConfigName)...).ExpectFailure().
		MatchErrKeyWords("Error: ", fmt.Sprintf(`Please check whether the registry credential stored in the authentication file at %q is correct`, EmptyConfigName)).
		WithDescription("fail with empty registry config").
		Exec()
}
