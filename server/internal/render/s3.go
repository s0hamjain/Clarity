package render

import (
	"context"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// s3Config reads the upload target from the environment. Concat's signature
// (FRD §14.1) has no room for it, unlike quality — this package's only other
// bit of config comes from the environment, matching FRD §22's server/.env.
func s3Config() (endpoint, bucket, accessKey, secretKey string) {
	return envOr("S3_ENDPOINT", "http://localhost:9000"),
		envOr("RENDER_BUCKET", "clarity-renders"),
		envOr("S3_ACCESS_KEY", "minioadmin"),
		envOr("S3_SECRET_KEY", "minioadmin")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// uploadToS3 puts localPath at key in the configured bucket and returns a
// direct, public URL (FRD §14.6). Path-style addressing — the bucket in the
// URL path rather than as a subdomain — because MinIO needs it; real S3
// accepts both.
func uploadToS3(ctx context.Context, localPath, key string) (string, error) {
	endpoint, bucket, accessKey, secretKey := s3Config()

	f, err := os.Open(localPath)
	if err != nil {
		return "", fmt.Errorf("opening %s: %w", localPath, err)
	}
	defer f.Close()

	client := s3.New(s3.Options{
		Region:       "us-east-1", // required by the SDK; MinIO ignores it
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
		Credentials:  credentials.NewStaticCredentialsProvider(accessKey, secretKey, ""),
	})

	_, err = client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        f,
		ContentType: aws.String("video/mp4"),
	})
	if err != nil {
		return "", fmt.Errorf("uploading to s3://%s/%s: %w", bucket, key, err)
	}

	return fmt.Sprintf("%s/%s/%s", endpoint, bucket, key), nil
}
