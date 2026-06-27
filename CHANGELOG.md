# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and the project uses [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-06-27

### Added

- Namespace-scoped HTTP execution of the contextq CLI.
- Labeled bearer-key creation, listing, authentication, and revocation.
- Local targets and an HTTP-backed contextq command client.
- Idempotent systemd and Caddy bootstrap over SSH.
- Linux amd64 and arm64 remote deployment bundles.
- OpenSSH connection reuse for multi-step operations.
- Health and end-to-end doctor checks.
- Bounded request concurrency, duration, body size, and command output.
- Embedded server, commit, toolchain, and bundled-contextq version reporting.
- Reproducible multi-platform release archives with checksums, licenses, an SPDX
  SBOM, and GitHub build-provenance attestations.
- CI, race detection, vulnerability scanning, CodeQL, and dependency updates.
- Installation, configuration, feature, API, operations, and security documentation.

[Unreleased]: https://github.com/norlinga/contextq-server/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/norlinga/contextq-server/releases/tag/v0.1.0
