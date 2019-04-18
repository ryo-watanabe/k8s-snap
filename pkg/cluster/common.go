package cluster

import (
	"fmt"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
	//"k8s.io/klog"
)

// Setup Kubernetes client for target cluster.
func buildKubeClient(kubeconfig string) (*kubernetes.Clientset, error) {
	// Check if Kubeconfig available.
	if kubeconfig == "" {
		return nil, fmt.Errorf("Cannot create Kubeconfig : Kubeconfig not given.")
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
		return nil, fmt.Errorf("Cannot create Kubeconfig : Kubeconfig not given.")
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
