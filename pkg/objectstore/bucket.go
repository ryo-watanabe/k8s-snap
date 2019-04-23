package objectstore

import (
	"os"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"

	"k8s.io/klog"
)

type Bucket struct {
	awsAccessKey string
	awsSecretKey string
	awsEndpoint string
	awsRegion string
	BucketName string
}

func NewBucket(awsAccessKey, awsSecretKey, awsEndpoint, awsRegion, bucketName string) *Bucket {
	return &Bucket{
		awsAccessKey: awsAccessKey,
		awsSecretKey: awsSecretKey,
		awsEndpoint: awsEndpoint,
		awsRegion: awsRegion,
		BucketName: bucketName,
	}
}

func (b *Bucket) setSession() (*session.Session, error) {
	creds := credentials.NewStaticCredentials(b.awsAccessKey, b.awsSecretKey, "")
	sess, err := session.NewSession(&aws.Config{
		Credentials: creds,
		Region: aws.String(b.awsRegion),
		Endpoint: &b.awsEndpoint,
	})
	if err != nil {
		return nil, err
	}
	return sess, nil
}

func (b *Bucket) ChkBucket() (bool, error) {
	// set session
	sess, err := b.setSession()
	if err != nil {
		return false, err
	}

	// list buckets
	svc := s3.New(sess)
	result, err := svc.ListBuckets(nil)
	if err != nil {
		return false, err
	}
	klog.Info("Buckets:")
	found := false
	for _, bu := range result.Buckets {
	    klog.Infof("-- %s created on %s\n", aws.StringValue(bu.Name), aws.TimeValue(bu.CreationDate))
	    if aws.StringValue(bu.Name) == b.BucketName {
		    found = true
	    }
	}

	return found, nil
}

func (b *Bucket) Upload(file *os.File, filename string) error {
	// set session
	sess, err := b.setSession()
	if err != nil {
		return err
	}

	uploader := s3manager.NewUploader(sess)
	_, err = uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(b.BucketName),
		Key: aws.String(filename),
		Body: file,
	})
	if err != nil {
		return fmt.Errorf("Error uploading %s to bucket %s : %s", filename, b.BucketName, err.Error())
	}

	return nil
}

func (b *Bucket) Download(file *os.File, filename string) error {
	// set session
	sess, err := b.setSession()
	if err != nil {
		return err
	}

	downloader := s3manager.NewDownloader(sess)
	_, err = downloader.Download(file,
		&s3.GetObjectInput{
			Bucket: aws.String(b.BucketName),
			Key:    aws.String(filename),
		})
	if err != nil {
		return fmt.Errorf("Error downloading %s from bucket %s : %s", filename, b.BucketName, err.Error())
	}

	return nil
}

func (b *Bucket) Delete(filename string) error {
	// set session
	sess, err := b.setSession()
	if err != nil {
		return err
	}

	svc := s3.New(sess)
	_, err = svc.DeleteObject(&s3.DeleteObjectInput{
		Bucket: aws.String(b.BucketName),
		Key: aws.String(filename),
	})
	if err != nil {
		return fmt.Errorf("Error deleting %s from bucket %s : %s", filename, b.BucketName, err.Error())
	}

	err = svc.WaitUntilObjectNotExists(&s3.HeadObjectInput{
	    Bucket: aws.String(b.BucketName),
	    Key:    aws.String(filename),
	})
	return err
}
