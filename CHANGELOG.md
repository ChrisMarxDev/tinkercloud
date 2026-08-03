# Changelog

All notable changes to Tinkercloud will be documented in this file.

The project follows [Semantic Versioning](https://semver.org/) once public
versions are released.

## [Unreleased]

## [0.1.6] - 2026-08-03

### Fixed

- Clear completed failed-update recovery snapshots after the prior healthy
  server has been restored and restarted, while retaining degraded recovery
  state when restore, restart, or cleanup fails.

### Changed

- Make the public beta install and first-deployment documentation directly
  copyable without version placeholders or prerequisite manifest editing.

## [0.1.5] - 2026-08-03

### Added

- Initial pre-release Tinkercloud gateway, deployer CLI, browser SDK, operational
  tooling, security contracts, and documentation.
- Open-source community, governance, security reporting, and repository
  maintenance files.
- One protected release workflow publishing the signed GitHub prerelease,
  `@tinkercloud/sdk` under `beta`, and `@tinkercloud/cli` under `next`.

[0.1.6]: https://github.com/ChrisMarxDev/tinkercloud/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/ChrisMarxDev/tinkercloud/releases/tag/v0.1.5
