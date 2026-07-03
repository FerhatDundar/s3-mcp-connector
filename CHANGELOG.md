# Changelog

## [0.2.1](https://github.com/FerhatDundar/s3-mcp-connector/compare/v0.2.0...v0.2.1) (2026-07-03)


### Bug Fixes

* MCP Registry badge always showed the oldest published version ([8946eb0](https://github.com/FerhatDundar/s3-mcp-connector/commit/8946eb0d82ce3c674f757443a79fe34ed40b27d0))
* release asset upload used a doubly-nested go-server/go-server path ([7cdad36](https://github.com/FerhatDundar/s3-mcp-connector/commit/7cdad3688f1cc943bde734cff2dd408c189a4656))
* validate AWS credentials at startup and align MCP server name ([2a7e36c](https://github.com/FerhatDundar/s3-mcp-connector/commit/2a7e36c5ffa3b56bdc0cd42931d188295618d699))

## [0.2.0](https://github.com/FerhatDundar/s3-mcp-connector/compare/v0.1.0...v0.2.0) (2026-07-02)


### Features

* add Windows to the release build matrix ([1f1cac6](https://github.com/FerhatDundar/s3-mcp-connector/commit/1f1cac62ce77a7ae124b28ae2abf6bd35c530e60))
* automate MCP Registry publishing in the release pipeline ([e2f833a](https://github.com/FerhatDundar/s3-mcp-connector/commit/e2f833a0467a727cc20b90dcb97ab319a844072f))


### Bug Fixes

* do not pin publish-mcp-registry checkout to the release tag ([3824c6d](https://github.com/FerhatDundar/s3-mcp-connector/commit/3824c6d7fb029a10c0e4bce0911c4616ff3fcf3b))
* shorten server.json description to satisfy MCP registry 100-char limit ([04fb671](https://github.com/FerhatDundar/s3-mcp-connector/commit/04fb671e245ccf0d174febf2985b998e710fe0d6))

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
