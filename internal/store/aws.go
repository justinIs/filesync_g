package store

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

type S3FileStoreConfig struct {
	Bucket  string
	Region  string
	Prefix  string
	Profile string
}

type S3FileStore struct {
	s3Config S3FileStoreConfig
	client   *s3.Client
	tmClient *transfermanager.Client
}

var _ FileStore = (*S3FileStore)(nil)

func NewS3FileStore(ctx context.Context, s3Config S3FileStoreConfig) (*S3FileStore, error) {
	if s3Config.Bucket == "" {
		return nil, errors.New("empty bucket")
	}
	s3Config.Prefix = strings.TrimSuffix(s3Config.Prefix, "/")

	s := &S3FileStore{
		s3Config: s3Config,
	}

	cfg, err := config.LoadDefaultConfig(
		ctx,
		config.WithSharedConfigProfile(s3Config.Profile),
		config.WithRegion(s3Config.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("store: LoadDefaultConfig: %w", err)
	}

	s.client = s3.NewFromConfig(cfg)
	s.tmClient = transfermanager.New(s.client)

	return s, nil
}

func (s *S3FileStore) addPrefix(key string) *string {
	if s.s3Config.Prefix == "" {
		return &key
	}
	str := fmt.Sprintf("%s/%s", s.s3Config.Prefix, key)
	return &str
}

func (s *S3FileStore) Put(ctx context.Context, key string, r io.Reader) error {
	_, err := s.tmClient.UploadObject(ctx, &transfermanager.UploadObjectInput{
		Bucket: &s.s3Config.Bucket,
		Key:    s.addPrefix(key),
		Body:   r,
	})
	if err != nil {
		return fmt.Errorf("store: put %s: %w", key, err)
	}
	return nil
}

func (s *S3FileStore) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: &s.s3Config.Bucket,
		Key:    s.addPrefix(key),
	})
	if err != nil {
		return fmt.Errorf("store: delete %s: %w", key, err)
	}
	return nil
}
