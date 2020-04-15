package main

import (
	//"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	clustersnapshot "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	informers "github.com/ryo-watanabe/k8s-snap/pkg/client/informers/externalversions"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	clientset "github.com/ryo-watanabe/k8s-snap/pkg/client/clientset/versioned"
	"github.com/ryo-watanabe/k8s-snap/pkg/client/clientset/versioned/fake"
	"github.com/ryo-watanabe/k8s-snap/pkg/cluster"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
	snapshotNamespace  = "default"
)

type Case struct {
	snapshots        []*clustersnapshot.Snapshot
	restores         []*clustersnapshot.Restore
	configs          []*clustersnapshot.ObjectstoreConfig
	preferences      []*clustersnapshot.RestorePreference
	secrets          []*corev1.Secret
	updatedSnapshots []*clustersnapshot.Snapshot
	updatedRestores  []*clustersnapshot.Restore
	deleteSnapshots  []*clustersnapshot.Snapshot
	deleteRestores   []*clustersnapshot.Restore
	queueOnly        bool
	handleKey        string
}

func TestSnapshot(t *testing.T) {

	cases := []Case{
		// 0:create snapshot
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Completed"),
				newConfiguredSnapshot("test2", ""),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test2", "InQueue"),
			},
			queueOnly: true,
			handleKey: "test2",
		},
		// 1:RFC3339
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", ""),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "InQueue"),
			},
			queueOnly: true,
			handleKey: "test1",
		},
		// 2:InQueue > InProgress > Completed
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "InQueue"),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "InProgress"),
				newConfiguredSnapshot("test1", "Completed"),
			},
			configs: []*clustersnapshot.ObjectstoreConfig{
				newObjectstoreConfig(),
			},
			secrets: []*corev1.Secret{
				newCloudCredentialSecret(),
			},
			handleKey: "test1",
		},
		// 3:InQueue > InProgress > Failed - secret not found
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "InQueue"),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "InProgress"),
				newConfiguredSnapshot("test1", "Failed"),
			},
			configs: []*clustersnapshot.ObjectstoreConfig{
				newObjectstoreConfig(),
			},
			handleKey: "test1",
		},
		// 4:Add expiration to failed snapshot and delete
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
			},
			deleteSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
			},
			handleKey: "test1",
		},
		// 5:Mark Failed to InProgress snapshot and delete
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "InProgress"),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
				newConfiguredSnapshot("test1", "Failed"),
			},
			handleKey: "test1",
		},
	}

	// Additional test data:

	dur, _ := time.ParseDuration("720h0m0s")

	// 0:Create snapshot
	cases[0].updatedSnapshots[0].Spec.TTL.Duration = dur
	// 1:RFC3339
	cases[1].snapshots[0].Spec.AvailableUntil.Time, _ = time.Parse(time.RFC3339, "2020-07-01T02:03:04Z")
	cases[1].updatedSnapshots[0].Spec.AvailableUntil.Time = time.Date(2020, time.July, 1, 2, 3, 4, 0, time.UTC)
	// 2:InQueue > InProgress > Completed
	// 3:InQueue > InProgress > Failed - secret not found
	cases[3].updatedSnapshots[1].Status.Reason = "secrets \"cloudCredentialSecret\" not found"
	// 4:Add expiration to failed snapshot
	cases[4].snapshots[0].Spec.TTL.Duration = dur
	cases[4].updatedSnapshots[0].Spec.TTL.Duration = dur
	cases[4].updatedSnapshots[0].Status.AvailableUntil = metav1.NewTime(cases[4].updatedSnapshots[0].ObjectMeta.CreationTimestamp.Add(dur))
	cases[4].updatedSnapshots[0].Status.TTL.Duration = dur
	// 5:Mark Failed to InProgress snapshot and delete
	cases[5].updatedSnapshots[0].Status.Reason = "Controller stopped while taking the snapshot"
	cases[5].updatedSnapshots[1].Status.Reason = "Controller stopped while taking the snapshot"

	for _, c := range cases {
		SnapshotTestCase(&c, t)
	}
}

func TestRestore(t *testing.T) {

	cases := []Case{
		// create restore
		Case{
			restores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Completed"),
				newConfiguredRestore("test2", ""),
			},
			updatedRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test2", "InQueue"),
			},
			queueOnly: true,
			handleKey: "test2",
		},
		// 1:InQueue > InProgress > Completed
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("snapshot", "Completed"),
			},
			restores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "InQueue"),
			},
			preferences: []*clustersnapshot.RestorePreference{
				newRestorePreference(),
			},
			updatedRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "InProgress"),
				newConfiguredRestore("test1", "Completed"),
			},
			configs: []*clustersnapshot.ObjectstoreConfig{
				newObjectstoreConfig(),
			},
			secrets: []*corev1.Secret{
				newCloudCredentialSecret(),
			},
			handleKey: "test1",
		},
	}

	// Additional test data:
	// Create restore
	cases[0].updatedRestores[0].Spec.TTL.Duration, _ = time.ParseDuration("168h0m0s")
	// 1:InQueue > InProgress > Completed

	for _, c := range cases {
		RestoreTestCase(&c, t)
	}
}

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	dynamic    *dynamicfake.FakeDynamicClient
	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	snapshotLister []*clustersnapshot.Snapshot
	restoreLister  []*clustersnapshot.Restore
	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}
	return f
}

func newConfiguredSnapshot(name, phase string) *clustersnapshot.Snapshot {
	return &clustersnapshot.Snapshot{
		TypeMeta: metav1.TypeMeta{APIVersion: clustersnapshot.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clustersnapshot.SnapshotSpec{
			ClusterName:       name,
			Kubeconfig:        "kubeconfig",
			ObjectstoreConfig: "objectstoreConfig",
		},
		Status: clustersnapshot.SnapshotStatus{
			Phase: phase,
		},
	}
}

func newObjectstoreConfig() *clustersnapshot.ObjectstoreConfig {
	return &clustersnapshot.ObjectstoreConfig{
		TypeMeta: metav1.TypeMeta{APIVersion: clustersnapshot.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "objectstoreConfig",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clustersnapshot.ObjectstoreConfigSpec{
			Region:                "refion",
			Endpoint:              "endpoint",
			Bucket:                "bucket",
			CloudCredentialSecret: "cloudCredentialSecret",
		},
	}
}

func newRestorePreference() *clustersnapshot.RestorePreference {
	return &clustersnapshot.RestorePreference{
		TypeMeta: metav1.TypeMeta{APIVersion: clustersnapshot.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "restorePreference",
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clustersnapshot.RestorePreferenceSpec{},
	}
}

func newCloudCredentialSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloudCredentialSecret",
			Namespace: metav1.NamespaceDefault,
		},
		Data: map[string][]byte{
			"accesskey": []byte("YWNjZXNza2V5"),
			"secretkey": []byte("c2VjcmV0a2V5"),
		},
	}
}

func newConfiguredRestore(name, phase string) *clustersnapshot.Restore {
	return &clustersnapshot.Restore{
		TypeMeta: metav1.TypeMeta{APIVersion: clustersnapshot.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clustersnapshot.RestoreSpec{
			ClusterName:           name,
			Kubeconfig:            "kubeconfig",
			SnapshotName:          "snapshot",
			RestorePreferenceName: "restorePreference",
		},
		Status: clustersnapshot.RestoreStatus{
			Phase: phase,
		},
	}
}

// FakeCmd for fake command interface
type mockCluster struct {
	cluster.Cluster
}

// Snapshot for fake cluster interface
func (c *mockCluster) Snapshot(snapshot *cbv1alpha1.Snapshot) error {
	return nil
}

// UploadSnapshot for fake cluster interface
func (c *mockCluster) UploadSnapshot(snapshot *cbv1alpha1.Snapshot, bucket objectstore.Objectstore) error {
	return nil
}

// Restore for fake cluster interface
func (c *mockCluster) Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket objectstore.Objectstore) error {
	return nil
}

//func (f *fixture) newController() (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
func (f *fixture) newController() (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.dynamic = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	var maxRetryMin int
	maxRetryMin = 5
	c := NewController(
		f.kubeclient, f.dynamic, f.client,
		i.Clustersnapshot().V1alpha1().Snapshots(),
		i.Clustersnapshot().V1alpha1().Restores(),
		snapshotNamespace, true, true, true, false, false, maxRetryMin,
		&mockCluster{},
	)

	c.snapshotsSynced = alwaysReady
	c.restoresSynced = alwaysReady
	c.recorder = &record.FakeRecorder{}

	return c, i, k8sI
}

func (f *fixture) initInformers(i informers.SharedInformerFactory, k8sI kubeinformers.SharedInformerFactory) {
	for _, p := range f.snapshotLister {
		i.Clustersnapshot().V1alpha1().Snapshots().Informer().GetIndexer().Add(p)
	}

	for _, p := range f.restoreLister {
		i.Clustersnapshot().V1alpha1().Restores().Informer().GetIndexer().Add(p)
	}
}

func (f *fixture) startInformers(i informers.SharedInformerFactory, k8sI kubeinformers.SharedInformerFactory) {
	stopCh := make(chan struct{})
	defer close(stopCh)
	i.Start(stopCh)
	//k8sI.Start(stopCh)
}

func (f *fixture) run(c *Controller, name, res string) {
	f.runTest(c, name, res, false, false)
}

func (f *fixture) runQueueOnly(c *Controller, name, res string) {
	f.runTest(c, name, res, false, true)
}

func (f *fixture) runExpectError(c *Controller, name, res string) {
	f.runTest(c, name, res, true, false)
}

func (f *fixture) runTest(c *Controller, name, res string, expectError, queueOnly bool) {

	if name != "" {
		var err error
		if res == "restores" {
			err = c.restoreSyncHandler(name, queueOnly)
		} else {
			err = c.snapshotSyncHandler(name, queueOnly)
		}
		if !expectError && err != nil {
			f.t.Errorf("error syncing custom resource: %v", err)
		} else if expectError && err == nil {
			f.t.Error("expected error syncing custom resource, got nil")
		}
	}

	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	t.Logf("Chacking Action %s %s", actual.GetVerb(), actual.GetResource().Resource)

	switch a := actual.(type) {
	case core.CreateAction:
		e, _ := expected.(core.CreateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintDiff(expObject, object))
		}
	case core.UpdateAction:
		e, _ := expected.(core.UpdateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintDiff(expObject, object))
		}
	case core.PatchAction:
		e, _ := expected.(core.PatchAction)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintDiff(expPatch, patch))
		}
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	ret := []core.Action{}
	for _, action := range actions {
		if action.GetNamespace() == snapshotNamespace && (action.Matches("get", "objectstoreconfigs") ||
			action.Matches("get", "restorepreferences") ||
			action.Matches("list", "snapshots") ||
			action.Matches("get", "snapshots") ||
			action.Matches("watch", "snapshots")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectUpdateSnapshotAction(s *clustersnapshot.Snapshot) {
	f.actions = append(f.actions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "snapshots"}, s.Namespace, s))
}

func (f *fixture) expectDeleteSnapshotAction(s *clustersnapshot.Snapshot) {
	f.actions = append(f.actions, core.NewDeleteAction(schema.GroupVersionResource{Resource: "snapshots"}, s.Namespace, s.Name))
}

func (f *fixture) expectUpdateRestoreAction(s *clustersnapshot.Restore) {
	f.actions = append(f.actions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "restores"}, s.Namespace, s))
}

func (f *fixture) expectDeleteRestoreAction(s *clustersnapshot.Restore) {
	f.actions = append(f.actions, core.NewDeleteAction(schema.GroupVersionResource{Resource: "restores"}, s.Namespace, s.Name))
}

func (f *fixture) expectUpdateSnapshotStatusAction(s *clustersnapshot.Snapshot) {
	action := core.NewUpdateAction(schema.GroupVersionResource{Resource: "snapshots"}, s.Namespace, s)
	// TODO: Until #38113 is merged, we can't use Subresource
	//action.Subresource = "status"
	f.actions = append(f.actions, action)
}

func getKey(obj interface{}, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		t.Errorf("Unexpected error getting key for proxy %v: %v", obj, err)
		return ""
	}
	return key
}

func SnapshotTestCase(c *Case, t *testing.T) {
	f := newFixture(t)

	for _, s := range c.snapshots {
		f.snapshotLister = append(f.snapshotLister, s)
		f.objects = append(f.objects, s)
	}
	for _, config := range c.configs {
		f.objects = append(f.objects, config)
	}
	for _, secret := range c.secrets {
		f.kubeobjects = append(f.kubeobjects, secret)
	}

	cntl, i, k8sI := f.newController()

	for _, us := range c.updatedSnapshots {
		f.expectUpdateSnapshotAction(us)
	}
	for _, ds := range c.deleteSnapshots {
		f.expectDeleteSnapshotAction(ds)
	}

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)

	if c.queueOnly {
		f.runQueueOnly(cntl, "default/"+c.handleKey, "snapshots")
	} else {
		f.run(cntl, "default/"+c.handleKey, "snapshots")
	}
}

func RestoreTestCase(c *Case, t *testing.T) {
	f := newFixture(t)

	for _, s := range c.snapshots {
		f.snapshotLister = append(f.snapshotLister, s)
		f.objects = append(f.objects, s)
	}
	for _, r := range c.restores {
		f.restoreLister = append(f.restoreLister, r)
		f.objects = append(f.objects, r)
	}
	for _, config := range c.configs {
		f.objects = append(f.objects, config)
	}
	for _, pref := range c.preferences {
		f.objects = append(f.objects, pref)
	}
	for _, secret := range c.secrets {
		f.kubeobjects = append(f.kubeobjects, secret)
	}

	cntl, i, k8sI := f.newController()

	for _, ur := range c.updatedRestores {
		f.expectUpdateRestoreAction(ur)
	}
	for _, dr := range c.deleteRestores {
		f.expectDeleteRestoreAction(dr)
	}

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)

	if c.queueOnly {
		f.runQueueOnly(cntl, "default/"+c.handleKey, "restores")
	} else {
		f.run(cntl, "default/"+c.handleKey, "restores")
	}
}

func TestQueues(t *testing.T) {
	f := newFixture(t)

	cntl, _, _ := f.newController()

	snap := newConfiguredSnapshot("test1", "")
	cntl.enqueueSnapshot(snap)
	cntl.processNextSnapshotItem(false)
}

type bucketMock struct {
	objectstore.Objectstore
}

var deleteFilename string
func (b bucketMock) Delete(filename string) error {
	deleteFilename = filename
	return nil
}

var downloadFilename string
func (b bucketMock) Download(file *os.File, filename string) error {
	downloadFilename = filename
	return nil
}

var objectInfo *objectstore.ObjectInfo
func (b bucketMock) ListObjectInfo() ([]objectstore.ObjectInfo, error) {
	return []objectstore.ObjectInfo{*objectInfo}, nil
}

func getBucketMock(namespace, objectstoreConfig string, kubeclient kubernetes.Interface, client clientset.Interface, insecure bool) (objectstore.Objectstore, error) {
	return bucketMock{}, nil
}

func TestBucket(t *testing.T) {

	snap := newConfiguredSnapshot("test1", "Completed")
	snap2 := newConfiguredSnapshot("test2", "InProgress")
	f := newFixture(t)
	f.objects = append(f.objects, newObjectstoreConfig())
	f.kubeobjects = append(f.kubeobjects, newCloudCredentialSecret())
	f.objects = append(f.objects, snap)
	f.snapshotLister = append(f.snapshotLister, snap)
	f.objects = append(f.objects, snap2)
	f.snapshotLister = append(f.snapshotLister, snap2)
	cntl, i, k8sI := f.newController()
	cntl.getBucket = getBucketMock
	f.initInformers(i, k8sI)

	// Delete object
	cntl.deleteSnapshot(snap)
	if deleteFilename != "test1.tgz" {
		t.Errorf("Error in delete file name")
	}

	// Do nothing in syncObjects
	err := cntl.syncObjects(false, false, false)
	if err != nil {
		t.Errorf("Error in do nothing in syncObject : %s", err.Error())
	}

	// syncObjects no orphans
	objectInfo = &objectstore.ObjectInfo{
		Name:             "test1.tgz",
		Size:             int64(131072),
		Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
		BucketConfigName: "bucket",
	}
	err = cntl.syncObjects(true, false, false)
	if err != nil {
		t.Errorf("Error in syncObject : %s", err.Error())
	}

	// syncObjects orphan object
	objectInfo = &objectstore.ObjectInfo{
		Name:             "orphan.tgz",
		Size:             int64(131072),
		Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
		BucketConfigName: "bucket",
	}
	err = cntl.syncObjects(true, false, false)
	if err != nil {
		t.Errorf("Error in syncObject : %s", err.Error())
	}
	if deleteFilename != "orphan.tgz" {
		t.Errorf("Error in delete orphan object")
	}

	// syncObjects restore from object
	objectInfo = &objectstore.ObjectInfo{
		Name:             "restore.tgz",
		Size:             int64(131072),
		Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
		BucketConfigName: "bucket",
	}
	err = cntl.syncObjects(false, true, false)
	if err != nil {
		t.Errorf("Error in syncObject : %s", err.Error())
	}
	if downloadFilename != "restore.tgz" {
		t.Errorf("Error in download object to restore")
	}

	// Restore snapshot resource from test tgz file
	var kubeobjects []runtime.Object
	var ukubeobjects []runtime.Object
	kubeClient := k8sfake.NewSimpleClientset(kubeobjects...)
	sch := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(sch, ukubeobjects...)
	err = cluster.SnapshotWithClient(snap2, kubeClient, dynamicClient)
	if err != nil {
		t.Errorf("Error in snapshotWithClient : %s", err.Error())
	}
	err = cntl.restoreSnapshotFromObjectFile(objectstore.ObjectInfo{Name: "test2.tgz"})
	if err != nil {
		t.Errorf("Error in restoreSnapshotFromObjectFile : %s", err.Error())
	}
	restoredSnap, _ := cntl.cbclientset.ClustersnapshotV1alpha1().Snapshots(cntl.namespace).Get("test2", metav1.GetOptions{})
	if restoredSnap.Status.Phase != "Completed" {
		t.Errorf("Error restored snapshot status is not 'Completed' : %s", restoredSnap.Status.Phase)
	}
}

func int32Ptr(i int32) *int32 { return &i }
