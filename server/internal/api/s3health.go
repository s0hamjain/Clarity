package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/s0hamjain/Clarity/server/internal/config"
)

// bucketChecker answers one question for /healthz: does the render bucket
// exist? A reachability probe is not enough — MinIO answers on its port long
// before `mc mb` has been run, and a job that renders for ninety seconds and
// then fails to upload is the worst possible way to discover the bucket is
// missing.
//
// This is an unauthenticated HEAD on the bucket rather than a signed
// HeadBucket, and that is deliberate: it checks something HeadBucket would not.
// The desktop app streams video_url straight from the object store with no
// credentials, so what has to be true is that the bucket exists AND is
// public-read (FRD §14.6). An unauthenticated HEAD answers exactly that —
// verified against MinIO, which returns 200 for a bucket with `mc anonymous
// set download` and 403 for one that is missing or private. It also keeps the
// AWS SDK out of the coordinator; P2's render/s3.go needs it for PutObject,
// and one S3 client configured in one place beats two.
type bucketChecker struct {
	url    string
	client *http.Client
}

func newBucketChecker(cfg *config.Config) *bucketChecker {
	// Path-style addressing: MinIO serves a bucket as a path, not a subdomain.
	return &bucketChecker{
		url:    strings.TrimRight(cfg.S3Endpoint, "/") + "/" + cfg.RenderBucket,
		client: &http.Client{},
	}
}

func (b *bucketChecker) check(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.url, nil)
	if err != nil {
		return err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("object store unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// 403 is the interesting one: MinIO answers AccessDenied both for a
		// bucket that does not exist and for one that is not public-read.
		// Either way the desktop app could not play a video from it, so both
		// are unhealthy. `mc anonymous set download <bucket>` is the fix.
		return fmt.Errorf("bucket %s is missing or not public-read (HTTP %d)", b.url, resp.StatusCode)
	}
	return nil
}
