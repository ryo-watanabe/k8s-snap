package main

import (
	"flag"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/cenkalti/backoff"
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
	"k8s.io/klog"

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
	restoreerror     error
	snaperror        error
	uploaderror      error
}

func newSnapshotCase(resultStatus, reason string) Case {
	c := Case{
		snapshots: []*clustersnapshot.Snapshot{
			newConfiguredSnapshot("test1", "InQueue"),
		},
		updatedSnapshots: []*clustersnapshot.Snapshot{
			newConfiguredSnapshot("test1", "InProgress"),
			newConfiguredSnapshot("test1", resultStatus),
		},
		configs: []*clustersnapshot.ObjectstoreConfig{
			newObjectstoreConfig(),
		},
		secrets: []*corev1.Secret{
			newCloudCredentialSecret(),
		},
		handleKey: "test1",
	}
	c.updatedSnapshots[1].Status.Reason = reason
	return c
}

func TestSnapshot(t *testing.T) {

	// Init klog
	klog.InitFlags(nil)
	flag.Set("logtostderr", "true")
	flag.Parse()
	klog.Infof("k8s-snap pkg main test")
	klog.Flush()

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
		newSnapshotCase("Completed", ""),
		// 3:InQueue > InProgress > Failed - secret not found
		newSnapshotCase("Failed", "secrets \"cloudCredentialSecret\" not found"),
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
		// 5:Mark Failed to InProgress snapshot
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
		// 6:Past date AvailableUntil
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", ""),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
			},
			handleKey: "test1",
		},
		// 7:Expiration for failed snapshot AvailableUntil set
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Failed"),
			},
			handleKey: "test1",
		},
		// 8:Expiration edited and deleted
		Case{
			snapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Completed"),
			},
			updatedSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Completed"),
			},
			deleteSnapshots: []*clustersnapshot.Snapshot{
				newConfiguredSnapshot("test1", "Completed"),
			},
			handleKey: "test1",
		},
		// 9:InQueue > InProgress > Failed - cluster.Snapshot returns error
		newSnapshotCase("Failed", "Mock cluster returns perm error"),
		// 10:InQueue > InProgress > Failed - cluster.Snapshot returns retry timeout
		newSnapshotCase("Failed", "Mock cluster returns not perm error"),
		// 11:InQueue > InProgress > Failed - cluster.Upload returns error
		newSnapshotCase("Failed", "Mock cluster upload returns perm error"),
		// 12:InQueue > InProgress > Failed - cluster.Upload returns retry timeout
		newSnapshotCase("Failed", "Mock cluster upload returns not perm error"),
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
	cases[3].secrets = nil
	// 4:Add expiration to failed snapshot
	cases[4].snapshots[0].Spec.TTL.Duration = dur
	cases[4].updatedSnapshots[0].Spec.TTL.Duration = dur
	cases[4].updatedSnapshots[0].Status.AvailableUntil = metav1.NewTime(cases[4].updatedSnapshots[0].ObjectMeta.CreationTimestamp.Add(dur))
	cases[4].updatedSnapshots[0].Status.TTL.Duration = dur
	// 5:Mark Failed to InProgress snapshot
	cases[5].updatedSnapshots[0].Status.Reason = "Controller stopped while taking the snapshot"
	cases[5].updatedSnapshots[1].Status.Reason = "Controller stopped while taking the snapshot"
	// 6:Past date AvailableUntil
	past := metav1.NewTime(time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC))
	cases[6].snapshots[0].Spec.AvailableUntil = past
	cases[6].updatedSnapshots[0].Spec.AvailableUntil = past
	cases[6].updatedSnapshots[0].Status.Reason = "AvailableUntil is set as past."
	// 7:Expiration for failed restore AvailableUntil set
	future := metav1.NewTime(time.Date(2050, 5, 20, 23, 59, 59, 0, time.UTC))
	cases[7].snapshots[0].Spec.AvailableUntil = future
	cases[7].updatedSnapshots[0].Spec.AvailableUntil = future
	cases[7].updatedSnapshots[0].Status.AvailableUntil = future
	cases[7].updatedSnapshots[0].Status.TTL.Duration = future.Time.Sub(cases[7].snapshots[0].ObjectMeta.CreationTimestamp.Time)
	// 8:Expiration edited
	cases[8].snapshots[0].Spec.AvailableUntil = past
	cases[8].snapshots[0].Status.AvailableUntil = future
	cases[8].updatedSnapshots[0].Spec.AvailableUntil = past
	cases[8].updatedSnapshots[0].Status.AvailableUntil = past
	// 9:InQueue > InProgress > Failed - cluster.Snapshot returns error
	cases[9].snaperror = backoff.Permanent(fmt.Errorf("Mock cluster returns perm error"))
	// 10:InQueue > InProgress > Failed - cluster.Snapshot returns retry timeout
	cases[10].snaperror = fmt.Errorf("Mock cluster returns not perm error")
	// 11:InQueue > InProgress > Failed - cluster.Upload returns error
	cases[11].uploaderror = backoff.Permanent(fmt.Errorf("Mock cluster upload returns perm error"))
	// 12:InQueue > InProgress > Failed - cluster.Upload returns retry timeout
	cases[12].uploaderror = fmt.Errorf("Mock cluster upload returns not perm error")

	for _, c := range cases {
		SnapshotTestCase(&c, t)
	}
}

func newRestoreCase(resultStatus, reason string) Case {
	c := Case{
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
			newConfiguredRestore("test1", resultStatus),
		},
		configs: []*clustersnapshot.ObjectstoreConfig{
			newObjectstoreConfig(),
		},
		secrets: []*corev1.Secret{
			newCloudCredentialSecret(),
		},
		handleKey: "test1",
	}
	c.updatedRestores[1].Status.Reason = reason
	c.restores[0].Spec.TTL.Duration, _ = time.ParseDuration("168h0m0s")
	c.updatedRestores[0].Spec.TTL.Duration, _ = time.ParseDuration("168h0m0s")
	c.updatedRestores[1].Spec.TTL.Duration, _ = time.ParseDuration("168h0m0s")
	return c
}

func TestRestore(t *testing.T) {

	cases := []Case{
		// 0:create restore
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
		newRestoreCase("Completed", ""),
		// 2:InQueue > InProgress > Failed with restore error
		newRestoreCase("Failed", "Mock cluster resturns a error"),
		// 3:InQueue > InProgress > Failed with invalid snapshot
		newRestoreCase("Failed", "Snapshot data is not in status 'Completed'"),
		// 4:InQueue > InProgress > Failed with snapshot not found
		newRestoreCase("Failed", "snapshots.clustersnapshot.rywt.io \"snapshot\" not found"),
		// 5:InQueue > InProgress > Failed with config not found
		newRestoreCase("Failed", "objectstoreconfigs.clustersnapshot.rywt.io \"objectstoreConfig\" not found"),
		// 6:InQueue > InProgress > Failed with secret not found
		newRestoreCase("Failed", "secrets \"cloudCredentialSecret\" not found"),
		// 7:InQueue > InProgress > Failed with preference not found
		newRestoreCase("Failed", "restorepreferences.clustersnapshot.rywt.io \"restorePreference\" not found"),
		// 8:Expiration for failed restore and delete
		Case{
			restores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Failed"),
			},
			updatedRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Failed"),
			},
			deleteRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Failed"),
			},
			handleKey: "test1",
		},
		// 9:Past date AvailableUntil
		Case{
			restores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", ""),
			},
			updatedRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Failed"),
			},
			handleKey: "test1",
		},
		// 10:Expiration for failed restore AvailableUntil set
		Case{
			restores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Failed"),
			},
			updatedRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Failed"),
			},
			handleKey: "test1",
		},
		// 11:Expiration edited and deleted
		Case{
			restores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Completed"),
			},
			updatedRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Completed"),
			},
			deleteRestores: []*clustersnapshot.Restore{
				newConfiguredRestore("test1", "Completed"),
			},
			handleKey: "test1",
		},
	}

	dur, _ := time.ParseDuration("168h0m0s")

	// Additional test data:
	// Create restore
	cases[0].updatedRestores[0].Spec.TTL.Duration = dur
	// 1:InQueue > InProgress > Completed
	// 2:InQueue > InProgress > Failed with restore error
	cases[2].restoreerror = fmt.Errorf("Mock cluster resturns a error")
	// 3:InQueue > InProgress > Failed with invalid snapshot
	cases[3].snapshots[0].Status.Phase = "Failed"
	// 4:InQueue > InProgress > Failed with snapshot not found
	cases[4].snapshots = nil
	// 5:InQueue > InProgress > Failed with config not found
	cases[5].configs = nil
	// 6:InQueue > InProgress > Failed with secret not found
	cases[6].secrets = nil
	// 7:InQueue > InProgress > Failed with preference not found
	cases[7].preferences = nil
	// 8:Expiration for failed restore
	cases[8].restores[0].Spec.TTL.Duration = dur
	cases[8].updatedRestores[0].Spec.TTL.Duration = dur
	cases[8].updatedRestores[0].Status.AvailableUntil = metav1.NewTime(cases[8].restores[0].ObjectMeta.CreationTimestamp.Add(dur))
	cases[8].updatedRestores[0].Status.TTL.Duration = dur
	// 9:Past date AvailableUntil
	past := metav1.NewTime(time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC))
	cases[9].restores[0].Spec.AvailableUntil = past
	cases[9].updatedRestores[0].Spec.AvailableUntil = past
	cases[9].updatedRestores[0].Status.Reason = "AvailableUntil is set as past."
	// 10:Expiration for failed restore AvailableUntil set
	future := metav1.NewTime(time.Date(2050, 5, 20, 23, 59, 59, 0, time.UTC))
	cases[10].restores[0].Spec.AvailableUntil = future
	cases[10].updatedRestores[0].Spec.AvailableUntil = future
	cases[10].updatedRestores[0].Status.AvailableUntil = future
	cases[10].updatedRestores[0].Status.TTL.Duration = future.Time.Sub(cases[10].restores[0].ObjectMeta.CreationTimestamp.Time)
	// 11:Expiration edited
	cases[11].restores[0].Spec.AvailableUntil = past
	cases[11].restores[0].Status.AvailableUntil = future
	cases[11].updatedRestores[0].Spec.AvailableUntil = past
	cases[11].updatedRestores[0].Status.AvailableUntil = past

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
var snapshotErr error

func (c *mockCluster) Snapshot(snapshot *cbv1alpha1.Snapshot) error {
	return snapshotErr
}

// UploadSnapshot for fake cluster interface
var uploadErr error

func (c *mockCluster) UploadSnapshot(snapshot *cbv1alpha1.Snapshot, bucket objectstore.Objectstore) error {
	return uploadErr
}

// Restore for fake cluster interface
var restoreErr error

func (c *mockCluster) Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket objectstore.Objectstore) error {
	return restoreErr
}

//func (f *fixture) newController() (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
func (f *fixture) newController() (*Controller, informers.SharedInformerFactory, kubeinformers.SharedInformerFactory) {
	f.client = fake.NewSimpleClientset(f.objects...)
	f.dynamic = dynamicfake.NewSimpleDynamicClient(runtime.NewScheme())
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	i := informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	k8sI := kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c := NewController(
		f.kubeclient, f.dynamic, f.client,
		i.Clustersnapshot().V1alpha1().Snapshots(),
		i.Clustersnapshot().V1alpha1().Restores(),
		snapshotNamespace, true, true, true, false, true, 5,
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

	//t.Logf("Chacking Action %s %s", actual.GetVerb(), actual.GetResource().Resource)

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

	snapshotErr = c.snaperror
	uploadErr = c.uploaderror

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)

	if c.queueOnly {
		f.runQueueOnly(cntl, "default/"+c.handleKey, "snapshots")
	} else {
		f.run(cntl, "default/"+c.handleKey, "snapshots")
	}

	snapshotErr = nil
	uploadErr = nil
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

	restoreErr = c.restoreerror

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)

	if c.queueOnly {
		f.runQueueOnly(cntl, "default/"+c.handleKey, "restores")
	} else {
		f.run(cntl, "default/"+c.handleKey, "restores")
	}

	restoreErr = nil
}

func TestQueues(t *testing.T) {

	f := newFixture(t)

	snap := newConfiguredSnapshot("test1", "")
	f.objects = append(f.objects, snap)
	f.snapshotLister = append(f.snapshotLister, snap)
	restore := newConfiguredRestore("test1", "")
	f.objects = append(f.objects, restore)
	f.restoreLister = append(f.restoreLister, restore)

	cntl, i, k8sI := f.newController()
	f.initInformers(i, k8sI)

	cntl.enqueueSnapshot(snap)
	cntl.processNextSnapshotItem(false)
	handledSnap, _ := cntl.cbclientset.ClustersnapshotV1alpha1().Snapshots(cntl.namespace).Get("test1", metav1.GetOptions{})
	if handledSnap.Status.Phase != "InQueue" {
		t.Errorf("Handled snap status not correct : %s", handledSnap.Status.Phase)
	}

	cntl.enqueueRestore(restore)
	cntl.processNextRestoreItem(false)
	handledRestore, _ := cntl.cbclientset.ClustersnapshotV1alpha1().Restores(cntl.namespace).Get("test1", metav1.GetOptions{})
	if handledRestore.Status.Phase != "InQueue" {
		t.Errorf("Handled restore status not correct : %s", handledRestore.Status.Phase)
	}
}

type bucketMock struct {
	objectstore.Objectstore
}

func (b bucketMock) GetName() string {
	return "config"
}
func (b bucketMock) GetEndpoint() string {
	return "example.com"
}
func (b bucketMock) GetBucketName() string {
	return "bucket"
}

var bucketFound bool

func (b bucketMock) ChkBucket() (bool, error) {
	return bucketFound, nil
}

func (b bucketMock) CreateBucket() error {
	return nil
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

var objectInfoList []objectstore.ObjectInfo

func (b bucketMock) ListObjectInfo() ([]objectstore.ObjectInfo, error) {
	return objectInfoList, nil
}

func getBucketMock(namespace, objectstoreConfig string, kubeclient kubernetes.Interface, client clientset.Interface, insecure bool) (objectstore.Objectstore, error) {
	return bucketMock{}, nil
}

func newBucketTestController(t *testing.T, snapshots []*clustersnapshot.Snapshot) *Controller {
	f := newFixture(t)
	f.objects = append(f.objects, newObjectstoreConfig())
	f.kubeobjects = append(f.kubeobjects, newCloudCredentialSecret())
	for _, snap := range snapshots {
		f.objects = append(f.objects, snap)
		f.snapshotLister = append(f.snapshotLister, snap)
	}
	cntl, i, k8sI := f.newController()
	cntl.getBucket = getBucketMock
	f.initInformers(i, k8sI)
	return cntl
}

func doSyncObjects(t *testing.T, cntl *Controller, deleteOrphanObjects, restoreOrphanedSnapshots, validateFileinfo bool) {
	err := cntl.syncObjects(deleteOrphanObjects, restoreOrphanedSnapshots, validateFileinfo)
	if err != nil {
		t.Errorf("Error in do nothing in syncObject : %s", err.Error())
	}
}

func chkSnapshot(t *testing.T, cntl *Controller, name, status, reason string) {
	snap, err := cntl.cbclientset.ClustersnapshotV1alpha1().Snapshots(cntl.namespace).Get(name, metav1.GetOptions{})
	if err != nil {
		t.Errorf("Error get snapshot %s : %s", name, err.Error())
	}
	if snap.Status.Phase != status || snap.Status.Reason != reason {
		t.Errorf("Error snapshot is not expected (%s:%s) : %v", status, reason, snap)
	}
}

func TestBucket(t *testing.T) {

	// Delete object
	snapshots := []*clustersnapshot.Snapshot{newConfiguredSnapshot("test1", "Completed")}
	cntl := newBucketTestController(t, snapshots)
	cntl.deleteSnapshot(snapshots[0])
	if deleteFilename != "test1.tgz" {
		t.Errorf("Error in delete file name")
	}

	// Do nothing in syncObjects
	snapshots = []*clustersnapshot.Snapshot{}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, false, false, false)

	// syncObjects bucket data repair
	snapshots = []*clustersnapshot.Snapshot{newConfiguredSnapshot("test1", "Completed")}
	snapshots[0].Status.StoredTimestamp = metav1.NewTime(time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC)).Rfc3339Copy()
	snapshots[0].Status.StoredFileSize = int64(131072)
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "test1.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "bucket",
		},
	}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, true, false, false)
	updatedSnap, _ := cntl.cbclientset.ClustersnapshotV1alpha1().Snapshots(cntl.namespace).Get("test1", metav1.GetOptions{})
	if updatedSnap.Spec.ObjectstoreConfig != "bucket" {
		t.Errorf("Error updated snapshot config is not match : %s", updatedSnap.Spec.ObjectstoreConfig)
	}

	// syncObjects orphan object and delete
	snapshots = []*clustersnapshot.Snapshot{}
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "orphan.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "bucket",
		},
	}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, true, false, false)
	if deleteFilename != "orphan.tgz" {
		t.Errorf("Error in delete orphan object")
	}

	// syncObjects find snapshot without object and set Failed
	snapshots = []*clustersnapshot.Snapshot{
		newConfiguredSnapshot("test1", "Completed"),
	}
	objectInfoList = []objectstore.ObjectInfo{}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, true, false, false)
	chkSnapshot(t, cntl, "test1", "Failed", "Snapshot file not found")

	// syncObjects validate size/timestamp and set Failed
	snapshots = []*clustersnapshot.Snapshot{
		newConfiguredSnapshot("test1", "Completed"),
	}
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "test1.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "objectstoreConfig",
		},
	}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, true, false, true)
	chkSnapshot(t, cntl, "test1", "Failed", "Snapshot file size or timestamp not matched")

	// syncObjects not validate and set object invalid snap Completed
	snapshots = []*clustersnapshot.Snapshot{
		newConfiguredSnapshot("test1", "Failed"),
	}
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "test1.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "bucket",
		},
	}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, true, false, false)
	chkSnapshot(t, cntl, "test1", "Completed", "")

	// syncObjects re-mark Failed snap to Completed
	snapshots = []*clustersnapshot.Snapshot{
		newConfiguredSnapshot("test1", "Failed"),
	}
	snapshots[0].Status.StoredTimestamp = metav1.NewTime(time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC)).Rfc3339Copy()
	snapshots[0].Status.StoredFileSize = int64(131072)
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "test1.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "objectstoreConfig",
		},
	}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, true, false, false)
	chkSnapshot(t, cntl, "test1", "Completed", "")

	// syncObjects downloads tgz file to restore
	snapshots = []*clustersnapshot.Snapshot{}
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "restore.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "bucket",
		},
	}
	cntl = newBucketTestController(t, snapshots)
	doSyncObjects(t, cntl, false, true, false)
	if downloadFilename != "restore.tgz" {
		t.Errorf("Error in download object to restore")
	}

	// Restore snapshot resource from test tgz file
	snapshots = []*clustersnapshot.Snapshot{
		newConfiguredSnapshot("test1", "InProgress"),
	}
	cntl = newBucketTestController(t, snapshots)
	var kubeobjects []runtime.Object
	var ukubeobjects []runtime.Object
	kubeClient := k8sfake.NewSimpleClientset(kubeobjects...)
	sch := runtime.NewScheme()
	dynamicClient := dynamicfake.NewSimpleDynamicClient(sch, ukubeobjects...)
	err := cluster.SnapshotWithClient(snapshots[0], kubeClient, dynamicClient)
	if err != nil {
		t.Errorf("Error in snapshotWithClient : %s", err.Error())
	}
	err = cntl.restoreSnapshotFromObjectFile(objectstore.ObjectInfo{Name: "test1.tgz"})
	if err != nil {
		t.Errorf("Error in restoreSnapshotFromObjectFile : %s", err.Error())
	}
	chkSnapshot(t, cntl, "test1", "Completed", "")
}

func TestControllerRun(t *testing.T) {

	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Namespace"},
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	config := newObjectstoreConfig()
	objectInfoList = []objectstore.ObjectInfo{
		objectstore.ObjectInfo{
			Name:             "test1.tgz",
			Size:             int64(131072),
			Timestamp:        time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC),
			BucketConfigName: "bucket",
		},
	}
	snap := newConfiguredSnapshot("test1", "Completed")

	f := newFixture(t)
	f.kubeobjects = append(f.kubeobjects, ns)
	f.objects = append(f.objects, config)
	f.objects = append(f.objects, snap)
	f.snapshotLister = append(f.snapshotLister, snap)
	cntl, i, k8sI := f.newController()
	cntl.getBucket = getBucketMock
	f.initInformers(i, k8sI)

	// Run controller
	var err error
	bucketFound = true
	stopCh := make(chan struct{})
	go func() {
		err = cntl.Run(1, 1, stopCh)
	}()
	time.Sleep(1 * time.Second)
	close(stopCh)
	if err != nil {
		t.Errorf("Error in controller Run : %s", err.Error())
	}

	// Run controller bucket not found
	bucketFound = false
	stopCh = make(chan struct{})
	go func() {
		err = cntl.Run(1, 1, stopCh)
	}()
	time.Sleep(1 * time.Second)
	close(stopCh)
	if err != nil {
		t.Errorf("Error in controller Run : %s", err.Error())
	}
}

func int32Ptr(i int32) *int32 { return &i }
