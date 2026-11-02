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
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"oras.land/oras-go/v2/errdef"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/credentials"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"oras.land/oras-go/v2/registry/remote/retry"
	oerrors "oras.land/oras/cmd/oras/internal/errors"
	"oras.land/oras/internal/credential"
	"oras.land/oras/internal/crypto"
	onet "oras.land/oras/internal/net"
	"oras.land/oras/internal/trace"
	"oras.land/oras/internal/version"
)

// ensureAuthCaches binds the remote to the auth cache store of the current
// command invocation, if one is available.
func (remo *Remote) ensureAuthCaches(common Common) {
	if common.authCaches != nil {
		remo.authCaches = common.authCaches
	}
}

// credentialIdentity identifies the credential source of a client. Static
// credentials are identified by the credential itself; otherwise the
// credential store is identified by its config paths. The key is held only in
// memory and is compared directly, so no secret is hashed or encoded.
type credentialIdentity struct {
	cred    auth.Credential
	configs string
}

// newCredentialIdentity returns the credential identity of a client.
func newCredentialIdentity(cred auth.Credential, configs []string) credentialIdentity {
	if cred != auth.EmptyCredential {
		return credentialIdentity{cred: cred}
	}
	return credentialIdentity{configs: strings.Join(configs, "\x00")}
}

const (
	caFileFlag                 = "ca-file"
	certFileFlag               = "cert-file"
	keyFileFlag                = "key-file"
	usernameFlag               = "username"
	passwordFlag               = "password"
	passwordFromStdinFlag      = "password-stdin"
	identityTokenFlag          = "identity-token"
	identityTokenFromStdinFlag = "identity-token-stdin"
)

// Remote options struct contains flags and arguments specifying one registry.
// Remote implements oerrors.Handler and interface.
type Remote struct {
	DistributionSpec
	CACertFilePath  string
	CertFilePath    string
	KeyFilePath     string
	Insecure        bool
	Configs         []string
	Username        string
	secretFromStdin bool
	Secret          string //nolint:gosec // G117: not a hardcoded secret, this is a CLI option field
	flagPrefix      string

	resolveFlag           []string
	applyDistributionSpec bool
	headerFlags           []string
	headers               http.Header
	warned                map[string]*sync.Map
	plainHTTP             func() (plainHTTP bool, enforced bool)
	store                 credentials.Store
	authCaches            *authCacheStore
}

// EnableDistributionSpecFlag set distribution specification flag as applicable.
func (remo *Remote) EnableDistributionSpecFlag() {
	remo.applyDistributionSpec = true
}

// ApplyFlags applies flags to a command flag set.
func (remo *Remote) ApplyFlags(fs *pflag.FlagSet) {
	remo.ApplyFlagsWithPrefix(fs, "", "")
	remo.applyStdinFlags(fs)
}

func (remo *Remote) applyStdinFlags(fs *pflag.FlagSet) {
	fs.BoolVar(&remo.secretFromStdin, passwordFromStdinFlag, false, "read password from stdin")
	fs.BoolVar(&remo.secretFromStdin, identityTokenFromStdinFlag, false, "read identity token from stdin")
}

// ApplyFlagsWithPrefix applies flags to a command flag set with a prefix string.
// Commonly used for non-unary remote targets.
func (remo *Remote) ApplyFlagsWithPrefix(fs *pflag.FlagSet, prefix, description string) {
	var (
		shortUser     string
		shortPassword string
		shortHeader   string
	)
	if prefix == "" {
		shortUser, shortPassword = "u", "p"
		shortHeader = "H"
	}
	remo.flagPrefix = prefix

	if remo.applyDistributionSpec {
		remo.DistributionSpec.ApplyFlagsWithPrefix(fs, prefix, description)
	}
	fs.StringVarP(&remo.Username, remo.flagPrefix+usernameFlag, shortUser, "", description+"registry username")
	fs.StringVarP(&remo.Secret, remo.flagPrefix+passwordFlag, shortPassword, "", description+"registry password or identity token")
	fs.StringVar(&remo.Secret, remo.flagPrefix+identityTokenFlag, "", description+"registry identity token")
	fs.BoolVar(&remo.Insecure, remo.flagPrefix+"insecure", false, "allow connections to "+description+"SSL registry without certs")
	plainHTTPFlagName := remo.flagPrefix + "plain-http"
	plainHTTP := fs.Bool(plainHTTPFlagName, false, "allow insecure connections to "+description+"registry without SSL check")
	remo.plainHTTP = func() (bool, bool) {
		return *plainHTTP, fs.Changed(plainHTTPFlagName)
	}
	fs.StringVar(&remo.CACertFilePath, remo.flagPrefix+caFileFlag, "", "server certificate authority file for the remote "+description+"registry")
	fs.StringVarP(&remo.CertFilePath, remo.flagPrefix+certFileFlag, "", "", "client certificate file for the remote "+description+"registry")
	fs.StringVarP(&remo.KeyFilePath, remo.flagPrefix+keyFileFlag, "", "", "client private key file for the remote "+description+"registry")
	fs.StringArrayVar(&remo.resolveFlag, remo.flagPrefix+"resolve", nil, "customized DNS for "+description+"registry, formatted in `host:port:address[:address_port]`")
	fs.StringArrayVar(&remo.Configs, remo.flagPrefix+"registry-config", nil, "`path` of the authentication file for "+description+"registry")
	fs.StringArrayVarP(&remo.headerFlags, remo.flagPrefix+"header", shortHeader, nil, "add custom headers to "+description+"requests")
}

// CheckStdinConflict checks if PasswordFromStdin or IdentityTokenFromStdin of a
// *pflag.FlagSet conflicts with read file from input.
func CheckStdinConflict(flags *pflag.FlagSet) error {
	switch {
	case flags.Changed(passwordFromStdinFlag):
		return fmt.Errorf("`-` read file from input and `--%s` read password from input cannot be both used", passwordFromStdinFlag)
	case flags.Changed(identityTokenFromStdinFlag):
		return fmt.Errorf("`-` read file from input and `--%s` read identity token from input cannot be both used", identityTokenFromStdinFlag)
	}
	return nil
}

// Parse tries to read password with optional cmd prompt.
func (remo *Remote) Parse(cmd *cobra.Command) error {
	usernameAndIDTokenFlags := []string{remo.flagPrefix + usernameFlag, remo.flagPrefix + identityTokenFlag}
	passwordAndIDTokenFlags := []string{remo.flagPrefix + passwordFlag, remo.flagPrefix + identityTokenFlag}
	certFileAndKeyFileFlags := []string{remo.flagPrefix + certFileFlag, remo.flagPrefix + keyFileFlag}
	if cmd.Flags().Lookup(identityTokenFromStdinFlag) != nil {
		usernameAndIDTokenFlags = append(usernameAndIDTokenFlags, identityTokenFromStdinFlag)
		passwordAndIDTokenFlags = append(passwordAndIDTokenFlags, identityTokenFromStdinFlag)
	}
	if cmd.Flags().Lookup(passwordFromStdinFlag) != nil {
		passwordAndIDTokenFlags = append(passwordAndIDTokenFlags, passwordFromStdinFlag)
	}
	if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), usernameAndIDTokenFlags...); err != nil {
		return err
	}
	if err := oerrors.CheckMutuallyExclusiveFlags(cmd.Flags(), passwordAndIDTokenFlags...); err != nil {
		return err
	}
	if err := remo.parseCustomHeaders(); err != nil {
		return err
	}
	if err := oerrors.CheckRequiredTogetherFlags(cmd.Flags(), certFileAndKeyFileFlags...); err != nil {
		return err
	}
	return remo.readSecret(cmd)
}

// readSecret tries to read password or identity token with
// optional cmd prompt.
func (remo *Remote) readSecret(cmd *cobra.Command) (err error) {
	if cmd.Flags().Changed(identityTokenFlag) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "WARNING! Using --identity-token via the CLI is insecure. Use --identity-token-stdin.")
	} else if cmd.Flags().Changed(passwordFlag) {
		_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "WARNING! Using --password via the CLI is insecure. Use --password-stdin.")
	} else if remo.secretFromStdin {
		// Prompt for credential
		secret, err := io.ReadAll(os.Stdin)
		if err != nil {
			return err
		}
		remo.Secret = strings.TrimSuffix(string(secret), "\n")
		remo.Secret = strings.TrimSuffix(remo.Secret, "\r")
	}
	return nil
}

// parseResolve parses resolve flag.
func (remo *Remote) parseResolve(baseDial onet.DialFunc) (onet.DialFunc, error) {
	if len(remo.resolveFlag) == 0 {
		return baseDial, nil
	}

	formatError := func(param, message string) error {
		return fmt.Errorf("failed to parse resolve flag %q: %s", param, message)
	}
	var dialer onet.Dialer
	for _, r := range remo.resolveFlag {
		parts := strings.SplitN(r, ":", 4)
		length := len(parts)
		if length < 3 {
			return nil, formatError(r, "expecting host:port:address[:address_port]")
		}
		host := parts[0]
		hostPort, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, formatError(r, "expecting uint64 host port")
		}
		// ipv6 zone is not parsed
		address := net.ParseIP(parts[2])
		if address == nil {
			return nil, formatError(r, "invalid IP address")
		}
		addressPort := hostPort
		if length > 3 {
			addressPort, err = strconv.Atoi(parts[3])
			if err != nil {
				return nil, formatError(r, "expecting uint64 address port")
			}
		}
		dialer.Add(host, hostPort, address, addressPort)
	}
	dialer.BaseDialContext = baseDial
	return dialer.DialContext, nil
}

// tlsConfig assembles the tls config.
func (remo *Remote) tlsConfig() (*tls.Config, error) {
	config := &tls.Config{
		InsecureSkipVerify: remo.Insecure,
	}
	if remo.CACertFilePath != "" {
		var err error
		config.RootCAs, err = crypto.LoadCertPool(remo.CACertFilePath)
		if err != nil {
			return nil, err
		}
	}
	if remo.CertFilePath != "" && remo.KeyFilePath != "" {
		cert, err := tls.LoadX509KeyPair(remo.CertFilePath, remo.KeyFilePath)
		if err != nil {
			return nil, err
		}
		config.Certificates = []tls.Certificate{cert}
	}
	return config, nil
}

// registryCredentialTransport scopes client certificates and custom headers to a registry origin.
type registryCredentialTransport struct {
	registryScheme    string
	registry          string
	registryTransport http.RoundTripper
	fallbackTransport http.RoundTripper
	sensitiveHeaders  []string
}

func (t *registryCredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if sameRegistryOrigin(req.URL.Scheme, req.URL.Host, t.registryScheme, t.registry) {
		return t.registryTransport.RoundTrip(req)
	}
	if len(t.sensitiveHeaders) != 0 {
		// Clone deep-copies the header map, so the caller's request is untouched.
		req = req.Clone(req.Context())
		removeHeaders(req.Header, t.sensitiveHeaders)
	}
	return t.fallbackTransport.RoundTrip(req)
}

func removeHeaders(header http.Header, names []string) {
	for key := range header {
		for _, name := range names {
			if strings.EqualFold(key, name) {
				delete(header, key)
				break
			}
		}
	}
}

func sameRegistryOrigin(scheme, authority, registryScheme, registry string) bool {
	if !strings.EqualFold(scheme, registryScheme) {
		return false
	}
	defaultPort := "443"
	if strings.EqualFold(scheme, "http") {
		defaultPort = "80"
	}
	host, port := splitAuthority(authority, defaultPort)
	registryHost, registryPort := splitAuthority(registry, defaultPort)
	return sameHost(host, registryHost) && port == registryPort
}

// sameHost reports whether two host components denote the same host. IP
// literals are compared by parsed address so that equivalent IPv6 spellings
// (for example "::1" and "0:0:0:0:0:0:0:1") match, while zone identifiers,
// which are case-sensitive, must be identical. DNS names are compared
// case-insensitively.
func sameHost(host, registryHost string) bool {
	addr, err := netip.ParseAddr(host)
	if err != nil {
		// Not an IP literal: compare as a DNS name.
		if _, err := netip.ParseAddr(registryHost); err == nil {
			return false
		}
		return strings.EqualFold(host, registryHost)
	}
	registryAddr, err := netip.ParseAddr(registryHost)
	if err != nil {
		return false
	}
	return addr == registryAddr
}

func splitAuthority(authority, defaultPort string) (host, port string) {
	host, port, err := net.SplitHostPort(authority)
	if err == nil {
		if port == "" {
			port = defaultPort
		}
		return host, port
	}
	return strings.Trim(authority, "[]"), defaultPort
}

// authClient assembles a oras auth client.
func (remo *Remote) authClient(registry string, plainHTTP, debug bool) (client *auth.Client, err error) {
	config, err := remo.tlsConfig()
	if err != nil {
		return nil, err
	}
	registryScheme := "https"
	if plainHTTP {
		registryScheme = "http"
	}
	hasClientCertificate := len(config.Certificates) != 0
	baseTransport := http.DefaultTransport.(*http.Transport).Clone()
	baseTransport.TLSClientConfig = config
	dialContext, err := remo.parseResolve(baseTransport.DialContext)
	if err != nil {
		return nil, err
	}
	baseTransport.DialContext = dialContext

	// sensitiveHeaders are the custom --header names supplied for this registry.
	sensitiveHeaders := make([]string, 0, len(remo.headers))
	for name := range remo.headers {
		sensitiveHeaders = append(sensitiveHeaders, name)
	}
	if hasClientCertificate {
		baseTransport.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dialContext(ctx, network, address)
			if err != nil {
				return nil, err
			}
			tlsConfig := baseTransport.TLSClientConfig.Clone()
			if !sameRegistryOrigin("https", address, registryScheme, registry) {
				tlsConfig.Certificates = nil
			}
			if tlsConfig.ServerName == "" {
				tlsConfig.ServerName, _ = splitAuthority(address, "443")
			}
			tlsConn := tls.Client(conn, tlsConfig)
			if baseTransport.TLSHandshakeTimeout > 0 {
				handshakeContext, cancel := context.WithTimeout(ctx, baseTransport.TLSHandshakeTimeout)
				defer cancel()
				ctx = handshakeContext
			}
			if err := tlsConn.HandshakeContext(ctx); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return tlsConn, nil
		}
	}
	registryTransport := retry.NewTransport(baseTransport)
	var fallbackTransport http.RoundTripper = registryTransport
	if hasClientCertificate {
		withoutCertificate := baseTransport.Clone()
		fallbackConfig := config.Clone()
		fallbackConfig.Certificates = nil
		withoutCertificate.TLSClientConfig = fallbackConfig
		fallbackTransport = retry.NewTransport(withoutCertificate)
	}
	var transport http.RoundTripper = registryTransport
	if hasClientCertificate || len(sensitiveHeaders) != 0 {
		transport = &registryCredentialTransport{
			registryScheme:    registryScheme,
			registry:          registry,
			registryTransport: registryTransport,
			fallbackTransport: fallbackTransport,
			sensitiveHeaders:  sensitiveHeaders,
		}
	}

	cred := remo.Credential()
	if remo.authCaches == nil {
		remo.authCaches = newAuthCacheStore()
	}
	cache := remo.authCaches.cache(newCredentialIdentity(cred, remo.Configs))

	client = &auth.Client{
		Client: &http.Client{
			// http.RoundTripper with a retry using the DefaultPolicy
			// see: https://pkg.go.dev/oras.land/oras-go/v2/registry/remote/retry#Policy
			Transport: transport,
		},
		Cache:  cache,
		Header: remo.headers.Clone(),
	}
	client.SetUserAgent("oras/" + version.GetVersion())
	if debug {
		client.Client.Transport = trace.NewTransport(client.Client.Transport, sensitiveHeaders...)
	}

	if cred != auth.EmptyCredential {
		client.Credential = func(_ context.Context, _ string) (auth.Credential, error) {
			return cred, nil
		}
	} else {
		var err error
		remo.store, err = credential.NewStore(remo.Configs...)
		if err != nil {
			return nil, err
		}
		client.Credential = credentials.Credential(remo.store)
	}

	return
}

// ConfigPath returns the config path of the credential store.
func (remo *Remote) ConfigPath() (string, error) {
	if remo.store == nil {
		return "", errors.New("no credential store initialized")
	}
	if ds, ok := remo.store.(*credentials.DynamicStore); ok {
		return ds.ConfigPath(), nil
	}
	return "", errors.New("store doesn't support getting config path")
}

func (remo *Remote) parseCustomHeaders() error {
	if len(remo.headerFlags) != 0 {
		headers := map[string][]string{}
		for _, h := range remo.headerFlags {
			name, value, found := strings.Cut(h, ":")
			if !found || strings.TrimSpace(name) == "" {
				// In conformance to the RFC 2616 specification
				// Reference: https://www.rfc-editor.org/rfc/rfc2616#section-4.2
				return fmt.Errorf("invalid header: %q", h)
			}
			headers[name] = append(headers[name], value)
		}
		remo.headers = headers
	}
	return nil
}

// Credential returns a credential based on the remote options.
func (remo *Remote) Credential() auth.Credential {
	return credential.Credential(remo.Username, remo.Secret)
}

func (remo *Remote) handleWarning(registry string, logger logrus.FieldLogger) func(warning remote.Warning) {
	if remo.warned == nil {
		remo.warned = make(map[string]*sync.Map)
	}
	warned := remo.warned[registry]
	if warned == nil {
		warned = &sync.Map{}
		remo.warned[registry] = warned
	}
	logger = logger.WithField("registry", registry)
	return func(warning remote.Warning) {
		if _, loaded := warned.LoadOrStore(warning.WarningValue, struct{}{}); !loaded {
			logger.Warn(warning.Text)
		}
	}
}

// NewRegistry assembles a oras remote registry.
func (remo *Remote) NewRegistry(registry string, common Common, logger logrus.FieldLogger) (reg *remote.Registry, err error) {
	reg, err = remote.NewRegistry(registry)
	if err != nil {
		return nil, err
	}
	registry = reg.Reference.Registry
	reg.PlainHTTP = remo.isPlainHTTP(registry)
	reg.HandleWarning = remo.handleWarning(registry, logger)
	remo.ensureAuthCaches(common)
	if reg.Client, err = remo.authClient(reg.Reference.Host(), reg.PlainHTTP, common.Debug); err != nil {
		return nil, err
	}
	return
}

// NewRepository assembles a oras remote repository.
func (remo *Remote) NewRepository(reference string, common Common, logger logrus.FieldLogger) (repo *remote.Repository, err error) {
	repo, err = remote.NewRepository(reference)
	if err != nil {
		if errors.Is(err, errdef.ErrInvalidReference) {
			return nil, fmt.Errorf("%q: %w", reference, err)
		}
		return nil, err
	}
	registry := repo.Reference.Registry
	repo.PlainHTTP = remo.isPlainHTTP(registry)
	repo.HandleWarning = remo.handleWarning(registry, logger)
	remo.ensureAuthCaches(common)
	if repo.Client, err = remo.authClient(repo.Reference.Host(), repo.PlainHTTP, common.Debug); err != nil {
		return nil, err
	}
	repo.SkipReferrersGC = true
	if remo.ReferrersAPI != ReferrersStateUnknown {
		if err := repo.SetReferrersCapability(remo.ReferrersAPI == ReferrersStateSupported); err != nil {
			return nil, err
		}
	}
	return
}

// isPlainHTTP returns the plain http flag for a given registry.
func (remo *Remote) isPlainHTTP(registry string) bool {
	plainHTTP, enforced := remo.plainHTTP()
	if enforced {
		return plainHTTP
	}
	host, _, _ := net.SplitHostPort(registry)
	if host == "localhost" || registry == "localhost" {
		// not specified, defaults to plain http for localhost
		return true
	}
	return plainHTTP
}

// ModifyError modifies error during cmd execution.
func (remo *Remote) ModifyError(cmd *cobra.Command, err error) (bool, error) {
	if errors.Is(err, auth.ErrBasicCredentialNotFound) {
		return true, remo.DecorateCredentialError(err)
	}

	var errResp *errcode.ErrorResponse
	if errors.As(err, &errResp) {
		cmd.SetErrPrefix(oerrors.RegistryErrorPrefix)
		return true, &oerrors.Error{
			Err: oerrors.ReportErrResp(errResp),
		}
	}
	return false, err
}

// DecorateCredentialError decorate error with recommendation.
func (remo *Remote) DecorateCredentialError(err error) *oerrors.Error {
	configPath := " "
	if path, pathErr := remo.ConfigPath(); pathErr == nil {
		configPath += fmt.Sprintf("at %q ", path)
	}
	return &oerrors.Error{
		Err:            oerrors.TrimErrBasicCredentialNotFound(err),
		Recommendation: fmt.Sprintf(`Please check whether the registry credential stored in the authentication file%sis correct`, configPath),
	}
}
