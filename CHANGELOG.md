# Changelog

## 0.1.0 (2026-07-03)

Initial release.

### ✨ Features

- MCP server exposing 8 S3 tools: `s3_list_buckets`, `s3_list_objects`,
  `s3_head_object`, `s3_get_object`, `s3_put_object`, `s3_delete_object`,
  `s3_create_bucket`, `s3_delete_bucket`
- Works against real AWS S3 or any S3-compatible endpoint (LocalStack,
  MinIO) via `S3_ENDPOINT_URL` / `S3_FORCE_PATH_STYLE`
- `markdown`/`json` response formats on every tool
- `--version` flag; ldflag-injected build version

### 🧪 Quality

- Unit tests, `go vet`, `gofmt`, `golangci-lint`, `govulncheck`, and
  CodeQL all wired into CI
- End-to-end verified against a live LocalStack S3 service during
  development

### 🤝 Project infrastructure

- Contribution guide, Code of Conduct, security policy, issue/PR templates
- Branch protection: all changes (including the maintainer's) land via
  reviewed, CI-green pull requests
- Automated semver releases via [release-please](https://github.com/googleapis/release-please),
  starting from this baseline
- Cross-platform (linux/darwin × amd64/arm64) zipped plugin bundles
  attached to every release

---

*From here on, this file is maintained automatically by release-please
based on [Conventional Commits](https://www.conventionalcommits.org/).*
