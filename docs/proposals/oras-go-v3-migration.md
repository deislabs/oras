# oras-go v3 Migration: Deprecation Fixes & New Feature Adoption

## Overview

This document tracks the full migration from oras-go v2 to v3, covering both the mechanical breaking-change fixes and the new v3 features worth adopting in the ORAS CLI. The basic import-path and API-rename migration is handled in PR #2002; this document plans the remaining work.

## Problem Statement & Motivation

oras-go v3 introduces a new module path (`github.com/oras-project/oras-go/v3`), several breaking API renames, and a set of new packages that provide capabilities the ORAS CLI currently either lacks entirely or implements through duplicated custom code. Fully adopting v3 means:

- Removing duplicated implementations (`internal/cache`, `internal/trace`) that are now first-class in the library.
- Fixing a process-safety bug in the cache (`oci.New` vs `oci.NewStorage`).
- Unlocking new user-facing capabilities: Podman auth.json credentials, per-registry TLS from `certs.d`, and automatic registry mirrors via `registries.conf`.

Reference: [`oras-go/docs/SCENARIOS.md`](https://github.com/oras-project/oras-go/blob/main/docs/SCENARIOS.md) and [`oras-go/docs/migration-v3/`](https://github.com/oras-project/oras-go/blob/main/docs/migration-v3/).

## Background & Context

### What PR #2002 Already Covers

The branch `feat/migrate-to-oras-go-v3` completes the mechanical migration:

| Status | Change |
|--------|--------|
| ✅ Done | Import path `oras.land/oras-go/v2` → `github.com/oras-project/oras-go/v3` |
| ✅ Done | `auth.Credential{}` → `credentials.Credential{}` |
| ✅ Done | `client.Credential =` → `client.CredentialFunc =` |
| ✅ Done | `credentials.Login/Logout` → `remote.Login/Logout` |
| ✅ Done | `repo.Client/PlainHTTP/HandleWarning` → `repo.Registry.*` |
| ✅ Done | `repo.Reference.Repository` → `repo.RepositoryName` |
| ✅ Done | `ref.Reference` (field) → `ref.GetReference()` |
| ✅ Done | `SetReferrersCapability()` — void, no longer returns error |

### New Packages in oras-go v3

| Package | Description |
|---------|-------------|
| `registry/remote/config` | Unified loader for Docker config.json, containers auth.json, registries.conf, policy.json, certs.d |
| `registry/remote/properties` | Typed registry configuration (`Registry`, `Transport`, `Mirror`) |
| `registry/remote/builder.go` | `ClientBuilder` factory: TLS, retry, credentials, user-agent, debug logging in one place |
| `registry/remote/middleware.go` | `RepositoryMiddleware`, `WithPolicyEnforcement`, `Compose` |
| `registry/remote/mirror.go` | Mirror fallback with pull policies |
| `registry/remote/policy` | `containers-policy.json` enforcement |
| `registry/remote/signature` | Atomic container signature signing/verification (OpenPGP, lookaside) |
| `content/cache` | Process-safe caching wrapper (`CacheReadOnlyTarget`, `Cache`, `NewFromEnv`) |
| `objects` | ORM-like API for OCI images, artifacts, and image indexes |

## Scenarios

**Enterprise/air-gapped users** rely on `registries.conf` to configure mirrors and blocked registries. Currently ORAS ignores this file; after adopting `config.LoadConfigs()`, mirrors are tried automatically on every pull operation.

**Podman users** store credentials in `containers/auth.json`, not Docker's `config.json`. Currently ORAS only reads Docker's format; `config.LoadConfigs()` reads both.

**Operators** in regulated environments may have per-registry CA certificates in `certs.d`. Currently these require a `--ca-file` flag on every command; after adopting the full config stack this happens automatically.

**Developers running parallel pulls** in CI share an `ORAS_CACHE` directory across processes. The current `oci.New()` based cache is not safe for concurrent writes; `oci.NewStorage()` fixes this silently.

## Proposal

### Part 1 — Remaining Deprecations (Bugs / Code Duplication)

#### 1.1 Fix Cache Process-Safety: `oci.New()` → `oci.NewStorage()`

**File:** `cmd/oras/internal/option/cache.go:34`

`oci.New()` maintains an `index.json` file that is not safe for concurrent process access. `oci.NewStorage()` omits `index.json` writes and is designed for shared cache use. (SCENARIOS.md §8.)

```go
// Before
ociStore, err := oci.New(opts.Root)

// After — process-safe for shared ORAS_CACHE directories
ociStore, err := oci.NewStorage(opts.Root)
```

Also update `option/cache_test.go` which uses `oci.New()` directly.

#### 1.2 Delete `internal/cache/` — Use `oras-go/content/cache`

**Files:** `internal/cache/target.go`, `internal/cache/target_test.go`, `cmd/oras/internal/option/cache.go`

`internal/cache/target.go` reimplements `CacheReadOnlyTarget`, `Fetch`, `FetchReference`, and `Exists` caching logic. This is now first-class in `github.com/oras-project/oras-go/v3/content/cache`. The library version also correctly uses `oci.NewStorage()` internally.

```go
// option/cache.go — After
import "github.com/oras-project/oras-go/v3/content/cache"

func (opts *Cache) CachedTarget(src oras.ReadOnlyTarget) (oras.ReadOnlyTarget, error) {
    opts.Root = os.Getenv("ORAS_CACHE")
    if opts.Root != "" {
        c := &cache.Cache{Root: opts.Root}
        return c.ReadOnlyTarget(src) // uses oci.NewStorage internally
    }
    return src, nil
}
```

Net reduction: ~100 lines of duplicated implementation removed.

#### 1.3 Replace Custom `handleWarning()` with `remote.NewWarningLogger()`

**File:** `cmd/oras/internal/option/remote.go:327-342`

The custom `handleWarning()` function with its `sync.Map` de-duplication tracks exactly what `remote.NewWarningLogger(registry, *slog.Logger)` provides in v3. (SCENARIOS.md §11.)

The complication is that ORAS uses `logrus` while `NewWarningLogger` takes `*slog.Logger`. The fix requires a small slog→logrus bridge handler:

```go
// internal/logutil/slog_bridge.go
type logrusHandler struct {
    logger logrus.FieldLogger
    attrs  []slog.Attr
}

func (h *logrusHandler) Handle(_ context.Context, r slog.Record) error {
    entry := h.logger
    r.Attrs(func(a slog.Attr) bool {
        entry = entry.WithField(a.Key, a.Value.Any())
        return true
    })
    entry.Warn(r.Message)
    return nil
}
// + Enabled, WithAttrs, WithGroup stubs
```

Then replace `handleWarning()`:

```go
// option/remote.go — After
import (
    "log/slog"
    "oras.land/oras/internal/logutil"
)

// In NewRepository() and NewRegistry():
slogLogger := slog.New(logutil.NewLogrusHandler(logger))
repo.Registry.HandleWarning = remote.NewWarningLogger(registry, slogLogger)
```

This removes `warned map[string]*sync.Map` from the `Remote` struct, removes the init code in `binary_target.go:63-64`, and eliminates the per-registry state tracking (~25 lines).

#### 1.4 Replace Custom Trace Transport with `remote.NewLoggingTransport()`

**File:** `internal/trace/transport.go`

`internal/trace/transport.go` (~160 lines) implements HTTP debug logging with auth header scrubbing and token body redaction. `remote.NewLoggingTransport(inner, *slog.Logger)` in v3 provides identical behavior with the same 16 KiB body cap, same header scrubbing (`Authorization`, `Set-Cookie`), same token field redaction, and adds sequential request IDs for correlating concurrent requests.

With `ClientBuilder` adoption (Part 2 below), debug logging is wired via `builder.Logger`:

```go
// option/remote.go — in newClientBuilder()
if debug {
    builder.Logger = slog.New(logutil.NewLogrusHandler(logger))
    // ClientBuilder internally wraps transport as:
    // LoggingTransport → retry.Transport → http.Transport (TLS)
}
```

Once `ClientBuilder` is adopted, `internal/trace/transport.go` and its call site can be deleted (~160 lines removed).

### Part 2 — New v3 Features to Implement

#### 2.1 `ClientBuilder` + `properties.Registry` in `NewRepository()`/`NewRegistry()`

**File:** `cmd/oras/internal/option/remote.go`

**Current approach:** `authClient()` manually assembles `tls.Config` → `http.Transport` → `retry.Transport` → `auth.Client`, then `NewRepository()` sets `repo.Registry.*` fields individually. This is 40+ lines of plumbing that `ClientBuilder` handles in one place.

**New approach:**

```go
func (remo *Remote) newClientBuilder(debug bool) (*remote.ClientBuilder, error) {
    builder := remote.NewClientBuilder()
    builder.UserAgent = "oras/" + version.GetVersion()

    // DNS resolve: set on BaseTransport
    baseTransport := http.DefaultTransport.(*http.Transport).Clone()
    dialContext, err := remo.parseResolve(baseTransport.DialContext)
    if err != nil {
        return nil, err
    }
    baseTransport.DialContext = dialContext
    builder.BaseTransport = baseTransport

    if debug {
        builder.Logger = slog.New(logutil.NewLogrusHandler(...))
    }

    cred := remo.Credential()
    if !cred.IsEmpty() {
        // Wrap inline credential as a single-entry store
        builder.CredentialStore = credentials.NewMemoryStore()
        // or set props.Credential directly (see 2.2)
    } else {
        store, err := credential.NewStore(remo.Configs...)
        if err != nil {
            return nil, err
        }
        builder.CredentialStore = store
    }
    return builder, nil
}

func (remo *Remote) NewRepository(reference string, common Common, logger logrus.FieldLogger) (*remote.Repository, error) {
    props, err := properties.NewReference(reference)
    if err != nil {
        return nil, err
    }

    // Override from CLI flags
    registry := props.Reference.Registry
    props.Transport.PlainHTTP = remo.isPlainHTTP(registry)
    props.Transport.Insecure = remo.Insecure
    if remo.CACertFilePath != "" {
        props.Transport.CACerts = []string{remo.CACertFilePath}
    }
    if remo.CertFilePath != "" && remo.KeyFilePath != "" {
        props.Transport.ClientCerts = []properties.ClientCert{
            {CertFile: remo.CertFilePath, KeyFile: remo.KeyFilePath},
        }
    }
    cred := remo.Credential()
    if !cred.IsEmpty() {
        props.Credential = cred
    }

    builder, err := remo.newClientBuilder(common.Debug)
    if err != nil {
        return nil, err
    }

    repo, err := remote.NewRepositoryWithProperties(props, builder)
    if err != nil {
        return nil, err
    }
    repo.SkipReferrersGC = true
    if remo.ReferrersAPI != ReferrersStateUnknown {
        repo.SetReferrersCapability(remo.ReferrersAPI == ReferrersStateSupported)
    }
    return repo, nil
}
```

`NewRepositoryWithProperties` also calls `NewWarningLogger` automatically, eliminating the need to set `HandleWarning` manually.

#### 2.2 `config.LoadConfigs()` for Full Ecosystem Config Stack

**File:** `cmd/oras/internal/option/remote.go`

**Current:** Only reads Docker `config.json` via `credential.NewStore()`. Ignores containers `auth.json`, per-registry TLS from `certs.d`, and registry mirrors from `registries.conf`.

**New:** Use `config.LoadConfigs()` as the baseline, then override with CLI flags. This follows Scenario 2 (CLI Tool with Flag Overrides) from SCENARIOS.md exactly.

```go
import "github.com/oras-project/oras-go/v3/registry/remote/config"

func (remo *Remote) registryProperties(reference string) (*properties.Registry, credentials.Store, error) {
    // Load full ecosystem config: Docker config.json, containers auth.json,
    // registries.conf (mirrors, blocked registries), certs.d (per-registry TLS).
    // Files that do not exist are silently skipped.
    opts := config.LoadConfigsOptions{}
    if len(remo.Configs) > 0 {
        opts.DockerConfigPath = remo.Configs[0] // --registry-config flag
    }
    configs, err := config.LoadConfigsWithOptions(opts)
    if err != nil {
        return nil, nil, err
    }

    // Config-driven properties (resolves mirrors, alias rewrites, certs.d TLS).
    props, err := configs.RegistryProperties(reference)
    if err != nil {
        return nil, nil, err
    }

    // CLI flags override config-file values (highest priority per Scenario 2).
    if remo.Insecure {
        props.Transport.Insecure = true
    }
    if remo.CACertFilePath != "" {
        props.Transport.CACerts = []string{remo.CACertFilePath}
    }
    if remo.CertFilePath != "" && remo.KeyFilePath != "" {
        props.Transport.ClientCerts = []properties.ClientCert{
            {CertFile: remo.CertFilePath, KeyFile: remo.KeyFilePath},
        }
    }
    plainHTTP, enforced := remo.plainHTTP()
    if enforced {
        props.Transport.PlainHTTP = plainHTTP
    } else if isLocalhost(props.Reference.Registry) {
        props.Transport.PlainHTTP = true
    }
    cred := remo.Credential()
    if !cred.IsEmpty() {
        props.Credential = cred // explicit credential wins over store
    }

    store, err := configs.CredentialStore(credentials.StoreOptions{})
    if err != nil {
        return nil, nil, err
    }
    return props, store, nil
}
```

**Capabilities unlocked at zero additional CLI surface:**
- **Registry mirrors**: When `registries.conf` exists (standard on RHEL/Fedora/CentOS/WSL), mirrors are tried automatically on every read operation before falling back to the primary registry.
- **Podman/containers auth.json**: Credentials saved by Podman (`podman login`) are read alongside Docker credentials.
- **certs.d TLS**: Per-registry CA certificates in `~/.config/containers/certs.d/` or `/etc/containers/certs.d/` are loaded without any CLI flags.

**Future flags to add (separate PRs):**
- `--registries-config` — override path to `registries.conf`
- `--policy-file` — override path to `policy.json` (prerequisite for policy enforcement, Part 3)

#### 2.3 `oras.CopyError` Structured Error Handling

**Files:** `cmd/oras/root/cp.go`, `cmd/oras/root/push.go`, `cmd/oras/root/pull.go`, `cmd/oras/root/attach.go`

v3 wraps copy failures in `*oras.CopyError` which identifies whether the failure originated at the source or destination and which descriptor failed. (SCENARIOS.md §15.)

```go
// In runCopy(), runPush(), runPull(), runAttach() after oras.Copy() calls:
_, err = oras.Copy(ctx, src, srcRef, dst, dstRef, opts)
if err != nil {
    var copyErr *oras.CopyError
    if errors.As(err, &copyErr) {
        switch copyErr.Origin {
        case oras.CopyErrorOriginSource:
            return fmt.Errorf("failed to read %s from source: %w",
                copyErr.Descriptor.Digest, copyErr.Err)
        case oras.CopyErrorOriginDestination:
            return fmt.Errorf("failed to write %s to destination: %w",
                copyErr.Descriptor.Digest, copyErr.Err)
        }
    }
    return err
}
```

This improves the user experience for `oras cp` failures, which often happen due to auth or network issues on one side of a registry-to-registry copy, making the error message clearly actionable.

#### 2.4 `remote.NewCredentialFunc()` Instead of `store.Get` Directly

**File:** `cmd/oras/internal/option/remote.go:289`

Minor hardening: `remote.NewCredentialFunc(store)` gracefully handles a nil store (returns empty credentials), whereas `store.Get` would panic. With `ClientBuilder` adoption this is handled internally, but as a standalone fix:

```go
// Before
client.CredentialFunc = remo.store.Get

// After
client.CredentialFunc = remote.NewCredentialFunc(remo.store)
```

### Part 3 — Future Work (Separate PRs)

The following new v3 capabilities require new CLI flags and more substantial user-facing design. They are not part of the oras-go v3 migration PR but are enabled by it.

| Capability | oras-go Package | SCENARIOS.md | Notes |
|------------|----------------|--------------|-------|
| Policy enforcement (`--policy-file`) | `registry/remote/policy` | §3 | Requires UX design for `insecureAcceptAnything` / `reject` / `signedBy` rules |
| OpenPGP signature verification | `registry/remote/signature` | §10 | Depends on `registries.d` config support and lookaside store URLs |
| `objects` package for push/pack | `objects` | §5 | Higher-level alternative to `PackManifest`; consider for `oras push` simplification |
| `ParseReferenceList` for multi-tag | `registry` | §4 | Convenience for `oras tag` with comma-separated tags |

### Implementation Order

| # | Task | Primary Files | Net LOC | Unblocked By |
|---|------|--------------|---------|-------------|
| 1 | `oci.New()` → `oci.NewStorage()` in cache | `option/cache.go` | −2 | — |
| 2 | Delete `internal/cache/`; use `oras-go/content/cache` | `internal/cache/`, `option/cache.go` | −100 | 1 |
| 3 | `logutil.NewLogrusHandler` slog bridge | new `internal/logutil/bridge.go` | +15 | — |
| 4 | `remote.NewWarningLogger()` via bridge | `option/remote.go`, `option/binary_target.go` | −25 | 3 |
| 5 | `ClientBuilder` in `NewRepository()`/`NewRegistry()` | `option/remote.go` | −40+30 | 3,4 |
| 6 | `config.LoadConfigs()` full config stack | `option/remote.go` | +30 | 5 |
| 7 | `remote.NewLoggingTransport()` via `builder.Logger` | `option/remote.go`, `internal/trace/` | −160 | 5 |
| 8 | `oras.CopyError` structured errors | `root/cp.go`, `root/push.go`, etc. | +20 | — |
| 9 | `remote.NewCredentialFunc()` | `option/remote.go` | −2 | — (or covered by 5) |

Steps 1–2 are self-contained bug fixes. Steps 3–7 form a coherent refactor of the remote client construction path. Steps 8–9 are independent improvements.

## Impact to Users and Ecosystem

- **Podman/containers-tools users** get automatic credential discovery without `--registry-config` pointing at `auth.json`.
- **Enterprise/air-gapped users** get automatic mirror fallback from `registries.conf` with no CLI changes.
- **CI operators** sharing a cache directory (`ORAS_CACHE`) across parallel jobs no longer risk cache corruption from `oci.New()` concurrent writes.
- **All users** get clearer `oras cp` error messages distinguishing source failures from destination failures.
- **Codebase**: net reduction of ~300 lines of duplicated code once all steps are complete.
