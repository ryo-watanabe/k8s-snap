package cluster

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	discoveryfake "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"

	clustersnapshot "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
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
	}

	sch := runtime.NewScheme()
	sch.AddKnownTypeWithName(schema.GroupVersionKind{Version:"v1", Kind:"Secret"}, secret)
	mapsecret, _ := runtime.DefaultUnstructuredConverter.ToUnstructured(secret)
	usecret := &unstructured.Unstructured{Object: mapsecret}
	ukubeobjects = append(ukubeobjects, usecret.DeepCopyObject())
	dynamicClient := dynamicfake.NewSimpleDynamicClient(sch, ukubeobjects...)

	snap := newConfiguredSnapshot("test1", "InProgress")

	err := snapshotWithClient(snap, kubeClient, dynamicClient)
	if err != nil {
		t.Errorf("Error in snapshot : %s", err.Error())
	}
}
