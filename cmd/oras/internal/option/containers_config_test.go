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

package option

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry/remote"
	"github.com/oras-project/oras-go/v3/registry/remote/auth"
	remoteconfig "github.com/oras-project/oras-go/v3/registry/remote/config"
	"github.com/oras-project/oras-go/v3/registry/remote/properties"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// useContainersConfig enables the containers configuration for the test and
// loads registries.conf from the given content, with every other config file
// pointed at a missing path.
func useContainersConfig(t *testing.T, registriesConf string) {
	t.Helper()
	t.Setenv(ContainersConfigEnv, "true")
	dir := t.TempDir()
	confPath := filepath.Join(dir, "registries.conf")
	if err := os.WriteFile(confPath, []byte(registriesConf), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing")
	configs, err := remoteconfig.LoadConfigsWithOptions(remoteconfig.LoadConfigsOptions{
		DockerConfigPath:     missing,
		ContainersAuthPath:   missing,
		RegistriesConfigPath: confPath,
		PolicyConfigPath:     missing,
		RegistriesDPath:      missing,
		CertsDirPaths:        []string{missing},
	})
	if err != nil {
		t.Fatal(err)
	}
	setContainersConfigLoader(t, func() (*remoteconfig.Configs, error) { return configs, nil })
}

func setContainersConfigLoader(t *testing.T, loader func() (*remoteconfig.Configs, error)) {
	t.Helper()
	original := loadContainersConfigs
	loadContainersConfigs = loader
	t.Cleanup(func() { loadContainersConfigs = original })
}

func TestContainersConfigEnabled(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"0", false},
		{"false", false},
		{"not-a-bool", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			t.Setenv(ContainersConfigEnv, tt.value)
			if got := containersConfigEnabled(); got != tt.want {
				t.Errorf("containersConfigEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}

const rewriteRegistriesConf = `
[[registry]]
prefix = "alias.example/team"
location = "localhost:5000/real"
`

func TestRemote_NewRepository_containersConfigRewrite(t *testing.T) {
	useContainersConfig(t, rewriteRegistriesConf)
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	repo, err := opts.NewRepository("alias.example/team/app", Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if got, want := repo.Reference().Registry, "localhost:5000"; got != want {
		t.Errorf("registry = %q, want %q", got, want)
	}
	if got, want := repo.Reference().Repository, "real/app"; got != want {
		t.Errorf("repository = %q, want %q", got, want)
	}
	if !repo.Registry.PlainHTTP {
		t.Error("PlainHTTP = false, want true for the rewritten localhost registry")
	}
	if !repo.SkipReferrersGC {
		t.Error("SkipReferrersGC = false, want true")
	}
}

func TestRemote_containersConfigWarningDedup(t *testing.T) {
	useContainersConfig(t, rewriteRegistriesConf)
	logger, hook := test.NewNullLogger()
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	repo, err := opts.NewRepository("alias.example/team/app", Common{}, logger)
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	reg, err := opts.NewRegistry("localhost:5000", Common{}, logger)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	warning := remote.Warning{WarningValue: remote.WarningValue{Code: 299, Agent: "-", Text: "foo"}}
	repo.Registry.HandleWarning(warning)
	reg.HandleWarning(warning)

	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("got %d log entries, want 1: %v", len(entries), entries)
	}
	if got, want := entries[0].Data["registry"], "localhost:5000"; got != want {
		t.Errorf("registry = %v, want %v", got, want)
	}
}

func TestRemote_NewRepository_containersConfigDisabled(t *testing.T) {
	useContainersConfig(t, rewriteRegistriesConf)
	t.Setenv(ContainersConfigEnv, "")
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	repo, err := opts.NewRepository("alias.example/team/app", Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if got, want := repo.Reference().Registry, "alias.example"; got != want {
		t.Errorf("registry = %q, want %q: registries.conf must be ignored without %s", got, want, ContainersConfigEnv)
	}
}

func TestRemote_containersConfigBlocked(t *testing.T) {
	useContainersConfig(t, `
[[registry]]
location = "blocked.example"
blocked = true
`)
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	if _, err := opts.NewRepository("blocked.example/app", Common{}, logrus.New()); !errors.Is(err, remoteconfig.ErrRegistryBlocked) {
		t.Errorf("NewRepository() error = %v, want %v", err, remoteconfig.ErrRegistryBlocked)
	}
	if _, err := opts.NewRegistry("blocked.example", Common{}, logrus.New()); !errors.Is(err, remoteconfig.ErrRegistryBlocked) {
		t.Errorf("NewRegistry() error = %v, want %v", err, remoteconfig.ErrRegistryBlocked)
	}
}

func TestRemote_NewRegistry_containersConfig(t *testing.T) {
	useContainersConfig(t, `
[[registry]]
location = "secure.example"
insecure = true
token-flow = "oauth2"
`)
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	reg, err := opts.NewRegistry("secure.example", Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	if got, want := reg.Reference.Registry, "secure.example"; got != want {
		t.Errorf("registry = %q, want %q", got, want)
	}
	if reg.Client == nil {
		t.Fatal("Client = nil")
	}
}

func TestRemote_NewRegistry_containersConfigInvalidTokenFlow(t *testing.T) {
	useContainersConfig(t, `
[[registry]]
location = "bad.example"
token-flow = "bogus"
`)
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	if _, err := opts.NewRegistry("bad.example", Common{}, logrus.New()); err == nil || !strings.Contains(err.Error(), "invalid token-flow") {
		t.Errorf("NewRegistry() error = %v, want invalid token-flow", err)
	}
}

func TestRemote_containersConfigLoadError(t *testing.T) {
	t.Setenv(ContainersConfigEnv, "1")
	wantErr := errors.New("bad registries.conf")
	setContainersConfigLoader(t, func() (*remoteconfig.Configs, error) { return nil, wantErr })
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	if _, err := opts.NewRepository("localhost:5000/app", Common{}, logrus.New()); !errors.Is(err, wantErr) {
		t.Errorf("NewRepository() error = %v, want %v", err, wantErr)
	}
	if _, err := opts.NewRegistry("localhost:5000", Common{}, logrus.New()); !errors.Is(err, wantErr) {
		t.Errorf("NewRegistry() error = %v, want %v", err, wantErr)
	}
}

// TestRemote_NewRepository_containersConfigResolve resolves a tag through an
// alias that registries.conf rewrites to a test registry, so the request must
// reach the rewritten location through the oras client, carrying --header.
func TestRemote_NewRepository_containersConfigResolve(t *testing.T) {
	const manifest = `{"schemaVersion":2,"mediaType":"application/vnd.oci.image.manifest.v1+json","config":{"mediaType":"application/vnd.oci.empty.v1+json","digest":"sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a","size":2},"layers":[]}`
	var gotHeader string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/real/app/manifests/v1" {
			http.NotFound(w, r)
			return
		}
		gotHeader = r.Header.Get("X-Test")
		w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
		w.Header().Set("Content-Length", fmt.Sprint(len(manifest)))
		w.Header().Set("Docker-Content-Digest", digest.FromString(manifest).String())
		if r.Method == http.MethodGet {
			_, _ = w.Write([]byte(manifest))
		}
	}))
	defer ts.Close()
	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatal(err)
	}

	useContainersConfig(t, fmt.Sprintf(`
[[registry]]
prefix = "alias.example/team"
location = "%s/real"
`, u.Host))
	opts := Remote{plainHTTP: plainHTTPEnabled, headerFlags: []string{"X-Test: value"}}
	if err := opts.parseCustomHeaders(); err != nil {
		t.Fatal(err)
	}
	repo, err := opts.NewRepository("alias.example/team/app", Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	desc, err := repo.Resolve(context.Background(), "v1")
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if desc.Size != int64(len(manifest)) {
		t.Errorf("Resolve() size = %d, want %d", desc.Size, len(manifest))
	}
	if desc.Digest != digest.FromString(manifest) {
		t.Errorf("Resolve() digest = %s, want %s", desc.Digest, digest.FromString(manifest))
	}
	if strings.TrimSpace(gotHeader) != "value" {
		t.Errorf("X-Test header = %q, want %q", gotHeader, "value")
	}
}

func TestRegistryHostProperties_noRegistriesConf(t *testing.T) {
	props, err := registryHostProperties(&remoteconfig.Configs{}, properties.Reference{Registry: "example.com"})
	if err != nil {
		t.Fatalf("registryHostProperties() error = %v", err)
	}
	if props.Transport.Insecure || props.Attributes.TokenFlow != properties.TokenFlowDefault {
		t.Errorf("registryHostProperties() = %+v, want defaults", props)
	}
}

func TestRemote_tlsConfig_transportProperties(t *testing.T) {
	opts := Remote{}
	config, err := opts.tlsConfig(&properties.Transport{Insecure: true})
	if err != nil {
		t.Fatalf("tlsConfig() error = %v", err)
	}
	if !config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true from registries.conf insecure")
	}
	if _, err := opts.tlsConfig(&properties.Transport{CACerts: []string{filepath.Join(t.TempDir(), "missing.crt")}}); err == nil {
		t.Error("tlsConfig() error = nil, want error for a missing certs.d CA file")
	}
	if _, err := opts.tlsConfig(&properties.Transport{Cert: "missing.cert", Key: "missing.key"}); err == nil {
		t.Error("tlsConfig() error = nil, want error for a missing certs.d client certificate")
	}
	config, err = opts.tlsConfig(nil)
	if err != nil {
		t.Fatalf("tlsConfig(nil) error = %v", err)
	}
	if config.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = true, want false without properties")
	}
}

// useFullContainersConfig enables the containers configuration with every
// file present: registries.conf, auth.json, policy.json, registries.d and
// certs.d for host.
func useFullContainersConfig(t *testing.T, host, registriesConf string) {
	t.Helper()
	t.Setenv(ContainersConfigEnv, "true")
	dir := t.TempDir()
	write := func(name string, content []byte) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, content, 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	auth := base64.StdEncoding.EncodeToString([]byte("authuser:authpass"))
	opts := remoteconfig.LoadConfigsOptions{
		DockerConfigPath:     filepath.Join(dir, "missing"),
		ContainersAuthPath:   write("auth.json", fmt.Appendf(nil, `{"auths":{%q:{"auth":%q}}}`, host, auth)),
		RegistriesConfigPath: write("registries.conf", []byte(registriesConf)),
		PolicyConfigPath:     write("policy.json", []byte(`{"default":[{"type":"insecureAcceptAnything"}]}`)),
		RegistriesDPath:      filepath.Dir(write("registries.d/default.yaml", []byte("default-docker:\n  lookaside: file:///tmp/sigstore\n"))),
		CertsDirPaths:        []string{filepath.Join(dir, "certs.d")},
	}
	write(filepath.Join("certs.d", host, "ca.crt"), localhostServerCert)
	write(filepath.Join("certs.d", host, "client.cert"), localhostClientCert)
	write(filepath.Join("certs.d", host, "client.key"), testingKey(localhostClientKey))
	configs, err := remoteconfig.LoadConfigsWithOptions(opts)
	if err != nil {
		t.Fatal(err)
	}
	setContainersConfigLoader(t, func() (*remoteconfig.Configs, error) { return configs, nil })
}

func TestRemote_NewRepository_containersConfigFullStack(t *testing.T) {
	const host = "registry.example:5000"
	useFullContainersConfig(t, host, fmt.Sprintf(`
[[registry]]
location = %q
token-flow = "oauth2"
referrers-api = "unsupported"
`, host))
	ctx := context.Background()

	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	repo, err := opts.NewRepository(host+"/app", Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if repo.Registry.Policy == nil {
		t.Error("Policy = nil, want the policy.json evaluator")
	}
	client, ok := repo.Registry.Client.(*auth.Client)
	if !ok {
		t.Fatalf("Client = %T, want *auth.Client", repo.Registry.Client)
	}
	if fetcher, ok := client.TokenFetcher.(*auth.CompositeTokenFetcher); !ok || !fetcher.StrictOAuth2 {
		t.Errorf("TokenFetcher = %#v, want a strict OAuth2 fetcher for token-flow = oauth2", client.TokenFetcher)
	}
	cred, err := client.CredentialFunc(ctx, properties.Resource{Registry: host})
	if err != nil {
		t.Fatalf("CredentialFunc() error = %v", err)
	}
	if cred.Username != "authuser" || cred.Password != "authpass" {
		t.Errorf("CredentialFunc() = %+v, want the auth.json credential", cred)
	}

	// --registry-config selects the credential files, so auth.json is skipped.
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	opts = Remote{plainHTTP: plainHTTPNotSpecified, Configs: []string{configPath}}
	repo, err = opts.NewRepository(host+"/app", Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	cred, err = repo.Registry.Client.(*auth.Client).CredentialFunc(ctx, properties.Resource{Registry: host})
	if err != nil {
		t.Fatalf("CredentialFunc() error = %v", err)
	}
	if !cred.IsEmpty() {
		t.Errorf("CredentialFunc() = %+v, want no credential with --registry-config", cred)
	}
}

func TestRemote_NewRegistry_containersConfigFullStack(t *testing.T) {
	const host = "registry.example:5000"
	useFullContainersConfig(t, host, fmt.Sprintf(`
[[registry]]
location = %q
token-flow = "distribution"
`, host))
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	reg, err := opts.NewRegistry(host, Common{}, logrus.New())
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	client, ok := reg.Client.(*auth.Client)
	if !ok {
		t.Fatalf("Client = %T, want *auth.Client", reg.Client)
	}
	if fetcher, ok := client.TokenFetcher.(*auth.CompositeTokenFetcher); !ok || fetcher.StrictOAuth2 {
		t.Errorf("TokenFetcher = %#v, want the distribution-spec fetcher", client.TokenFetcher)
	}
}

func TestRemote_NewRepository_containersConfigInvalidReference(t *testing.T) {
	useContainersConfig(t, "")
	opts := Remote{plainHTTP: plainHTTPNotSpecified}
	if _, err := opts.NewRepository("localhost:5000/INVALID", Common{}, logrus.New()); !errors.Is(err, errdef.ErrInvalidReference) {
		t.Errorf("NewRepository() error = %v, want %v", err, errdef.ErrInvalidReference)
	}
}

func TestRemote_tlsConfig_transportCACert(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.crt")
	if err := os.WriteFile(caPath, localhostServerCert, 0o600); err != nil {
		t.Fatal(err)
	}
	opts := Remote{}
	config, err := opts.tlsConfig(&properties.Transport{CACert: caPath})
	if err != nil {
		t.Fatalf("tlsConfig() error = %v", err)
	}
	if config.RootCAs == nil {
		t.Error("RootCAs = nil, want the CA certificate")
	}
}
