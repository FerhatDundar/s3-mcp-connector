# go-server

MCP server source for the S3 connector. Single-file (`main.go`), built with
[mark3labs/mcp-go](https://github.com/mark3labs/mcp-go) and the
[AWS SDK for Go v2](https://github.com/aws/aws-sdk-go-v2).

## Build

```bash
go mod tidy                          # fetches deps, writes go.sum
go build -o s3-connector-server .
cp s3-connector-server ../plugin/servers/go/
```

Requires Go 1.23+.

## Manual test against LocalStack

Start LocalStack from the parent folder (`docker compose up -d`), then run
the server directly and drive it over stdio with a JSON-RPC test script —
or use any MCP client. Minimal example with plain `curl`-style JSON-RPC
piped to the binary:

```bash
export AWS_ACCESS_KEY_ID=test
export AWS_SECRET_ACCESS_KEY=test
export AWS_REGION=us-east-1
export S3_ENDPOINT_URL=http://localhost:4566
export S3_FORCE_PATH_STYLE=true

./s3-connector-server
```

Then send `initialize`, `notifications/initialized`, and `tools/call`
JSON-RPC messages over stdin (one per line), reading one JSON reply per
line from stdout. This is exactly how the connector was validated during
development: created a bucket, wrote a text object and a binary
(base64) object, listed/head'd/read them back, deleted both objects and
the bucket, and confirmed a 404 error path for a nonexistent bucket — all
against a real LocalStack S3 service, not mocks.

## Code layout

- **S3 client setup** — reads `AWS_REGION`, `S3_ENDPOINT_URL`,
  `S3_FORCE_PATH_STYLE` and builds an `*s3.Client` with `BaseEndpoint` /
  `UsePathStyle` overridden when set (this is what makes LocalStack work).
- **Core operations** — one function per S3 action, each returning a
  `map[string]any` for uniform JSON/Markdown rendering.
- **Markdown rendering** — one `*MD` function per operation.
- **MCP wiring** — `main()` registers all 8 tools on an `mcp-go` server and
  serves over stdio.
