package main

import (
	//"fmt"
	"reflect"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"

	clustersnapshot "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	informers "github.com/ryo-watanabe/k8s-snap/pkg/client/informers/externalversions"

	"github.com/ryo-watanabe/k8s-snap/pkg/client/clientset/versioned/fake"
	"github.com/ryo-watanabe/k8s-snap/pkg/cluster"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
	snapshotNamespace = "default"
)

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	dynamic	   *dynamicfake.FakeDynamicClient
	kubeclient *k8sfake.Clientset
	// Objects to put in the store.
	snapshotLister     []*clustersnapshot.Snapshot
        restoreLister     []*clustersnapshot.Restore
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
			ClusterName:    name,
			Kubeconfig:     "kubeconfig",
			ObjectstoreConfig:  "objectstoreConfig",
		},
		Status: clustersnapshot.SnapshotStatus{
			Phase:                   phase,
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
			Region: "refion",
			Endpoint: "endpoint",
			Bucket: "bucket",
			CloudCredentialSecret: "cloudCredentialSecret",
		},
	}
}

func newCloudCredentialSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind:"Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloudCredentialSecret",
			Namespace: metav1.NamespaceDefault,
		},
		Data: map[string][]byte {
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
			ClusterName:    name,
			Kubeconfig:     "kubeconfig",
			SnapshotName:  "snapshot",
		},
		Status: clustersnapshot.RestoreStatus{
			Phase:                   phase,
		},
	}
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
		snapshotNamespace, true, true, true, maxRetryMin,
		cluster.NewFakeClusterCmd(),
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
		if action.GetNamespace() == snapshotNamespace && (
				action.Matches("get", "objectstoreconfigs") ||
				action.Matches("list", "snapshots") ||
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

func (f *fixture) expectUpdateRestoreAction(s *clustersnapshot.Restore) {
	f.actions = append(f.actions, core.NewUpdateAction(schema.GroupVersionResource{Resource: "restores"}, s.Namespace, s))
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

func TestCreateSnapshot(t *testing.T) {
	f := newFixture(t)

	// pre-existing snapshot resource
	s1 := newConfiguredSnapshot("test1", "Completed")
	f.snapshotLister = append(f.snapshotLister, s1)
	f.objects = append(f.objects, s1)
        // new snapshot resource
	s2 := newConfiguredSnapshot("test2", "")
	f.snapshotLister = append(f.snapshotLister, s2)
	f.objects = append(f.objects, s2)

	c, i, k8sI := f.newController()

	ex_snapshot := newConfiguredSnapshot("test2", "InQueue")
        ex_snapshot.Spec.TTL.Duration, _ = time.ParseDuration("720h0m0s")
	f.expectUpdateSnapshotAction(ex_snapshot)

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)
	f.runQueueOnly(c, getKey(ex_snapshot, t), "snapshots")
}

func TestCreateSnapshotRFC3339(t *testing.T) {
	f := newFixture(t)

        // new snapshot resource
	s1 := newConfiguredSnapshot("test1", "")
        s1.Spec.AvailableUntil.Time, _ = time.Parse(time.RFC3339, "2020-07-01T02:03:04Z")
	f.snapshotLister = append(f.snapshotLister, s1)
	f.objects = append(f.objects, s1)

	c, i, k8sI := f.newController()

	ex_snapshot := newConfiguredSnapshot("test1", "InQueue")
        ex_snapshot.Spec.AvailableUntil.Time = time.Date(2020, time.July, 1, 2, 3, 4, 0, time.UTC)
	f.expectUpdateSnapshotAction(ex_snapshot)

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)
	f.runQueueOnly(c, getKey(ex_snapshot, t), "snapshots")
}

func TestInProgressAndCompletedSnapshot(t *testing.T) {
	f := newFixture(t)

	// pre-existing snapshot resource
	s1 := newConfiguredSnapshot("test1", "Completed")
	f.snapshotLister = append(f.snapshotLister, s1)
	f.objects = append(f.objects, s1)
        // new snapshot resource
	s2 := newConfiguredSnapshot("test2", "InQueue")
	f.snapshotLister = append(f.snapshotLister, s2)
	f.objects = append(f.objects, s2)
	// bucket config
	b := newObjectstoreConfig()
	f.objects = append(f.objects, b)
	secret := newCloudCredentialSecret()
	f.kubeobjects = append(f.kubeobjects, secret)

	c, i, k8sI := f.newController()

	ex_snapshot := newConfiguredSnapshot("test2", "InProgress")
	f.expectUpdateSnapshotAction(ex_snapshot)
	ex2_snapshot := newConfiguredSnapshot("test2", "Completed")
	f.expectUpdateSnapshotAction(ex2_snapshot)

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)
	f.run(c, getKey(ex_snapshot, t), "snapshots")
}

func TestCreateRestore(t *testing.T) {
	f := newFixture(t)

	// pre-existing snapshot resource
	s1 := newConfiguredRestore("test1", "Completed")
	f.restoreLister = append(f.restoreLister, s1)
	f.objects = append(f.objects, s1)
        // new snapshot resource
	s2 := newConfiguredRestore("test2", "")
	f.restoreLister = append(f.restoreLister, s2)
	f.objects = append(f.objects, s2)

	c, i, k8sI := f.newController()

	ex_restore := newConfiguredRestore("test2", "InQueue")
        ex_restore.Spec.TTL.Duration, _ = time.ParseDuration("168h0m0s")
	f.expectUpdateRestoreAction(ex_restore)

	f.initInformers(i, k8sI)
	f.startInformers(i, k8sI)
	f.runQueueOnly(c, getKey(ex_restore, t), "restores")
}

func int32Ptr(i int32) *int32 { return &i }
