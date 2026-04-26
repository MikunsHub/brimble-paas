package s3

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Client struct {
	client    *awss3.Client
	presigner *awss3.PresignClient
	bucket    string
}

func NewClient(ctx context.Context, region, endpoint, accessKeyID, secretAccessKey, bucket string) (*Client, error) {
	cfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(region),
		awsconfig.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	client := awss3.NewFromConfig(cfg, func(o *awss3.Options) {
		o.UsePathStyle = true
		if endpoint != "" {
			o.BaseEndpoint = &endpoint
		}
	})

	return &Client{
		client:    client,
		presigner: awss3.NewPresignClient(client),
		bucket:    bucket,
	}, nil
}

func (c *Client) EnsureBucket(ctx context.Context) error {
	_, err := c.client.HeadBucket(ctx, &awss3.HeadBucketInput{Bucket: &c.bucket})
	if err == nil {
		return nil
	}

	_, err = c.client.CreateBucket(ctx, &awss3.CreateBucketInput{Bucket: &c.bucket})
	if err != nil {
		var apiErr interface{ ErrorCode() string }
		if errors.As(err, &apiErr) {
			code := apiErr.ErrorCode()
			if code == "BucketAlreadyOwnedByYou" || code == "BucketAlreadyExists" {
				return nil
			}
		}
		return fmt.Errorf("create bucket %q: %w", c.bucket, err)
	}

	return nil
}

func (c *Client) CreateDeploymentUploadURL(ctx context.Context, fileName, contentType string, expires time.Duration) (filePath, url string, err error) {
	ext := strings.ToLower(filepath.Ext(fileName))
	if ext != ".zip" {
		return "", "", fmt.Errorf("file must be a .zip archive")
	}
	if contentType == "" {
		contentType = "application/zip"
	}

	filePath = fmt.Sprintf("uploads/%s/source.zip", randomHex(12))
	input := &awss3.PutObjectInput{
		Bucket:      &c.bucket,
		Key:         &filePath,
		ContentType: &contentType,
	}
	req, err := c.presigner.PresignPutObject(ctx, input, func(opts *awss3.PresignOptions) {
		opts.Expires = expires
	})
	if err != nil {
		return "", "", fmt.Errorf("presign put object: %w", err)
	}

	return filePath, req.URL, nil
}

func (c *Client) Upload(ctx context.Context, key string, body io.Reader, contentType string) error {
	_, err := c.client.PutObject(ctx, &awss3.PutObjectInput{
		Bucket:      &c.bucket,
		Key:         &key,
		Body:        body,
		ContentType: &contentType,
	})
	if err != nil {
		return fmt.Errorf("put object %q: %w", key, err)
	}
	return nil
}

func (c *Client) Download(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := c.client.GetObject(ctx, &awss3.GetObjectInput{
		Bucket: &c.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	return out.Body, nil
}

func (c *Client) Exists(ctx context.Context, key string) (bool, error) {
	_, err := c.client.HeadObject(ctx, &awss3.HeadObjectInput{
		Bucket: &c.bucket,
		Key:    &key,
	})
	if err == nil {
		return true, nil
	}

	var notFound *types.NotFound
	if errors.As(err, &notFound) {
		return false, nil
	}

	var apiErr interface{ ErrorCode() string }
	if errors.As(err, &apiErr) && (apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey") {
		return false, nil
	}

	return false, fmt.Errorf("head object %q: %w", key, err)
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}
