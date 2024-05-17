package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"io"
	"nbox/internal/application"
	"nbox/internal/domain"
	"nbox/internal/domain/models"
)

type storeAdapter struct {
	s3     *s3.Client
	config *application.Config
}

func NewStoreAdapter(s3 *s3.Client, config *application.Config) domain.StoreOperations {
	return &storeAdapter{
		s3:     s3,
		config: config,
	}
}

func (b *storeAdapter) store(ctx context.Context, path string, stage models.Stage) (*s3.PutObjectOutput, error) {
	var out bytes.Buffer
	err := json.Indent(&out, []byte(stage.Template.Value), "", "  ")
	if err != nil {
		return nil, err
	}

	return b.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(path),
		Body:   bytes.NewReader(out.Bytes()),
	})
}

func (b *storeAdapter) BoxExists(ctx context.Context, service string, stage string, template string) (bool, error) {
	path := fmt.Sprintf("%s/%s/%s", service, stage, template)

	_, err := b.s3.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(path),
	})

	return err == nil, err
}

func (b *storeAdapter) RetrieveBox(ctx context.Context, service string, stage string, template string) ([]byte, error) {
	path := fmt.Sprintf("%s/%s/%s", service, stage, template)
	object, err := b.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.config.BucketName),
		Key:    aws.String(path),
	})

	if err != nil {
		return nil, err
	}

	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(object.Body)

	body, err := io.ReadAll(object.Body)

	if err != nil {
		return nil, err
	}

	return body, nil
}

func (b *storeAdapter) UpsertBox(ctx context.Context, box *models.Box) []string {
	var result []string
	for stageName, stage := range box.Stage {
		path := fmt.Sprintf("%s/%s/%s", box.Service, stageName, stage.Template.Name)

		stage.Template.Name = path
		box.Stage[stageName] = stage
		_, err := b.store(ctx, path, stage)

		if err == nil {
			result = append(result, path)
		}
	}
	return result
}
