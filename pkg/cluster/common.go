package cluster

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
)

// Cluster interfaces for taking and restoring snapshot of k8s clusters
type Cluster interface {
	Snapshot(ctx context.Context, snapshot *cbv1alpha1.Snapshot) error
	UploadSnapshot(snapshot *cbv1alpha1.Snapshot, bucket objectstore.Objectstore) error
	Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket objectstore.Objectstore) error
}

// Cmd for execute cluster commands
type Cmd struct {
}

// NewClusterCmd returns new Cmd
func NewClusterCmd() *Cmd {
	return &Cmd{}
}

// Snapshot take a snapshot
func (c *Cmd) Snapshot(ctx context.Context, snapshot *cbv1alpha1.Snapshot) error {
	return Snapshot(ctx, snapshot)
}

// UploadSnapshot uploads the snapshot data to the object store bucket
func (c *Cmd) UploadSnapshot(snapshot *cbv1alpha1.Snapshot, bucket objectstore.Objectstore) error {
	return UploadSnapshot(snapshot, bucket)
}

// Restore restores snapshot data on a cluster
func (c *Cmd) Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket objectstore.Objectstore) error {
	return Restore(restore, pref, bucket)
}

// Setup Kubernetes client for target cluster.
func buildKubeClient(kubeconfig string) (*kubernetes.Clientset, error) {
	// Check if Kubeconfig available.
	if kubeconfig == "" {
		return nil, fmt.Errorf("Cannot create Kubeconfig : Kubeconfig not given")
	}

	// Setup Rancher Kubeconfig to access customer cluster.
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("Error building kubernetes clientset: %s", err.Error())
	}
	return kubeClient, err
}

// Setup Kubernetes dynamic client for target cluster.
func buildDynamicClient(kubeconfig string) (dynamic.Interface, error) {
	// Check if Kubeconfig available.
	if kubeconfig == "" {
		return nil, fmt.Errorf("Cannot create Kubeconfig : Kubeconfig not given")
	}

	// Setup Rancher Kubeconfig to access customer cluster.
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, fmt.Errorf("Error building kubeconfig: %s", err.Error())
	}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, fmt.Errorf("Error building dynamic client: %s", err.Error())
	}
	return dynamicClient, err
}

// ConfigMapMarker creates and deletes a config map to get a marker for Resource Version
func ConfigMapMarker(ctx context.Context, kubeClient kubernetes.Interface, name string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ConfigMap",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "default",
		},
	}
	configMap, err := kubeClient.CoreV1().ConfigMaps("default").Create(ctx, configMap, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	err = kubeClient.CoreV1().ConfigMaps("default").Delete(ctx, name, metav1.DeleteOptions{})
	if err != nil {
		return nil, err
	}
	return configMap, nil
}

func getUnstructuredMap(obj map[string]interface{}, name string) map[string]interface{} {
	item, ok := obj[name]
	if !ok {
		return nil
	}
	m, ok := item.(map[string]interface{})
	if !ok {
		return nil
	}
	return m
}

func getUnstructuredSlice(obj map[string]interface{}, name string) []interface{} {
	item, ok := obj[name]
	if !ok {
		return nil
	}
	s, ok := item.([]interface{})
	if !ok {
		return nil
	}
	return s
}

func getUnstructuredString(obj map[string]interface{}, name string) string {
	item, ok := obj[name]
	if !ok {
		return ""
	}
	s, ok := item.(string)
	if !ok {
		return ""
	}
	return s
}

