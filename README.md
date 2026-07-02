<div align="center">

# 🪣 s3-mcp-connector

**Talk to Amazon S3 — or LocalStack, MinIO, any S3-compatible store — from an MCP-speaking agent.**

[![CI](https://github.com/FerhatDundar/s3-mcp-connector/actions/workflows/ci.yml/badge.svg)](https://github.com/FerhatDundar/s3-mcp-connector/actions/workflows/ci.yml)
[![CodeQL](https://github.com/FerhatDundar/s3-mcp-connector/actions/workflows/codeql.yml/badge.svg)](https://github.com/FerhatDundar/s3-mcp-connector/actions/workflows/codeql.yml)
[![Latest release](https://img.shields.io/github/v/release/FerhatDundar/s3-mcp-connector?color=blueviolet&label=release)](https://github.com/FerhatDundar/s3-mcp-connector/releases/latest)
[![Go Reference](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/license-MIT-yellow.svg)](LICENSE)
[![Tested with LocalStack](https://img.shields.io/badge/tested%20with-LocalStack-6A2FEE?logo=amazonaws&logoColor=white)](https://localstack.cloud/)
[![MCP](https://img.shields.io/badge/protocol-MCP-orange)](https://modelcontextprotocol.io/)
[![Conventional Commits](https://img.shields.io/badge/commits-conventional-ff69b4)](https://www.conventionalcommits.org/)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](CONTRIBUTING.md)

</div>

---

A single static Go binary that speaks the [Model Context Protocol](https://modelcontextprotocol.io/)
and exposes 8 tools for working with S3: list buckets, list/read/write/delete
objects, create/delete buckets. Point it at real AWS or at a local
LocalStack/MinIO instance with one environment variable — same binary,
same tools, zero code changes.

No Python, no `uv`, no runtime dependency to install — just a binary and
an `.mcp.json`.

## ✨ Why this exists

> An agent that can only *talk about* your S3 buckets isn't that useful.
> This gives it hands: it can look inside a bucket, read a config file out
> of it, drop a report back in, or clean up a stale prefix — safely, with
> guardrails on size and destructive actions built in.

## 🧰 Tools

| Tool | What it does | Write? |
|---|---|:---:|
| `s3_list_buckets` | List buckets visible to the credentials | |
| `s3_list_objects` | List objects in a bucket, optional prefix, paginated | |
| `s3_head_object` | Object metadata (size, type, ETag) without downloading | |
| `s3_get_object` | Read an object's content — text or base64, size-capped | |
| `s3_put_object` | Write a small text/base64 object (≤ 5 MB) | ✍️ |
| `s3_delete_object` | Delete one object | 🗑️ destructive |
| `s3_create_bucket` | Create a bucket | ✍️ |
| `s3_delete_bucket` | Delete an *empty* bucket | 🗑️ destructive |

Every tool accepts an optional `response_format`: `markdown` (default,
pretty tables for a chat UI) or `json` (for programmatic use).

`s3_get_object` auto-detects text vs. binary content and caps output at
**200,000 bytes** by default (raise via `max_bytes`, hard cap
**5,000,000**) — it's built for reading configs, logs, and small data
files, not bulk transfer. Reach for the AWS CLI or SDK directly for large
objects.

## 🚀 Quickstart

**Fastest path:** grab a prebuilt bundle from the [latest release](https://github.com/FerhatDundar/s3-mcp-connector/releases/latest) —
download `s3-mcp-connector-plugin-<version>-<os>-<arch>.zip`, unzip it,
and point Cowork/Claude at the `plugin/` folder inside (see step 4 of
[SETUP.md](SETUP.md)). No Go toolchain required.

**From source:**

```bash
# 1. Build
cd go-server
go mod tidy
go build -o s3-connector-server .
cp s3-connector-server ../plugin/servers/go/

# 2. Spin up LocalStack to test against (no AWS account needed)
cd ..
docker compose up -d

# 3. Point the connector at it
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export S3_ENDPOINT_URL=http://localhost:4566
export S3_FORCE_PATH_STYLE=true

./go-server/s3-connector-server   # serves MCP over stdio
```

Or `make build && make localstack-up` — see the [Makefile](Makefile) for
every shortcut (`test`, `vet`, `fmt`, `lint`, `tidy`, `localstack-down`).

Full walkthrough — including wiring this up as a Claude/Cowork plugin and
switching from LocalStack to real AWS — is in **[SETUP.md](SETUP.md)**.

## 🔐 Configuration

Everything is environment variables, passed through by the plugin's
`.mcp.json`:

| Variable | Purpose | Default |
|---|---|---|
| `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` | AWS credentials. Any non-empty values work against LocalStack. | — |
| `AWS_REGION` | Region to use. | `us-east-1` |
| `S3_ENDPOINT_URL` | Custom endpoint. Leave unset for real AWS. | *(unset)* |
| `S3_FORCE_PATH_STYLE` | `true` for LocalStack/MinIO (path-style addressing). | `false` |

## 🧪 Quality bar

This isn't a toy script — it's got the same checks you'd expect from a
production Go service:

- ✅ **Unit tests** for every input-validation path (`go test ./...`)
- ✅ **`go vet`** + **`gofmt`** clean
- ✅ **[golangci-lint](https://golangci-lint.run/)** (govet, staticcheck, errcheck, gosec, and more)
- ✅ **[govulncheck](https://go.dev/blog/vuln)** — no known vulnerabilities in the dependency graph
- ✅ **[CodeQL](https://codeql.github.com/)** static security analysis on every push
- ✅ **End-to-end verified against real LocalStack** — every tool
  (create/delete bucket, put/get/head/list/delete object, and the 404
  error path) was exercised against a live S3-compatible service, not
  mocks
- ✅ **[Dependabot](.github/dependabot.yml)** keeps Go modules and Actions current

All of it runs in [CI](.github/workflows/ci.yml) on every push and PR.

## 🏷️ Releases & versioning

Versions follow [semver](https://semver.org/) and are cut automatically by
[release-please](https://github.com/googleapis/release-please) from
[Conventional Commits](https://www.conventionalcommits.org/) on `main`:

- `fix: ...` → patch (`v0.1.0` → `v0.1.1`)
- `feat: ...` → minor (`v0.1.1` → `v0.2.0`)
- `feat!: ...` / `BREAKING CHANGE:` footer → major (`v0.2.0` → `v1.0.0`)

Every merged PR updates a standing **"chore(main): release vX.Y.Z"** PR
with an auto-generated [CHANGELOG.md](CHANGELOG.md). Merging that PR tags
the release, publishes it on GitHub, and a follow-up job builds and
attaches zipped, ready-to-install plugin bundles for
linux/darwin × amd64/arm64. See [.github/workflows/release-please.yml](.github/workflows/release-please.yml).

## 📁 Layout

```
s3-mcp-connector/
├── README.md                  ← you are here
├── SETUP.md                   ← step-by-step setup guide (LocalStack + real AWS)
├── CONTRIBUTING.md             ← how to contribute
├── CODE_OF_CONDUCT.md
├── SECURITY.md                 ← vulnerability reporting
├── CODEOWNERS
├── LICENSE                     ← MIT
├── Makefile                    ← build / test / lint / localstack shortcuts
├── docker-compose.yml          ← LocalStack, for local testing
├── .golangci.yml                ← lint rules
├── release-please-config.json  ← semver/changelog automation config
├── .release-please-manifest.json
├── .github/
│   ├── workflows/
│   │   ├── ci.yml                     ← build, vet, test, lint, govulncheck
│   │   ├── codeql.yml                 ← security scanning
│   │   ├── pr-title.yml               ← Conventional Commits PR title check
│   │   ├── release-please.yml         ← version PRs, tagging, GitHub releases
│   │   └── rebuild-release-assets.yml ← manual re-attach fallback
│   ├── ISSUE_TEMPLATE/
│   ├── PULL_REQUEST_TEMPLATE.md
│   └── dependabot.yml
├── go-server/                  ← the MCP server source
│   ├── main.go
│   ├── main_test.go
│   ├── go.mod / go.sum
│   └── README.md
└── plugin/                     ← installable Cowork/Claude plugin
    ├── .claude-plugin/plugin.json
    ├── .mcp.json                ← holds credentials locally — never commit real ones
    └── servers/go/              ← compiled binary goes here
```

## 🤝 Contributing

PRs and issues are very welcome — see **[CONTRIBUTING.md](CONTRIBUTING.md)**
for the full guide (setup, coding conventions, how to add a new tool) and
the **[Code of Conduct](CODE_OF_CONDUCT.md)**.

`main` is protected: every change, including the maintainer's, lands via
pull request with CI green. PR titles must follow
[Conventional Commits](https://www.conventionalcommits.org/) — that's what
drives the automatic versioning above.

Found a security issue? Please follow **[SECURITY.md](SECURITY.md)**
instead of opening a public issue.

## 📄 License

[MIT](LICENSE) © Ferhat Dundar
