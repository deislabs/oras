# Releasing ORAS CLI

ORAS CLI releases are managed by [`scripts/release.sh`](scripts/release.sh), which automates preparation, tagging, validation, and publishing.

## Prerequisites

Before starting a release:

- Install the GitHub CLI (`gh`) v2.68.0 or later and authenticate it. `validate` uses `gh attestation verify` with `--source-ref`, which older versions do not have.
- Ensure the `GPG_PRIVATE_KEY` and `GPG_PASSPHRASE` GitHub Actions secrets are configured for release signing.
- Configure Git commit and tag signing (GPG or SSH).
- Install `gpg`; `publish` uses it to verify artifact signatures even if you sign with SSH.
- Ensure the ORAS project release public key is available in [`KEYS`](KEYS) for artifact signature verification.
- Before releasing, update the Go version in `go.mod` and the relevant GitHub Actions workflows when the project moves to a new Go version.
- Make sure the branch you are releasing from (`main`, or the maintenance branch for a backport release) is in a releasable state and that fixes for applicable critical and high CVEs have landed on it.

## Release process

Run commands from the repository root. Replace `VERSION` and `COMMIT_SHA` with the intended release version and commit SHA.

### 1. Prepare the release

```sh
scripts/release.sh prep VERSION
```

The script validates the version and release branch, updates `internal/version/version.go`, creates a signed-off (DCO) release-preparation commit, pushes a branch, and opens a pull request.

It also prints a release-vote template for Slack `#oras-maintainers`. The pull request must pass CI and receive the required maintainer super-majority approval before merging.

Releases for the current series and new minor versions are prepared from `main`. Patch releases for older series must use their corresponding `release-X.Y` branch. Set `ORAS_MAIN_RELEASE_VERSION` to override the release series detected for `main` when necessary.

### 2. Tag the release

After the release pull request is approved and merged, tag the merged commit:

```sh
scripts/release.sh tag VERSION COMMIT_SHA
```

The script verifies that the commit records the requested version, creates and pushes a signed `vVERSION` tag, and creates a `release-X.Y` branch for a new minor release.

The tag triggers the `release-github` and `release-ghcr` workflows.

### 3. Validate the release

Run `validate` right after tagging; it waits up to 30 minutes for the tag-triggered workflows to finish:

```sh
scripts/release.sh validate VERSION
```

The script downloads release artifacts, checks their SHA-256 checksums, verifies the build provenance attestations of every archive and the checksums file, and verifies the binary version and available commit information. It fails if any of these has no valid attestation. A tag on a maintenance branch must include the attestation step in `release-github.yml`, or `validate` fails.

Review the release notes and verify that the GitHub Release is associated with the intended `vVERSION` tag and commit, not a GoReleaser placeholder tag such as `untagged-<hash>`.

### 4. Publish the release

After validation succeeds, run:

```sh
scripts/release.sh publish VERSION
```

The script verifies artifact signatures against the ORAS project release key, adds verification information to the release notes (the GPG instructions and the `gh attestation verify` steps), publishes the GitHub Release, and triggers the Snap publishing workflow (with `isStable=false` for pre-releases).

## Dry runs

Use `--dry-run` to preview the `prep` and `tag` phases without changing the repository:

```sh
scripts/release.sh --dry-run prep VERSION
scripts/release.sh --dry-run tag VERSION COMMIT_SHA
```

`validate` and `publish` need a real tag and the `_dist/` directory downloaded by `validate`, so they cannot be dry-run before a release.

## Container image tags

The `release-ghcr` workflow always pushes the `vVERSION` image tag. For stable releases it also moves the rolling `vX.Y`, `vX`, and `latest` tags, but only when `vVERSION` is the highest stable tag in that minor series, major series, or overall, respectively. Pre-releases never move rolling tags, and a backport to an older series moves only the rolling tags it is newest for, so rolling tags never move backwards.

## Post-release tasks

- Open a pull request in [oras-www](https://github.com/oras-project/oras-www) and complete the following checklist, replacing `X.Y` with the release series and `VERSION` with the full release version:
  - For a new minor or preview release, add the version to `versions.json`, `versions-map`, `versioned_docs/`, and `versioned_sidebars/`, following the preview-to-GA steps in `tools/refresh-commands.sh`.
  - In `versions-map`, set the `X.Y` line to `X.Y VERSION`, run `npm run refresh-commands`, and commit the regenerated `versioned_docs/version-X.Y/commands/` files.
  - In `versioned_docs/version-X.Y/installation.mdx`, update every pinned version, including download snippets, direct links, `go install`, the `ghcr.io/oras-project/oras` image, and the `doras` alias, then refresh the sample `oras version` output from the released binary.
  - In `versioned_docs/version-X.Y/how_to_guides/verifying_binaries.mdx`, update the example version and asset file names.
  - In `src/components/ORAS_CLI/index.jsx`, update the homepage installation snippet.
  - Publish a blog post for a minor release; a blog post is optional for a patch release.
  - Link the pull request from the release-vote or announcement thread.
  - The release is not finished until this pull request is open.
- Announce the release in Slack `#oras`.
- Publish applicable security advisories after the release has shipped.

## Verifying release artifacts

Users can verify a downloaded release in two ways.

With GPG, import the public key from [`KEYS`](KEYS) and check the detached signature that is attached to the release:

```sh
gpg --import KEYS
gpg --verify oras_VERSION_linux_amd64.tar.gz.asc oras_VERSION_linux_amd64.tar.gz
```

With the GitHub CLI, check the build provenance attestation:

```sh
gh attestation verify oras_VERSION_linux_amd64.tar.gz --repo oras-project/oras
```

The attestations are Sigstore bundles, so they can also be downloaded with `gh attestation download` and checked with `cosign verify-blob-attestation`.

## Implementation references

- [`scripts/release.sh`](scripts/release.sh)
- [`.goreleaser.yml`](.goreleaser.yml)
- [Release workflows](.github/workflows)
