// Command s3-connector-server is an MCP server that lets an agent work
// with Amazon S3 (or an S3-compatible endpoint like LocalStack): list
// buckets, list/inspect/read/write/delete objects, and create/delete
// buckets. Built with the AWS SDK for Go v2 and mark3labs/mcp-go, mirroring
// the layout and conventions of the sibling discord-mcp-connector and
// sqlite-mcp-connector.
//
// Auth / config (all via environment variables, passed through by the
// plugin's .mcp.json):
//
//	AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY / AWS_SESSION_TOKEN — standard
//	    AWS credentials. Any non-empty values work against LocalStack.
//	AWS_REGION            — defaults to "us-east-1" if unset.
//	S3_ENDPOINT_URL        — optional. Point at a LocalStack/MinIO endpoint,
//	    e.g. http://localhost:4566. Leave unset for real AWS S3.
//	S3_FORCE_PATH_STYLE    — optional, "true"/"1" to force path-style
//	    addressing (required by LocalStack and most S3-compatible services).
package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	defaultRegion  = "us-east-1"
	defaultMaxKeys = 1000
	// defaultGetLimit caps how many bytes s3_get_object returns inline by
	// default, to avoid dumping huge objects into a tool response. Callers
	// can raise it up to hardGetLimit via the max_bytes parameter.
	defaultGetLimit = 200_000
	hardGetLimit    = 5_000_000
	// putContentLimit caps how much text s3_put_object will accept in one
	// call — this is a tool for small config/data objects, not bulk upload.
	putContentLimit = 5_000_000
)

// ---------------------------------------------------------------------
// S3 client setup
// ---------------------------------------------------------------------

var s3Client *s3.Client

func newS3Client(ctx context.Context) (*s3.Client, error) {
	region := strings.TrimSpace(os.Getenv("AWS_REGION"))
	if region == "" {
		region = defaultRegion
	}

	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("could not load AWS config: %w", err)
	}

	endpoint := strings.TrimSpace(os.Getenv("S3_ENDPOINT_URL"))
	forcePathStyle := isTruthy(os.Getenv("S3_FORCE_PATH_STYLE"))

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		if endpoint != "" {
			o.BaseEndpoint = aws.String(endpoint)
		}
		if forcePathStyle {
			o.UsePathStyle = true
		}
	})
	return client, nil
}

// clampInt32 safely narrows an int (e.g. from an MCP tool argument) to
// int32, clamping instead of overflowing on out-of-range values.
func clampInt32(v int) int32 {
	switch {
	case v > math.MaxInt32:
		return math.MaxInt32
	case v < math.MinInt32:
		return math.MinInt32
	default:
		return int32(v)
	}
}

func isTruthy(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

// friendlyS3Error adds actionable hints to common S3 failures instead of
// surfacing the raw SDK error string.
func friendlyS3Error(action string, err error) error {
	if err == nil {
		return nil
	}
	var re *smithyhttp.ResponseError
	msg := err.Error()
	if errors.As(err, &re) && re.Response != nil {
		switch re.Response.StatusCode {
		case 301, 400:
			if strings.Contains(msg, "region") {
				return fmt.Errorf("%s failed: wrong region for this bucket, or S3_ENDPOINT_URL/AWS_REGION misconfigured. (%s)", action, msg)
			}
		case 403:
			return fmt.Errorf("%s failed: 403 Forbidden — check AWS credentials and bucket/object permissions. (%s)", action, msg)
		case 404:
			return fmt.Errorf("%s failed: 404 Not Found — bucket or key doesn't exist. (%s)", action, msg)
		}
	}
	if strings.Contains(msg, "NoSuchBucket") {
		return fmt.Errorf("%s failed: no such bucket. Use s3_list_buckets to see what's available. (%s)", action, msg)
	}
	if strings.Contains(msg, "NoSuchKey") {
		return fmt.Errorf("%s failed: no object with that key in the bucket. Use s3_list_objects to check. (%s)", action, msg)
	}
	if strings.Contains(msg, "BucketNotEmpty") {
		return fmt.Errorf("%s failed: bucket is not empty. Delete all objects first (s3_list_objects + s3_delete_object), then retry. (%s)", action, msg)
	}
	if strings.Contains(msg, "BucketAlreadyOwnedByYou") {
		return fmt.Errorf("%s failed: bucket already exists and is already yours — nothing to do. (%s)", action, msg)
	}
	if strings.Contains(msg, "BucketAlreadyExists") {
		return fmt.Errorf("%s failed: bucket name is already taken globally (S3 bucket names are unique across all AWS accounts). Pick another name. (%s)", action, msg)
	}
	if strings.Contains(msg, "connect") || strings.Contains(msg, "no such host") || strings.Contains(msg, "connection refused") {
		return fmt.Errorf("%s failed: could not reach the S3 endpoint. If testing against LocalStack, confirm it's running (docker compose up) and S3_ENDPOINT_URL is set correctly. (%s)", action, msg)
	}
	return fmt.Errorf("%s failed: %w", action, err)
}

// ---------------------------------------------------------------------
// Core operations — each returns a map[string]any for uniform rendering
// ---------------------------------------------------------------------

func listBuckets(ctx context.Context) (map[string]any, error) {
	out, err := s3Client.ListBuckets(ctx, &s3.ListBucketsInput{})
	if err != nil {
		return nil, friendlyS3Error("list buckets", err)
	}
	buckets := make([]map[string]any, 0, len(out.Buckets))
	for _, b := range out.Buckets {
		entry := map[string]any{"name": aws.ToString(b.Name)}
		if b.CreationDate != nil {
			entry["created"] = b.CreationDate.UTC().Format(time.RFC3339)
		}
		buckets = append(buckets, entry)
	}
	return map[string]any{"count": len(buckets), "buckets": buckets}, nil
}

func listObjects(ctx context.Context, bucket, prefix, continuationToken string, maxKeys int32) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("bucket is required. Use s3_list_buckets to find bucket names")
	}
	if maxKeys < 1 {
		maxKeys = defaultMaxKeys
	}
	input := &s3.ListObjectsV2Input{
		Bucket:  aws.String(bucket),
		MaxKeys: aws.Int32(maxKeys),
	}
	if strings.TrimSpace(prefix) != "" {
		input.Prefix = aws.String(prefix)
	}
	if strings.TrimSpace(continuationToken) != "" {
		input.ContinuationToken = aws.String(continuationToken)
	}
	out, err := s3Client.ListObjectsV2(ctx, input)
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("list objects in bucket %q", bucket), err)
	}
	objects := make([]map[string]any, 0, len(out.Contents))
	for _, o := range out.Contents {
		entry := map[string]any{
			"key":  aws.ToString(o.Key),
			"size": aws.ToInt64(o.Size),
		}
		if o.LastModified != nil {
			entry["last_modified"] = o.LastModified.UTC().Format(time.RFC3339)
		}
		if o.ETag != nil {
			entry["etag"] = strings.Trim(aws.ToString(o.ETag), `"`)
		}
		objects = append(objects, entry)
	}
	result := map[string]any{
		"bucket":       bucket,
		"count":        len(objects),
		"objects":      objects,
		"is_truncated": aws.ToBool(out.IsTruncated),
	}
	if out.NextContinuationToken != nil {
		result["next_continuation_token"] = aws.ToString(out.NextContinuationToken)
	}
	return result, nil
}

func headObject(ctx context.Context, bucket, key string) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("bucket and key are both required")
	}
	out, err := s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("head object %q in bucket %q", key, bucket), err)
	}
	result := map[string]any{
		"bucket":        bucket,
		"key":           key,
		"size":          aws.ToInt64(out.ContentLength),
		"content_type":  aws.ToString(out.ContentType),
		"etag":          strings.Trim(aws.ToString(out.ETag), `"`),
		"storage_class": string(out.StorageClass),
	}
	if out.LastModified != nil {
		result["last_modified"] = out.LastModified.UTC().Format(time.RFC3339)
	}
	return result, nil
}

func getObject(ctx context.Context, bucket, key string, maxBytes int64, forceBase64 bool) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("bucket and key are both required")
	}
	if maxBytes <= 0 {
		maxBytes = defaultGetLimit
	}
	if maxBytes > hardGetLimit {
		maxBytes = hardGetLimit
	}

	out, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("get object %q from bucket %q", key, bucket), err)
	}
	defer func() { _ = out.Body.Close() }()

	totalSize := aws.ToInt64(out.ContentLength)

	limited := io.LimitReader(out.Body, maxBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("reading object body for %q: %w", key, err)
	}

	truncated := int64(len(data)) > maxBytes
	if truncated {
		data = data[:maxBytes]
	}

	result := map[string]any{
		"bucket":        bucket,
		"key":           key,
		"content_type":  aws.ToString(out.ContentType),
		"total_size":    totalSize,
		"returned_size": len(data),
		"truncated":     truncated,
	}
	if truncated {
		result["truncated_hint"] = "Object is larger than the byte limit returned. Raise max_bytes (hard cap 5,000,000) to fetch more, or fetch a narrower range externally."
	}

	if forceBase64 || !utf8.Valid(data) {
		result["encoding"] = "base64"
		result["content"] = base64.StdEncoding.EncodeToString(data)
	} else {
		result["encoding"] = "text"
		result["content"] = string(data)
	}
	return result, nil
}

func putObject(ctx context.Context, bucket, key, content, contentType string, base64Encoded bool) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("bucket and key are both required")
	}
	if content == "" {
		return nil, fmt.Errorf("content is required and cannot be empty (use a real S3 client for large/binary uploads — this tool is for small text/config objects)")
	}
	if len(content) > putContentLimit {
		return nil, fmt.Errorf("content is %d bytes; this tool's limit is %d bytes for a single call", len(content), putContentLimit)
	}

	var body []byte
	if base64Encoded {
		decoded, err := base64.StdEncoding.DecodeString(content)
		if err != nil {
			return nil, fmt.Errorf("content is not valid base64: %w", err)
		}
		body = decoded
	} else {
		body = []byte(content)
	}

	input := &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
		Body:   strings.NewReader(string(body)),
	}
	if strings.TrimSpace(contentType) != "" {
		input.ContentType = aws.String(contentType)
	}

	out, err := s3Client.PutObject(ctx, input)
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("put object %q into bucket %q", key, bucket), err)
	}
	return map[string]any{
		"bucket": bucket,
		"key":    key,
		"size":   len(body),
		"etag":   strings.Trim(aws.ToString(out.ETag), `"`),
	}, nil
}

func deleteObject(ctx context.Context, bucket, key string) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" || strings.TrimSpace(key) == "" {
		return nil, fmt.Errorf("bucket and key are both required")
	}
	_, err := s3Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("delete object %q from bucket %q", key, bucket), err)
	}
	return map[string]any{"bucket": bucket, "key": key, "deleted": true}, nil
}

func createBucket(ctx context.Context, bucket, region string) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	input := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	if strings.TrimSpace(region) != "" && region != "us-east-1" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}
	_, err := s3Client.CreateBucket(ctx, input)
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("create bucket %q", bucket), err)
	}
	return map[string]any{"bucket": bucket, "created": true}, nil
}

func deleteBucket(ctx context.Context, bucket string) (map[string]any, error) {
	if strings.TrimSpace(bucket) == "" {
		return nil, fmt.Errorf("bucket is required")
	}
	_, err := s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		return nil, friendlyS3Error(fmt.Sprintf("delete bucket %q", bucket), err)
	}
	return map[string]any{"bucket": bucket, "deleted": true}, nil
}

// ---------------------------------------------------------------------
// Markdown rendering
// ---------------------------------------------------------------------

func listBucketsMD(d map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# S3 buckets\n\nFound %v bucket(s).\n\n", d["count"])
	buckets, _ := d["buckets"].([]map[string]any)
	if len(buckets) == 0 {
		b.WriteString("_No buckets visible with these credentials._\n")
		return b.String()
	}
	b.WriteString("| Name | Created |\n|------|---------|\n")
	for _, bk := range buckets {
		fmt.Fprintf(&b, "| %v | %v |\n", bk["name"], bk["created"])
	}
	return b.String()
}

func listObjectsMD(d map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Objects in `%v`\n\n%v object(s)", d["bucket"], d["count"])
	if t, _ := d["is_truncated"].(bool); t {
		b.WriteString(" (more available — pass `continuation_token`)")
	}
	b.WriteString(".\n\n")
	objects, _ := d["objects"].([]map[string]any)
	if len(objects) == 0 {
		b.WriteString("_No objects found._\n")
		return b.String()
	}
	b.WriteString("| Key | Size | Last modified |\n|-----|------|---------------|\n")
	for _, o := range objects {
		fmt.Fprintf(&b, "| %v | %v | %v |\n", o["key"], o["size"], o["last_modified"])
	}
	if tok, _ := d["next_continuation_token"].(string); tok != "" {
		fmt.Fprintf(&b, "\n_Next page: `continuation_token=%s`._\n", tok)
	}
	return b.String()
}

func headObjectMD(d map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Object metadata: `%v`\n\n", d["key"])
	fmt.Fprintf(&b, "- **Bucket**: %v\n", d["bucket"])
	fmt.Fprintf(&b, "- **Size**: %v bytes\n", d["size"])
	fmt.Fprintf(&b, "- **Content type**: %v\n", d["content_type"])
	fmt.Fprintf(&b, "- **Last modified**: %v\n", d["last_modified"])
	fmt.Fprintf(&b, "- **ETag**: %v\n", d["etag"])
	fmt.Fprintf(&b, "- **Storage class**: %v\n", d["storage_class"])
	return b.String()
}

func getObjectMD(d map[string]any) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Object: `%v` (bucket `%v`)\n\n", d["key"], d["bucket"])
	fmt.Fprintf(&b, "- **Content type**: %v\n", d["content_type"])
	fmt.Fprintf(&b, "- **Total size**: %v bytes, returned %v bytes\n", d["total_size"], d["returned_size"])
	fmt.Fprintf(&b, "- **Encoding**: %v\n", d["encoding"])
	if hint, ok := d["truncated_hint"].(string); ok {
		fmt.Fprintf(&b, "- **Note**: %s\n", hint)
	}
	b.WriteString("\n---\n\n")
	fmt.Fprintf(&b, "%v\n", d["content"])
	return b.String()
}

func putObjectMD(d map[string]any) string {
	var b strings.Builder
	b.WriteString("# Object written\n\n")
	fmt.Fprintf(&b, "- **Bucket**: %v\n", d["bucket"])
	fmt.Fprintf(&b, "- **Key**: `%v`\n", d["key"])
	fmt.Fprintf(&b, "- **Size**: %v bytes\n", d["size"])
	fmt.Fprintf(&b, "- **ETag**: %v\n", d["etag"])
	return b.String()
}

func deleteObjectMD(d map[string]any) string {
	return fmt.Sprintf("Deleted `%v` from bucket `%v`.\n", d["key"], d["bucket"])
}

func createBucketMD(d map[string]any) string {
	return fmt.Sprintf("Created bucket `%v`.\n", d["bucket"])
}

func deleteBucketMD(d map[string]any) string {
	return fmt.Sprintf("Deleted bucket `%v`.\n", d["bucket"])
}

// ---------------------------------------------------------------------
// MCP wiring
// ---------------------------------------------------------------------

func getFormat(req mcp.CallToolRequest) string {
	f := req.GetString("response_format", "markdown")
	if f != "json" {
		f = "markdown"
	}
	return f
}

func resultOrError(data map[string]any, err error, format string, mdFn func(map[string]any) string) *mcp.CallToolResult {
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Error: %v", err))
	}
	if format == "json" {
		return mcp.NewToolResultText(renderJSON(data))
	}
	return mcp.NewToolResultText(mdFn(data))
}

func renderJSON(v any) string {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error": %q}`, err.Error())
	}
	return string(b)
}

func main() {
	ctx := context.Background()
	client, err := newS3Client(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "s3-connector-server: %v\n", err)
		os.Exit(1)
	}
	s3Client = client

	s := server.NewMCPServer("s3_connector", "0.1.0", server.WithToolCapabilities(false))

	s.AddTool(mcp.NewTool("s3_list_buckets",
		mcp.WithDescription("List S3 buckets visible to the configured credentials. Call this first to confirm AWS credentials and endpoint (real AWS or LocalStack) are working."),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		data, err := listBuckets(ctx)
		return resultOrError(data, err, getFormat(req), listBucketsMD), nil
	})

	s.AddTool(mcp.NewTool("s3_list_objects",
		mcp.WithDescription("List objects in an S3 bucket, optionally filtered by key prefix. Paginated via continuation_token."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name. Get it from s3_list_buckets.")),
		mcp.WithString("prefix", mcp.Description("Only list keys starting with this prefix, e.g. 'logs/2026/'.")),
		mcp.WithNumber("max_keys", mcp.DefaultNumber(1000), mcp.Min(1), mcp.Max(1000), mcp.Description("Max objects to return in this page (max 1000).")),
		mcp.WithString("continuation_token", mcp.Description("Token from a previous call's next_continuation_token, to fetch the next page.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		prefix := req.GetString("prefix", "")
		token := req.GetString("continuation_token", "")
		maxKeys := req.GetInt("max_keys", defaultMaxKeys)
		data, err := listObjects(ctx, bucket, prefix, token, clampInt32(maxKeys))
		return resultOrError(data, err, getFormat(req), listObjectsMD), nil
	})

	s.AddTool(mcp.NewTool("s3_head_object",
		mcp.WithDescription("Get metadata for one S3 object (size, content type, last modified, ETag) without downloading its body."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name.")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Object key.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		key, err := req.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := headObject(ctx, bucket, key)
		return resultOrError(data, err, getFormat(req), headObjectMD), nil
	})

	s.AddTool(mcp.NewTool("s3_get_object",
		mcp.WithDescription("Read an S3 object's content. Returns text directly if the object is valid UTF-8 text; otherwise (or if base64=true) returns base64-encoded content. Output is capped by max_bytes (default 200,000, hard cap 5,000,000) to avoid dumping huge objects — use s3_head_object first to check size."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name.")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Object key.")),
		mcp.WithNumber("max_bytes", mcp.DefaultNumber(defaultGetLimit), mcp.Description("Max bytes to return (hard cap 5,000,000).")),
		mcp.WithBoolean("base64", mcp.DefaultBool(false), mcp.Description("Force base64 encoding even for text objects.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(true),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		key, err := req.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		maxBytes := req.GetInt("max_bytes", defaultGetLimit)
		base64Flag := req.GetBool("base64", false)
		data, err := getObject(ctx, bucket, key, int64(maxBytes), base64Flag)
		return resultOrError(data, err, getFormat(req), getObjectMD), nil
	})

	s.AddTool(mcp.NewTool("s3_put_object",
		mcp.WithDescription("Write an object to S3. Intended for small text/config/data objects (limit 5,000,000 bytes per call), not bulk file upload. Set base64=true to write binary content passed as base64 text."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name.")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Object key to write, e.g. 'reports/2026-07-03.json'.")),
		mcp.WithString("content", mcp.Required(), mcp.Description("The object content — plain text, or base64 if base64=true.")),
		mcp.WithString("content_type", mcp.Description("MIME type to set, e.g. 'application/json', 'text/plain'.")),
		mcp.WithBoolean("base64", mcp.DefaultBool(false), mcp.Description("If true, decode 'content' from base64 before writing.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		key, err := req.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		content, err := req.RequireString("content")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		contentType := req.GetString("content_type", "")
		base64Flag := req.GetBool("base64", false)
		data, err := putObject(ctx, bucket, key, content, contentType, base64Flag)
		return resultOrError(data, err, getFormat(req), putObjectMD), nil
	})

	s.AddTool(mcp.NewTool("s3_delete_object",
		mcp.WithDescription("Delete a single object from an S3 bucket. Irreversible unless the bucket has versioning enabled."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name.")),
		mcp.WithString("key", mcp.Required(), mcp.Description("Object key to delete.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		key, err := req.RequireString("key")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := deleteObject(ctx, bucket, key)
		return resultOrError(data, err, getFormat(req), deleteObjectMD), nil
	})

	s.AddTool(mcp.NewTool("s3_create_bucket",
		mcp.WithDescription("Create a new S3 bucket. Bucket names must be globally unique (on real AWS) and follow S3 naming rules (lowercase, 3-63 chars, no underscores)."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name to create.")),
		mcp.WithString("region", mcp.Description("Region to create the bucket in. Defaults to the connector's configured AWS_REGION.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(false),
		mcp.WithIdempotentHintAnnotation(false),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		region := req.GetString("region", "")
		data, err := createBucket(ctx, bucket, region)
		return resultOrError(data, err, getFormat(req), createBucketMD), nil
	})

	s.AddTool(mcp.NewTool("s3_delete_bucket",
		mcp.WithDescription("Delete an S3 bucket. The bucket must be empty first (delete all objects with s3_delete_object, or s3_list_objects + loop). Irreversible."),
		mcp.WithString("bucket", mcp.Required(), mcp.Description("Bucket name to delete.")),
		mcp.WithString("response_format", mcp.Enum("markdown", "json"), mcp.DefaultString("markdown"), mcp.Description("Output format: 'markdown' or 'json'")),
		mcp.WithReadOnlyHintAnnotation(false),
		mcp.WithDestructiveHintAnnotation(true),
		mcp.WithIdempotentHintAnnotation(true),
		mcp.WithOpenWorldHintAnnotation(true),
	), func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		bucket, err := req.RequireString("bucket")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		data, err := deleteBucket(ctx, bucket)
		return resultOrError(data, err, getFormat(req), deleteBucketMD), nil
	})

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "s3-connector-server error: %v\n", err)
		os.Exit(1)
	}
}
