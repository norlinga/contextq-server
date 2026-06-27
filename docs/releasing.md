# Releasing

Official releases are built by GitHub Actions from immutable `v*` tags. The release
workflow builds all supported controller and server architectures, publishes SHA-256
checksums and an SPDX SBOM, and creates a GitHub build-provenance attestation.

## Before the first release

Configure the GitHub repository to:

- require the CI and CodeQL checks on `main`
- enable private vulnerability reporting
- enable secret scanning and push protection where the repository plan permits it
- prevent tag updates or deletion for `v*` tags

No repository secrets are required by the release workflow. It uses the scoped
`GITHUB_TOKEN` and GitHub's OIDC token for attestations.

## Prepare a release

1. Confirm `.go-version` names a supported, security-patched Go release.
2. Confirm `CONTEXTQ_VERSION` names the contextq release to bundle. When changing
   it, also reconcile `THIRD_PARTY_NOTICES.md`, `licenses/`, and the package list in
   `scripts/release.sh` with that contextq tag.
3. Move the relevant entries in `CHANGELOG.md` from `Unreleased` to the new version.
4. Run the same local checks used by CI:

   ```sh
   gofmt -w cmd internal
   go mod tidy
   make check
   go test -race ./...
   ```

5. Build and inspect release assets on Linux using the version in `.go-version`:

   ```sh
   scripts/release.sh v0.1.0
   sha256sum -c dist/release/SHA256SUMS
   ```

   Canonical releases use the Go version declared in `.go-version`; `go.mod` records
   the minimum compatible language version. Archive ownership,
   timestamps, ordering, and gzip headers are normalized from the release commit so
   the same commit and toolchain produce stable archives.

6. Commit the release changes and ensure CI passes on `main`.

## Publish

Create and push an annotated tag whose name matches the version:

```sh
git tag -a v0.1.0 -m "contextq-server v0.1.0"
git push origin main v0.1.0
```

The release workflow validates that the tag resolves to the checked-out commit,
runs tests and vet, builds the assets, attests them, and creates the GitHub release.
Do not move or reuse a published release tag.

## Verify a published release

```sh
gh release download v0.1.0 --repo norlinga/contextq-server --dir release
cd release
sha256sum -c SHA256SUMS
gh attestation verify ./*.tar.gz --repo norlinga/contextq-server
```

Extract a controller archive and run `contextq-server version` to confirm its
version, commit, build date, Go version, and bundled contextq version.
