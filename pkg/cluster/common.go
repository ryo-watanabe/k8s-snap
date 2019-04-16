package cluster

import (
	"fmt"
	//"strings"
	//"encoding/base64"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//corev1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/api/errors"
	//cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	//"k8s.io/klog"
)

// Setup Kubernetes client for target cluster.
func BuildKubeClient(kubeconfig string) (*kubernetes.Clientset, dynamic.Interface, error) {
	// Check if Kubeconfig available.
	if kubeconfig == "" {
		return nil, nil, fmt.Errorf("Cannot create Kubeconfig : Kubeconfig not given.")
	}

	// Setup Rancher Kubeconfig to access customer cluster.
	cfg, err := clientcmd.RESTConfigFromKubeConfig([]byte(kubeconfig))
	if err != nil {
		return nil, nil, fmt.Errorf("Error building kubeconfig: %s", err.Error())
	}
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("Error building kubernetes clientset: %s", err.Error())
	}
	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("Error building dynamic client: %s", err.Error())
	}
	return kubeClient, dynamicClient, err
}
