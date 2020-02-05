package cluster

import (
	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
)

type FakeClusterCmd struct {
}

func NewFakeClusterCmd() *FakeClusterCmd {
	return &FakeClusterCmd{}
}

func (c *FakeClusterCmd)Snapshot(snapshot *cbv1alpha1.Snapshot) error {
	return nil
}

func (c *FakeClusterCmd)UploadSnapshot(snapshot *cbv1alpha1.Snapshot, bucket *objectstore.Bucket) error {
	return nil
}

func (c *FakeClusterCmd)Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket *objectstore.Bucket) error {
	return nil
}
