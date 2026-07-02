package main

import (
	"context"
	"strings"
	"testing"
)

func TestIsTruthy(t *testing.T) {
	cases := map[string]bool{
		"true":   true,
		"True":   true,
		"1":      true,
		"yes":    true,
		"on":     true,
		"":       false,
		"false":  false,
		"0":      false,
		"nope":   false,
		"  ":     false,
		" TRUE ": true,
	}
	for in, want := range cases {
		if got := isTruthy(in); got != want {
			t.Errorf("isTruthy(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestFriendlyS3ErrorNil(t *testing.T) {
	if err := friendlyS3Error("do a thing", nil); err != nil {
		t.Errorf("expected nil error to stay nil, got %v", err)
	}
}

func TestFriendlyS3ErrorWrapsMessage(t *testing.T) {
	base := errTest("boom")
	err := friendlyS3Error("list objects in bucket \"x\"", base)
	if err == nil {
		t.Fatal("expected non-nil error")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected wrapped error to retain original message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "list objects") {
		t.Errorf("expected wrapped error to include the action, got: %v", err)
	}
}

// errTest is a minimal error type so tests don't need to construct real
// AWS SDK error values for the plain-message fallback path.
type errTest string

func (e errTest) Error() string { return string(e) }

// The following tests exercise the input-validation guards on each core
// operation. All of them return before touching the (nil) s3Client, so
// they run without any network access or LocalStack instance.

func TestListObjectsRequiresBucket(t *testing.T) {
	if _, err := listObjects(context.Background(), "", "", "", 10); err == nil {
		t.Error("expected error for empty bucket")
	}
}

func TestHeadObjectRequiresBucketAndKey(t *testing.T) {
	if _, err := headObject(context.Background(), "", "key"); err == nil {
		t.Error("expected error for empty bucket")
	}
	if _, err := headObject(context.Background(), "bucket", ""); err == nil {
		t.Error("expected error for empty key")
	}
}

func TestGetObjectRequiresBucketAndKey(t *testing.T) {
	if _, err := getObject(context.Background(), "", "key", 0, false); err == nil {
		t.Error("expected error for empty bucket")
	}
	if _, err := getObject(context.Background(), "bucket", "", 0, false); err == nil {
		t.Error("expected error for empty key")
	}
}

func TestPutObjectValidation(t *testing.T) {
	ctx := context.Background()

	if _, err := putObject(ctx, "", "key", "content", "", false); err == nil {
		t.Error("expected error for empty bucket")
	}
	if _, err := putObject(ctx, "bucket", "", "content", "", false); err == nil {
		t.Error("expected error for empty key")
	}
	if _, err := putObject(ctx, "bucket", "key", "", "", false); err == nil {
		t.Error("expected error for empty content")
	}

	huge := strings.Repeat("a", putContentLimit+1)
	if _, err := putObject(ctx, "bucket", "key", huge, "", false); err == nil {
		t.Error("expected error for content over the size limit")
	}

	if _, err := putObject(ctx, "bucket", "key", "not-base64!!", "", true); err == nil {
		t.Error("expected error for invalid base64 content when base64=true")
	}
}

func TestDeleteObjectRequiresBucketAndKey(t *testing.T) {
	if _, err := deleteObject(context.Background(), "", "key"); err == nil {
		t.Error("expected error for empty bucket")
	}
	if _, err := deleteObject(context.Background(), "bucket", ""); err == nil {
		t.Error("expected error for empty key")
	}
}

func TestCreateBucketRequiresName(t *testing.T) {
	if _, err := createBucket(context.Background(), "", "us-east-1"); err == nil {
		t.Error("expected error for empty bucket name")
	}
}

func TestDeleteBucketRequiresName(t *testing.T) {
	if _, err := deleteBucket(context.Background(), ""); err == nil {
		t.Error("expected error for empty bucket name")
	}
}

func TestRenderJSONRoundTrips(t *testing.T) {
	out := renderJSON(map[string]any{"bucket": "x", "count": 3})
	if !strings.Contains(out, "\"bucket\": \"x\"") {
		t.Errorf("expected rendered JSON to contain bucket field, got: %s", out)
	}
}
