# Setup guide — S3 connector

Two paths: test locally against **LocalStack** (no AWS account, no cost,
recommended first), then optionally point the same connector at **real
AWS S3**. Steps 1–2 are on your machine; step 3 is AWS's website if you go
that route.

---

## 1. Get the server binary

**Option A — download a release (no Go needed):** grab
`s3-mcp-connector-plugin-<version>-<os>-<arch>.zip` from the
[latest release](https://github.com/FerhatDundar/s3-mcp-connector/releases/latest)
and unzip it — the `plugin/` folder inside is ready to install, skip to
step 4.

**Option B — build from source:** you need Go 1.25+ installed
(`go version` to check; get it from <https://go.dev/dl/> if not) and
Docker (for LocalStack testing).

```bash
cd subprojects/s3-mcp-connector/go-server
go mod tidy                          # fetches deps, writes go.sum
go build -o s3-connector-server .
cp s3-connector-server ../plugin/servers/go/
```

## 2. Test against LocalStack

From the subproject root:

```bash
cd subprojects/s3-mcp-connector
docker compose up -d                 # starts LocalStack S3 on :4566
```

Check it's up: `curl http://localhost:4566/_localstack/health` should show
`"s3": "available"`.

Configure the connector's `.mcp.json` for LocalStack (`plugin/.mcp.json`):

```json
{
  "mcpServers": {
    "s3": {
      "command": "${CLAUDE_PLUGIN_ROOT}/servers/go/s3-connector-server",
      "env": {
        "AWS_REGION": "us-east-1",
        "AWS_ACCESS_KEY_ID": "test",
        "AWS_SECRET_ACCESS_KEY": "test",
        "S3_ENDPOINT_URL": "http://localhost:4566",
        "S3_FORCE_PATH_STYLE": "true"
      }
    }
  }
}
```

LocalStack accepts any non-empty access key / secret — `test`/`test` is
the convention. `S3_FORCE_PATH_STYLE=true` is required for LocalStack to
resolve bucket URLs correctly.

When you're done testing: `docker compose down` (add `-v` or delete the
`localstack-data/` folder to wipe LocalStack's state between runs).

> **Note on the `latest` image tag:** this connector's `docker-compose.yml`
> pins `localstack/localstack:3` (Community edition, no license needed).
> Some newer LocalStack image tags default to a Pro build that refuses to
> start without a `LOCALSTACK_AUTH_TOKEN`. If you see `License activation
> failed!` in `docker compose logs`, switch back to a `:3.x`-style
> community tag.

## 3. (Optional) Point at real AWS S3

1. In the AWS Console, create or reuse an IAM user/role with S3 permissions
   scoped to the buckets you want the agent to touch (avoid `s3:*` on `*`
   in production — least privilege).
2. Generate an access key for that identity: **IAM → Users → your user →
   Security credentials → Create access key**.
3. Update `plugin/.mcp.json`:

```json
{
  "mcpServers": {
    "s3": {
      "command": "${CLAUDE_PLUGIN_ROOT}/servers/go/s3-connector-server",
      "env": {
        "AWS_REGION": "us-east-1",
        "AWS_ACCESS_KEY_ID": "your-real-access-key",
        "AWS_SECRET_ACCESS_KEY": "your-real-secret-key",
        "S3_ENDPOINT_URL": "",
        "S3_FORCE_PATH_STYLE": "false"
      }
    }
  }
}
```

Leave `S3_ENDPOINT_URL` empty and `S3_FORCE_PATH_STYLE` false/omitted for
real AWS — the SDK resolves the correct regional endpoint automatically.

Keep this file out of any shared/committed location — it now holds real
credentials. (The subproject's `.gitignore` already blocks `.env`/`*.token`
files; the `.mcp.json` inside the *installed* plugin lives in Cowork's
plugin directory, not the repo.)

## 4. Install the plugin and verify

1. In the Claude desktop app: **Settings → Capabilities** → add a plugin
   from a local folder, and point it at:
   `subprojects/s3-mcp-connector/plugin/`
   (Same way `sqlite-connector` and `discord-connector` were installed. A
   plugin can't be registered from inside a chat session — it has to be
   added here.)
2. Restart / reload so the new MCP server is picked up.
3. Back in a chat, verify:
   - Ask: **"Run s3_list_buckets"** → should return your bucket(s), or an
     empty list if none exist yet (not an error).
   - Ask: **"Create an S3 bucket called test-bucket"** → confirm with
     `s3_list_buckets`.
   - Ask: **"Write 'hello' to key test.txt in test-bucket"** then
     **"Read test.txt from test-bucket"** → should round-trip the text.

Done. From here you can tell the agent things like *"list everything under
`logs/2026/` in the reports bucket"* or *"back up this file to S3."*

---

## Troubleshooting

| Symptom | Likely cause / fix |
|---------|--------------------|
| `no such host` / `connection refused` | LocalStack isn't running (`docker compose up -d`), or `S3_ENDPOINT_URL` is wrong. |
| `License activation failed!` in `docker compose logs` | You're on a Pro LocalStack image tag. Pin `localstack/localstack:3` (see note in step 2). |
| `403 Forbidden` | Wrong/missing AWS credentials, or the IAM identity lacks permission on that bucket. |
| `NoSuchBucket` / `404 Not Found` | Bucket name typo, wrong region, or (LocalStack) state was wiped by a prior `docker compose down -v`. |
| `BucketAlreadyExists` on real AWS | S3 bucket names are globally unique across *all* AWS accounts — pick a more specific name. |
| Object reads back as base64 instead of text | Content wasn't valid UTF-8, or `base64=true` was passed explicitly — this is correct behavior for binary data. |
| Tools don't appear at all | Binary not built/copied (step 1), or plugin not installed/reloaded (step 4). |
