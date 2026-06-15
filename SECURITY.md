# Reporting security issues

As Forkana is a fork of Gitea, please see Gitea's [security policy](https://github.com/go-gitea/gitea/security).

If you want to also inform us, please [join our Slack](https://join.slack.com/t/okturtles/shared_invite/zt-10jmpfgxj-tXQ1MKW7t8qqdyY6fB7uyQ) and DM `@greg`. You can also securely contact Greg [via Keybase](https://keybase.io/greg).

## Dependency age gating

Forkana delays dependency updates for ecosystems with native age controls:

- Node.js/frontend dependencies use pnpm `minimumReleaseAge: 20160` in `pnpm-workspace.yaml`.
- Python/tooling dependencies use uv `exclude-newer = "14 days"` in `pyproject.toml`.
- Go: No native age gating exists as of Go 1.25. While `go.sum` and `GOSUMDB` ensure package integrity and prevent tampering, they do not provide supply-chain age controls. Age-gating for Go is currently tracked as a separate follow-up item.
