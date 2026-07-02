#!/usr/bin/env bash
# Renders a fresh server.json for the MCP Registry from a set of already-
# built release zip assets. Used by the release pipeline (and callable by
# hand) so server.json's version/identifiers/hashes always match exactly
# what's attached to the GitHub release being published.
#
# Usage: render-server-json.sh <version> <tag> <asset-dir>
#   version    e.g. 0.1.0  (no leading v)
#   tag        e.g. v0.1.0 (matches the GitHub release tag)
#   asset-dir  directory containing the
#              s3-mcp-connector-plugin-<tag>-<goos>-<goarch>.zip files
#
# Prints the rendered server.json to stdout.
set -euo pipefail

if [ "$#" -ne 3 ]; then
  echo "usage: $0 <version> <tag> <asset-dir>" >&2
  exit 1
fi

VERSION="$1"
TAG="$2"
ASSET_DIR="$3"
REPO="FerhatDundar/s3-mcp-connector"
PLATFORMS=(darwin-arm64 darwin-amd64 linux-amd64 linux-arm64 windows-amd64 windows-arm64)

command -v jq >/dev/null || { echo "jq is required" >&2; exit 1; }
command -v shasum >/dev/null && HASH_CMD=(shasum -a 256) || HASH_CMD=(sha256sum)

env_vars='[
  {"name":"AWS_ACCESS_KEY_ID","description":"AWS access key ID","isRequired":true,"isSecret":true,"format":"string"},
  {"name":"AWS_SECRET_ACCESS_KEY","description":"AWS secret access key","isRequired":true,"isSecret":true,"format":"string"},
  {"name":"AWS_REGION","description":"AWS region (default us-east-1)","format":"string"},
  {"name":"S3_ENDPOINT_URL","description":"Custom S3 endpoint, e.g. http://localhost:4566 for LocalStack. Leave unset for real AWS.","format":"string"},
  {"name":"S3_FORCE_PATH_STYLE","description":"Set to true for LocalStack/MinIO path-style addressing.","format":"string"}
]'

packages="[]"
for plat in "${PLATFORMS[@]}"; do
  file="$ASSET_DIR/s3-mcp-connector-plugin-${TAG}-${plat}.zip"
  if [ ! -f "$file" ]; then
    echo "missing release asset: $file" >&2
    exit 1
  fi
  sha=$("${HASH_CMD[@]}" "$file" | awk '{print $1}')
  url="https://github.com/${REPO}/releases/download/${TAG}/s3-mcp-connector-plugin-${TAG}-${plat}.zip"

  pkg=$(jq -n \
    --arg url "$url" \
    --arg version "$VERSION" \
    --arg sha "$sha" \
    --argjson envVars "$env_vars" \
    '{
      registryType: "mcpb",
      identifier: $url,
      version: $version,
      fileSha256: $sha,
      transport: { type: "stdio" },
      environmentVariables: $envVars
    }')
  packages=$(jq --argjson p "$pkg" '. + [$p]' <<<"$packages")
done

jq -n \
  --arg version "$VERSION" \
  --argjson packages "$packages" \
  '{
    "$schema": "https://static.modelcontextprotocol.io/schemas/2025-12-11/server.schema.json",
    name: "io.github.FerhatDundar/s3-mcp-connector",
    description: "MCP server for Amazon S3 and S3-compatible endpoints (LocalStack, MinIO). Single Go binary.",
    repository: { url: "https://github.com/FerhatDundar/s3-mcp-connector", source: "github" },
    version: $version,
    websiteUrl: "https://github.com/FerhatDundar/s3-mcp-connector#readme",
    packages: $packages
  }'
