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
	rbac "k8s.io/api/rbac/v1"

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

func newConfiguredConfigMap(name, data string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			SelfLink:  "/api/v1/namespaces/default/configmaps/" + name,
		},
		Data: map[string]string{
			"message": data,
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

var downloadFilename string

func (b bucketMock) Download(file *os.File, filename string) error {
	downloadFilename = filename
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
	scheme.AddKnownTypeWithName(schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRoleBinding"}, &unstructured.Unstructured{})

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

func convertToUnstructured(t *testing.T, obj runtime.Object) (runtime.Object) {
	maped, err := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	if err != nil {
		t.Errorf("Error converting object to unstructured : %s", err.Error())
	}
	unstrctrd := &unstructured.Unstructured{Object: maped}
	return unstrctrd.DeepCopyObject()
}

func unstrctrdResourceV1(ns, name, kind, resourcename string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{}
	item.SetGroupVersionKind(schema.GroupVersionKind{Version:"v1", Kind:kind})
	item.SetSelfLink("/api/v1/namespaces/" + ns + "/" + resourcename + "/" + name)
	return item
}

func unstrctrdResource(group, version, ns, name, kind, resourcename string) *unstructured.Unstructured {
	item := &unstructured.Unstructured{}
	item.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind:kind})
	item.SetSelfLink("/apis/" + group + "/" + version + "/namespaces/" + ns + "/" + resourcename + "/" + name)
	return item
}

func newRestorePreference(name string) *clustersnapshot.RestorePreference {
	return &clustersnapshot.RestorePreference{
		TypeMeta: metav1.TypeMeta{APIVersion: clustersnapshot.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clustersnapshot.RestorePreferenceSpec{
			ExcludeNamespaces: []string{"kube-system"},
		},
	}
}

func newConfiguredRestore(name, snapshot, preference, phase string) *clustersnapshot.Restore {
	return &clustersnapshot.Restore{
		TypeMeta: metav1.TypeMeta{APIVersion: clustersnapshot.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
		},
		Spec: clustersnapshot.RestoreSpec{
			ClusterName:           name,
			Kubeconfig:            "kubeconfig",
			SnapshotName:          snapshot,
			RestorePreferenceName: preference,
		},
		Status: clustersnapshot.RestoreStatus{
			Phase: phase,
		},
	}
}

func newClusterRoleBinding() *rbac.ClusterRoleBinding {
	return &rbac.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{APIVersion: "rbac.authorization.k8s.io/v1", Kind: "ClusterRoleBinding"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cluster-admin-default",
			Namespace: metav1.NamespaceDefault,
			SelfLink:  "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/cluster-admin-default",
		},
		Subjects: []rbac.Subject{
			rbac.Subject{Kind: "ServiceAccount", Name: "default", Namespace: "default"},
		},
		RoleRef: rbac.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}
}

func getAPIResourceListV1(name, kind string) *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: "v1",
		APIResources: []metav1.APIResource{
			metav1.APIResource{Name: name, Version: "v1", Kind: kind, Namespaced: true,
				Verbs: []string{"list", "create", "get", "delete"},
			},
		},
	}
}

func getAPIResourceList(group, version, name, kind string) *metav1.APIResourceList {
	return &metav1.APIResourceList{
		GroupVersion: group + "/" + version,
		APIResources: []metav1.APIResource{
			metav1.APIResource{Name: name, Group: group, Version: version, Kind: kind, Namespaced: true,
				Verbs: []string{"list", "create", "get", "delete"},
			},
		},
	}
}

func TestSnapshot(t *testing.T) {

	kubeClient := k8sfake.NewSimpleClientset(kubeobjects...)
	kubeClient.Discovery().(*discoveryfake.FakeDiscovery).Fake.Resources = []*metav1.APIResourceList{
		getAPIResourceListV1("secrets", "Secret"),
		getAPIResourceListV1("configmaps", "ConfigMaps"),
		getAPIResourceListV1("services", "Service"),
		getAPIResourceListV1("endpoints", "Endpoints"),
		getAPIResourceListV1("pods", "Pod"),
		getAPIResourceList("rbac.authorization.k8s.io", "v1", "clusterrolebindings", "ClusterRoleBinding"),
	}

	// Set resouces stored in snapshots
	ukubeobjects = append(ukubeobjects, convertToUnstructured(t, newConfiguredSecret()))
	ukubeobjects = append(ukubeobjects, unstrctrdResourceV1("default", "svc1", "Service", "services").DeepCopyObject())
	ukubeobjects = append(ukubeobjects, unstrctrdResourceV1("default", "svc1", "Endpoints", "endpoints").DeepCopyObject())
	ukubeobjects = append(ukubeobjects, unstrctrdResourceV1("kube-system", "secret1", "Secret", "secrets").DeepCopyObject())
	pod := unstrctrdResourceV1("default", "pod1", "Pod", "pods")
	pod.SetOwnerReferences([]metav1.OwnerReference{metav1.OwnerReference{Name:"Pod-owner"}})
	ukubeobjects = append(ukubeobjects, pod.DeepCopyObject())
	ukubeobjects = append(ukubeobjects, convertToUnstructured(t, newClusterRoleBinding()).DeepCopyObject())

	sch := runtime.NewScheme()
	dynamicClient := newDynamicClient(sch, 1, ukubeobjects...)
	var snapstart bool

	kubeClient.Fake.PrependReactor("create", "*", func(action core.Action) (bool, runtime.Object, error) {

		t.Logf("Create %s called", action.GetResource().Resource)

		// Add/Delete/Modify contents during snapshot
		if snapstart {
			err := dynamicTracker.Add(convertToUnstructured(t, newConfiguredConfigMap("test1", "test1")))
			if err != nil {
				t.Errorf("Error in create configmap : %s", err.Error())
			}
			err = dynamicTracker.Update(
				schema.GroupVersionResource{Version:"v1", Resource:"configmaps"},
				convertToUnstructured(t, newConfiguredConfigMap("test1", "test1_edited")),
				"default",
			)
			if err != nil {
				t.Errorf("Error in update configmap : %s", err.Error())
			}
			err = dynamicTracker.Delete(
				schema.GroupVersionResource{Version:"v1", Resource:"configmaps"},
				"default",
				"test1",
			)
			if err != nil {
				t.Errorf("Error in delete configmap : %s", err.Error())
			}
		}

		obj := action.(core.CreateAction).GetObject()
		uobj := convertToUnstructured(t, obj)
		newAction := core.NewCreateAction(action.GetResource(), action.GetNamespace(), uobj).DeepCopy()
		dynamicClient.Fake.Invokes(newAction, uobj)
		accessor, _ := meta.Accessor(obj)
		accessor.SetResourceVersion(strconv.FormatUint(dynamicTracker.GetResourceVersion(), 10))

		// toggle snapstart switch
		snapstart = !snapstart

		return false, obj, nil
	})

	kubeClient.Fake.PrependReactor("delete", "*", func(action core.Action) (bool, runtime.Object, error) {
		t.Logf("Delete %s called", action.GetResource().Resource)
		dynamicClient.Fake.Invokes(action, nil)
		return false, nil, nil
	})

	snapstart = false
	snap := newConfiguredSnapshot("test1", "InProgress")

	// Get a snapshot
	err := snapshotWithClient(snap, kubeClient, dynamicClient)
	if err != nil {
		t.Errorf("Error in snapshotWithClient : %s", err.Error())
	}
	if snap.Status.NumberOfContents != int32(len(ukubeobjects)) {
		t.Errorf(
			"Number of snapshot contents %d not equals to number of objects %d",
			snap.Status.NumberOfContents,
			len(ukubeobjects),
		)
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

	pref := newRestorePreference("pref1")
	restore := newConfiguredRestore("test1", "test2", "pref1", "InProgress")

	// Downlaod snapshot tgz
	err = downloadSnapshot(restore, bucket)
	if err != nil {
		t.Errorf("Error in downloadSnapshot : %s", err.Error())
	}
	if downloadFilename != "test2.tgz" {
		t.Error("Error download filename not match")
	}

	// Restore resources
	restore = newConfiguredRestore("test1", "test1", "pref1", "InProgress")
	err = restoreResources(restore, pref, kubeClient, dynamicClient)
	if err != nil {
		t.Errorf("Error in restoreResources : %s", err.Error())
	}
}

