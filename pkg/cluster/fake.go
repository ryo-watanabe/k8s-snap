package cluster

import (
	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
)

// FakeCmd for fake command interface
type FakeCmd struct {
}

// NewFakeClusterCmd returns fake cluster interface
func NewFakeClusterCmd() *FakeCmd {
	return &FakeCmd{}
}

// Snapshot for fake cluster interface
func (c *FakeCmd) Snapshot(snapshot *cbv1alpha1.Snapshot) error {
	return nil
}

// UploadSnapshot for fake cluster interface
func (c *FakeCmd) UploadSnapshot(snapshot *cbv1alpha1.Snapshot, bucket *objectstore.Bucket) error {
	return nil
}

// Restore for fake cluster interface
func (c *FakeCmd) Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket *objectstore.Bucket) error {
	return nil
}