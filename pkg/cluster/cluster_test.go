package cluster

import (
	"os"
	"strconv"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apimachinery/pkg/watch"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"

	clustersnapshot "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
)

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

func newConfiguredSecret() *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Secret"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloudCredentialSecret",
			Namespace: metav1.NamespaceDefault,
			SelfLink:  "/api/v1/namespaces/default/secrets/cloudCredentialSecret",
		},
		Data: map[string][]byte{
			"accesskey": []byte("YWNjZXNza2V5"),
			"secretkey": []byte("c2VjcmV0a2V5"),
		},
	}
}

var kubeobjects []runtime.Object
var ukubeobjects []runtime.Object

type bucketMock struct {
	objectstore.Objectstore
}

var uploadFilename string

func (b bucketMock) Upload(file *os.File, filename string) error {
	uploadFilename = filename
	return nil
}

var objectInfo *objectstore.ObjectInfo
var getObjectInfoFilename string

func (b bucketMock) GetObjectInfo(filename string) (*objectstore.ObjectInfo, error) {
	getObjectInfoFilename = filename
	return objectInfo, nil
}

//var intResourceVersion uint64
var dynamicTracker ObjectTracker

func newDynamicClient(scheme *runtime.Scheme, rv uint64, objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	// In order to use List with this client, you have to have the v1.List registered in your scheme. Neat thing though
	// it does NOT have to be the *same* list
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "fake-dynamic-client-group", Version: "v1", Kind: "List"}, &unstructured.UnstructuredList{})

	codecs := serializer.NewCodecFactory(scheme)
	dynamicTracker = NewObjectTracker(scheme, codecs.UniversalDecoder(), rv)
	for _, obj := range objects {
		if err := dynamicTracker.Add(obj); err != nil {
			panic(err)
		}
	}

	cs := &dynamicfake.FakeDynamicClient{}
	cs.AddReactor("*", "*", core.ObjectReaction(dynamicTracker))
	cs.AddWatchReactor("*", func(action core.Action) (handled bool, ret watch.Interface, err error) {
		gvr := action.GetResource()
		ns := action.GetNamespace()
		watch, err := dynamicTracker.Watch(gvr, ns)
		if err != nil {
			return false, nil, err
		}
		return true, watch, nil
	})

	return cs
}

func TestSnapshot(t *testing.T) {

	secret := newConfiguredSecret()
	kubeobjects = append(kubeobjects, secret)
	kubeClient := k8sfake.NewSimpleClientset(kubeobjects...)
	kubeClient.Discovery().(*discoveryfake.FakeDiscovery).Fake.Resources = []*metav1.APIResourceList{
		&metav1.APIResourceList{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				metav1.APIResource{
					Name: "secrets",
					Version: "v1",
					Kind: "Secret",
					Verbs: []string{"list", "create", "get", "delete"},
					Namespaced: true,
				},
			},
		},
		&metav1.APIResourceList{
			GroupVersion: "v1",
			APIResources: []metav1.APIResource{
				metav1.APIResource{
					Name: "configmaps",
					Version: "v1",
					Kind: "ConfigMap",
					Verbs: []string{"list", "create", "get", "delete"},
					Namespaced: true,
				},
			},
		},
	}

	sch := runtime.NewScheme()
	sch.AddKnownTypeWithName(schema.GroupVersionKind{Version:"v1", Kind:"Secret"}, secret)
	mapsecret, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	usecret := &unstructured.Unstructured{Object: mapsecret}
	ukubeobjects = append(ukubeobjects, usecret.DeepCopyObject())
	dynamicClient := newDynamicClient(sch, 1, ukubeobjects...)

	kubeClient.Fake.PrependReactor("create", "*", func(action core.Action) (bool, runtime.Object, error) {
		t.Logf("Create %s called", action.GetResource().Resource)
		obj := action.(core.CreateAction).GetObject()
		mapObject, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
		uObject := &unstructured.Unstructured{Object: mapObject}
		newAction := core.NewCreateAction(action.GetResource(), action.GetNamespace(), uObject.DeepCopyObject()).DeepCopy()
		dynamicClient.Fake.Invokes(newAction, uObject.DeepCopyObject())
		accessor, _ := meta.Accessor(obj)
		accessor.SetResourceVersion(strconv.FormatUint(dynamicTracker.GetResourceVersion(), 10))
		return false, obj, nil
	})
	kubeClient.Fake.PrependReactor("delete", "*", func(action core.Action) (bool, runtime.Object, error) {
		t.Logf("Delete ConfigMap called")
		dynamicClient.Fake.Invokes(action, nil)
		return false, nil, nil
	})

	snap := newConfiguredSnapshot("test1", "InProgress")

	// Get a snapshot
	err := snapshotWithClient(snap, kubeClient, dynamicClient)
	if err != nil {
		t.Errorf("Error in snapshotWithClient : %s", err.Error())
	}

	// Uplaod the snapshot file
	bucket := &bucketMock{}
	objSize := int64(131072)
	objTime := time.Date(2001, 5, 20, 23, 59, 59, 0, time.UTC)
	objectInfo = &objectstore.ObjectInfo{Name:"test1.tgz", Size: objSize, Timestamp: objTime, BucketConfigName: "bucket"}
	err = UploadSnapshot(snap, bucket)
	if err != nil {
		t.Errorf("Error in UploadSnapshot : %s", err.Error())
	}
	if uploadFilename != "test1.tgz" {
		t.Error("Error upload filename not match")
	}
	if getObjectInfoFilename != "test1.tgz" {
		t.Error("Error GetObjectInfo filename not match")
	}
	if snap.Status.StoredFileSize != objSize {
		t.Error("Error file size not match")
	}
	metav1ObjTime := metav1.NewTime(objTime)
	if !snap.Status.StoredTimestamp.Equal(&metav1ObjTime) {
		t.Error("Error timestamp not match")
	}
}
