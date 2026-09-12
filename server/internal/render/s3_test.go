package render

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// s3Available skips the test (not fails it) when MinIO isn't reachable at the
// default local endpoint — these tests need `docker run` MinIO (SETUP.md §7)
// and the clarity-renders bucket to exist.
func s3Available(t *testing.T) bool {
	t.Helper()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(envOr("S3_ENDPOINT", "http://localhost:9000") + "/minio/health/live")
	if err != nil {
		t.Skip("MinIO is not reachable at S3_ENDPOINT; skipping S3 tests")
		return false
	}
	resp.Body.Close()
	return true
}

func TestUploadToS3(t *testing.T) {
	if !s3Available(t) {
		return
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "test.mp4")
	content := []byte("not a real mp4, just bytes for the upload test")
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	key := "renders/test-upload.mp4"
	url, err := uploadToS3(ctx, path, key)
	if err != nil {
		t.Fatalf("uploadToS3: %v", err)
	}
	t.Logf("uploaded to %s", url)

	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s: expected 200, got %d — is the bucket public-read?", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading response body: %v", err)
	}
	if string(body) != string(content) {
		t.Fatalf("uploaded content mismatch: got %q, want %q", body, content)
	}
}
