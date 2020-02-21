package objectstore

type FakeBucket struct {
	Name       string
	AccessKey  string
	SecretKey  string
	Endpoint   string
	Region     string
	BucketName string
}

func NewFakeBucket(name, accessKey, secretKey, endpoint, region, bucketName string) *FakeBucket {
	return &FakeBucket{
		Name:       name,
		AccessKey:  accessKey,
		SecretKey:  secretKey,
		Endpoint:   endpoint,
		Region:     region,
		BucketName: bucketName,
	}
}