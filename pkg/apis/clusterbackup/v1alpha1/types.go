package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Foo is a specification for a Foo resource
type Backup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupSpec   `json:"spec"`
	Status BackupStatus `json:"status"`
}

// FooSpec is the spec for a Foo resource
type BackupSpec struct {
	ClusterName string `json:"clusterName"`
	Kubeconfig string `json:"kubeconfig"`
}

// FooStatus is the status for a Foo resource
type BackupStatus struct {
	Phase string `json:"phase"`
	Reason string `json:"reason"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Foo is a specification for a Foo resource
type Restore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreSpec   `json:"spec"`
	Status RestoreStatus `json:"status"`
}

// FooSpec is the spec for a Foo resource
type RestoreSpec struct {
	ClusterName string `json:"clusterName"`
	BackupName string `json:"backupName"`
	Kubeconfig string `json:"kubeconfig"`
}

// FooStatus is the status for a Foo resource
type RestoreStatus struct {
	Phase string `json:"phase"`
	Reason string `json:"reason"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FooList is a list of Foo resources
type BackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Backup `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FooList is a list of Foo resources
type RestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Restore `json:"items"`
}
