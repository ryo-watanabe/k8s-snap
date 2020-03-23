package objectstore

import (
	"testing"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/aws/aws-sdk-go/service/s3/s3manager/s3manageriface"
)

// NewMockBucket returns new mock Bucket
func NewMockBucket(name, accessKey, secretKey, endpoint, region, bucketName string, insecure bool) *Bucket {
	return &Bucket{
		Name:       name,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		Endpoint:   endpoint,
		Region:     region,
		BucketName: bucketName,
		insecure:   insecure,
		newS3func:  newMockS3,
		newUploaderfunc:  newMockUploader,
		newDownloaderfunc:  newMockDownloader,
	}
}

// Mock interfaces

type mockS3Client struct {
	s3iface.S3API
}

func (m mockS3Client) ListBuckets(input *s3.ListBucketsInput) (*s3.ListBucketsOutput, error) {
	return &s3.ListBucketsOutput{}, nil
}

func newMockS3(sess *session.Session) s3iface.S3API {
	return mockS3Client{}
}

// Mock Uploader

type mockUploader struct {
	s3manageriface.UploaderAPI
}

func newMockUploader(sess *session.Session) s3manageriface.UploaderAPI {
	return mockUploader{}
}

// Mock Downloader

type mockDownloader struct {
	s3manageriface.DownloaderAPI
}

func newMockDownloader(sess *session.Session) s3manageriface.DownloaderAPI {
	return mockDownloader{}
}

func TestBucket(t *testing.T) {
	b := NewMockBucket("test", "ACCESSKEY", "SECRETKEY", "https://endpoint.net", "region", "k8s-snap", true)

	found, err := b.ChkBucket()
	if err != nil {
		t.Errorf("Error in ChkBucket : %s", err.Error())
	}
	if found {
		t.Error("Bucket found error")
	}
}
