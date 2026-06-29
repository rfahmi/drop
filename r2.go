package main

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type R2Client struct {
	Client     *s3.Client
	BucketName string
}

func NewR2Client(ctx context.Context) (*R2Client, error) {
	accountID := os.Getenv("R2_ACCOUNT_ID")
	accessKeyID := os.Getenv("R2_ACCESS_KEY_ID")
	secretAccessKey := os.Getenv("R2_SECRET_ACCESS_KEY")
	bucketName := os.Getenv("R2_BUCKET_NAME")

	if accountID == "" || accessKeyID == "" || secretAccessKey == "" || bucketName == "" {
		return nil, fmt.Errorf("R2 environment variables are missing (R2_ACCOUNT_ID, R2_ACCESS_KEY_ID, R2_SECRET_ACCESS_KEY, R2_BUCKET_NAME)")
	}

	r2Resolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
		return aws.Endpoint{
			URL: fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID),
		}, nil
	})

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithEndpointResolverWithOptions(r2Resolver),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(accessKeyID, secretAccessKey, "")),
		config.WithRegion("auto"),
	)
	if err != nil {
		return nil, err
	}

	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	return &R2Client{
		Client:     client,
		BucketName: bucketName,
	}, nil
}

type FileInfo struct {
	Name         string    `json:"name"`
	Size         int64     `json:"size"`
	LastModified time.Time `json:"last_modified"`
}

func (r *R2Client) ListFiles(ctx context.Context) ([]FileInfo, error) {
	var files []FileInfo
	paginator := s3.NewListObjectsV2Paginator(r.Client, &s3.ListObjectsV2Input{
		Bucket: &r.BucketName,
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, obj := range page.Contents {
			files = append(files, FileInfo{
				Name:         *obj.Key,
				Size:         *obj.Size,
				LastModified: *obj.LastModified,
			})
		}
	}
	return files, nil
}

func (r *R2Client) UploadFile(ctx context.Context, key string, body io.Reader, size *int64, contentType string) error {
	input := &s3.PutObjectInput{
		Bucket:        &r.BucketName,
		Key:           &key,
		Body:          body,
		ContentLength: size,
	}
	if contentType != "" {
		input.ContentType = &contentType
	}
	_, err := r.Client.PutObject(ctx, input)
	return err
}

func (r *R2Client) DownloadFile(ctx context.Context, key string) (io.ReadCloser, *int64, error) {
	resp, err := r.Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &r.BucketName,
		Key:    &key,
	})
	if err != nil {
		return nil, nil, err
	}
	return resp.Body, resp.ContentLength, nil
}

func (r *R2Client) DeleteFile(ctx context.Context, key string) error {
	_, err := r.Client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &r.BucketName,
		Key:    &key,
	})
	return err
}

func (r *R2Client) RenameFile(ctx context.Context, srcKey, destKey string) error {
	source := url.PathEscape(r.BucketName + "/" + srcKey)
	_, err := r.Client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     &r.BucketName,
		Key:        &destKey,
		CopySource: aws.String(source),
	})
	if err != nil {
		return err
	}
	return r.DeleteFile(ctx, srcKey)
}
