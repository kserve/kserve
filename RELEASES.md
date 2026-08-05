# KServe Release Policy

This document describes the release policy for the KServe project, including release cadence,
versioning, support windows, and what users can expect from each release.

For the detailed release process used by release managers, see [release/RELEASE_PROCESS.md](release/RELEASE_PROCESS.md).

## Release Cadence

KServe targets a new minor release every **8 weeks**. The actual timing may vary based on
feature readiness, critical bug fixes, or community needs.

Each release cycle follows this timeline:

| Week  | Milestone              |
|-------|------------------------|
| 1-5   | Development            |
| 5     | Feature freeze, RC0    |
| 6     | RC1+ (if needed)       |
| 7-8   | Final GA release       |

## Versioning Scheme

KServe follows [Semantic Versioning](https://semver.org/) with the format `0.MINOR.PATCH`:

- **Minor release** (`0.Y.0`): Contains new features, improvements, and bug fixes.
- **Patch release** (`0.Y.Z`): Contains only security fixes for supported versions.
  No bug fix backports except breaking change; users should upgrade to the latest minor release.
- **Release candidate** (`0.Y.0-rcN`): Pre-release for testing before a GA release.

> **Note:** KServe is pre-1.0 and follows a `0.Y.Z` scheme. Minor version bumps (`Y`) may
> include breaking changes. Breaking changes are documented in release notes.

## Release Types

| Type | Tag Format | Purpose |
|------|-----------|---------|
| Release Candidate | `v0.Y.0-rcN` | Pre-release for community testing. Not for production use. |
| GA (Stable) | `v0.Y.0` | Production-ready release with full support. |
| Patch | `v0.Y.Z` | Security fixes only(breaking change) for supported versions. |

## Support Policy

KServe maintains support for the **latest two minor releases**:

- **Latest minor release (N)**: Receives security patches as patch releases.
- **Previous minor release (N-1)**: Receives critical security patches only.
- **Older releases (N-2 and below)**: Not supported. Users should upgrade.

Bug fixes are **not** backported to older releases. With an 8-week release cadence,
users are encouraged to upgrade to the latest minor release to receive all fixes.

### Supported Versions

<!-- Update this table with each new release -->
| Version | Release Date | Status |
|---------|-------------|--------|
| v0.19   | Jun 12, 2026 | **Active** - latest |
| v0.18   | Apr 29, 2026 | Security patches only |
| v0.17   | Mar 13, 2026 | End of life |

### Kubernetes Compatibility

Each KServe release is tested against specific Kubernetes versions. Refer to the
[compatibility matrix](https://kserve.github.io/website/latest/admin/kubernetes_deployment/)
in the documentation for details.

## Deprecation Policy

- Deprecated features are announced at least **one release cycle** before removal.
- Deprecations are listed in the release notes of the version where they are deprecated.
- Deprecated APIs continue to function for at least one additional release after the
  deprecation announcement.

## Release Artifacts

Each release includes the following artifacts:

| Artifact | Location |
|----------|----------|
| Container images | [GitHub Container Registry](https://github.com/orgs/kserve/packages) |
| Python packages | [PyPI (kserve)](https://pypi.org/project/kserve/), [PyPI (kserve-storage)](https://pypi.org/project/kserve-storage/) |
| Helm charts | [GHCR Helm Registry](https://github.com/orgs/kserve/packages) |
| Install manifests | Included in each [GitHub Release](https://github.com/kserve/kserve/releases) |
| Release notes | Auto-generated on each [GitHub Release](https://github.com/kserve/kserve/releases) |

## Release Management

Releases are performed by **release managers**, selected from the project's
[OWNERS](OWNERS) file (approvers and above). A release manager is designated for
each release cycle and is responsible for:

- Coordinating the release timeline and feature freeze
- Running the release process ([release/RELEASE_PROCESS.md](release/RELEASE_PROCESS.md))
- Publishing release candidates and the final release
- Communicating release status to the community

For project governance and roles, see the
[KServe Community](https://github.com/kserve/community) repository.

## Release Announcements

Releases are announced through:

- [GitHub Releases](https://github.com/kserve/kserve/releases) (primary)
- [KServe Blog](https://kserve.github.io/website/latest/blog/) (for major releases)
- [KServe Slack](https://cloud-native.slack.com/archives/C06AH2C3K8B)

## Security

For reporting security vulnerabilities and the security patch process,
see [SECURITY.md](SECURITY.md).
