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
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/oras-project/oras-go/v3/errdef"
	"github.com/oras-project/oras-go/v3/registry/remote"
	remoteconfig "github.com/oras-project/oras-go/v3/registry/remote/config"
	"github.com/oras-project/oras-go/v3/registry/remote/credentials"
	"github.com/oras-project/oras-go/v3/registry/remote/policy"
	"github.com/oras-project/oras-go/v3/registry/remote/properties"
	"github.com/oras-project/oras-go/v3/registry/remote/signature"
	"github.com/sirupsen/logrus"
	"oras.land/oras/internal/version"
)

// ContainersConfigEnv is the environment variable that enables the
// experimental containers configuration support. When it is set to a true
// value ("1", "true", ...), registry access is configured from the container
// ecosystem files in their default locations, as podman and buildah are:
//
//   - registries.conf: aliases, blocked registries, location rewrites,
//     mirrors, insecure, token-flow, referrers-api and blocked-realm-prefixes
//   - certs.d: per-registry CA and client certificates
//   - auth.json: credentials, after the oras credential store
//   - policy.json and registries.d: pull policy and signedBy verification
//
// Command line flags take precedence over the files.
const ContainersConfigEnv = "ORAS_EXPERIMENTAL_CONTAINERS_CONFIG"

// containersConfigEnabled reports whether ContainersConfigEnv is set to a true
// value.
func containersConfigEnabled() bool {
	enabled, _ := strconv.ParseBool(os.Getenv(ContainersConfigEnv))
	return enabled
}

// loadContainersConfigs loads the containers configuration files once per
// process. Missing files are skipped; a file that cannot be parsed is an
// error.
var loadContainersConfigs = sync.OnceValues(func() (*remoteconfig.Configs, error) {
	configs, err := remoteconfig.LoadConfigs()
	if err != nil {
		return nil, fmt.Errorf("failed to load containers configuration (%s is set): %w", ContainersConfigEnv, err)
	}
	return configs, nil
})

// containersCredentialStore returns the credentials from Docker config.json
// and containers auth.json to fall back on after the oras credential store,
// or nil when --registry-config selects the credential files.
func (remo *Remote) containersCredentialStore(configs *remoteconfig.Configs) credentials.Store {
	if len(remo.Configs) != 0 {
		return nil
	}
	store, err := configs.CredentialStore(credentials.StoreOptions{})
	if err != nil {
		// No credential file was found.
		return nil
	}
	return store
}

// newConfiguredRepository assembles a remote repository configured from the
// containers configuration files.
func (remo *Remote) newConfiguredRepository(reference string, common Common, logger logrus.FieldLogger) (*remote.Repository, error) {
	configs, err := loadContainersConfigs()
	if err != nil {
		return nil, err
	}
	props, err := configs.RegistryProperties(reference)
	if err != nil {
		if errors.Is(err, errdef.ErrInvalidReference) {
			return nil, fmt.Errorf("%q: %w", reference, err)
		}
		return nil, err
	}
	fallbackStore := remo.containersCredentialStore(configs)
	evaluator, err := containersPolicyEvaluator(configs)
	if err != nil {
		return nil, err
	}

	// The builder assembles the repository with its mirrors and policy. The
	// primary registry client is then replaced with the oras client, which
	// carries --resolve, --header, the client certificate scoping and the
	// --debug trace.
	builder := remote.NewClientBuilder()
	builder.UserAgent = "oras/" + version.GetVersion()
	builder.PolicyEvaluator = evaluator
	if fallbackStore != nil {
		builder.CredentialStore = fallbackStore
	}
	if props.Attributes.TokenFlow == properties.TokenFlowDefault {
		// Keep the distribution-spec token flow that oras uses without the
		// containers configuration, for mirrors too.
		props.Attributes.TokenFlow = properties.TokenFlowDistribution
	}
	repo, err := builder.NewRepository(props)
	if err != nil {
		return nil, err
	}

	registry := repo.Registry.Reference.Registry
	repo.Registry.PlainHTTP = remo.isPlainHTTP(registry)
	repo.Registry.HandleWarning = remo.handleWarning(registry, logger)
	if repo.Registry.Client, err = remo.authClientWithProperties(repo.Reference().Host(), repo.Registry.PlainHTTP, common.Debug, props, fallbackStore); err != nil {
		return nil, err
	}
	repo.SkipReferrersGC = true
	// The --distribution-spec flag takes precedence over referrers-api in
	// registries.conf.
	if remo.ReferrersAPI != ReferrersStateUnknown {
		repo.SetReferrersCapability(remo.ReferrersAPI == ReferrersStateSupported)
	}
	return repo, nil
}

// newConfiguredRegistry assembles a remote registry configured from the
// containers configuration files. Only the entries that apply to the registry
// host are used, and the registry is not rewritten, so that credentials are
// stored under the name given on the command line.
func (remo *Remote) newConfiguredRegistry(registry string, common Common, logger logrus.FieldLogger) (*remote.Registry, error) {
	configs, err := loadContainersConfigs()
	if err != nil {
		return nil, err
	}
	reg, err := remote.NewRegistry(registry)
	if err != nil {
		return nil, err
	}
	registry = reg.Reference.Registry
	props, err := registryHostProperties(configs, reg.Reference)
	if err != nil {
		return nil, err
	}
	reg.PlainHTTP = remo.isPlainHTTP(registry)
	reg.HandleWarning = remo.handleWarning(registry, logger)
	if reg.Client, err = remo.authClientWithProperties(reg.Reference.Host(), reg.PlainHTTP, common.Debug, props, remo.containersCredentialStore(configs)); err != nil {
		return nil, err
	}
	return reg, nil
}

// registryHostProperties resolves the registries.conf entry and certs.d
// certificates that apply to a registry host. Configs.RegistryProperties
// cannot be used because it requires a repository.
func registryHostProperties(configs *remoteconfig.Configs, ref properties.Reference) (*properties.Registry, error) {
	props := properties.NewRegistryFromReference(ref)
	if rc := configs.RegistriesConfig; rc != nil {
		if rc.IsBlocked(ref.Registry) {
			return nil, fmt.Errorf("%w: %s", remoteconfig.ErrRegistryBlocked, ref.Registry)
		}
		if entry := rc.FindRegistry(ref.Registry); entry != nil {
			props.Transport.Insecure = entry.Insecure
			switch strings.ToLower(entry.TokenFlow) {
			case "":
			case "oauth2":
				props.Attributes.TokenFlow = properties.TokenFlowOAuth2
			case "distribution":
				props.Attributes.TokenFlow = properties.TokenFlowDistribution
			default:
				return nil, fmt.Errorf("invalid token-flow %q for registry %q: must be %q or %q",
					entry.TokenFlow, entry.Prefix, "oauth2", "distribution")
			}
		}
	}
	if len(configs.CertsDirPaths) != 0 {
		certs, err := remoteconfig.LoadCertsDirFromPaths(ref.Host(), configs.CertsDirPaths)
		if err != nil {
			return nil, fmt.Errorf("failed to load certs for %s: %w", ref.Host(), err)
		}
		if certs != nil {
			certs.ApplyToTransport(&props.Transport)
		}
	}
	return props, nil
}

// containersPolicyEvaluator builds the policy.json evaluator, verifying
// signedBy requirements against the registries.d lookaside storage. It returns
// nil when no policy.json was found.
func containersPolicyEvaluator(configs *remoteconfig.Configs) (*policy.Evaluator, error) {
	var opts []policy.EvaluatorOption
	if configs.RegistriesDConfig != nil {
		opts = append(opts, policy.WithSignedByVerifier(signature.NewSignedByVerifierFromConfig(configs.RegistriesDConfig)))
	}
	evaluator, err := configs.PolicyEvaluator(opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load policy.json: %w", err)
	}
	return evaluator, nil
}
