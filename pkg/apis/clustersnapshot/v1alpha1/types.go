package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Foo is a specification for a Foo resource
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SnapshotSpec   `json:"spec"`
	Status SnapshotStatus `json:"status"`
}

// FooSpec is the spec for a Foo resource
type SnapshotSpec struct {
	ClusterName string `json:"clusterName"`
	Kubeconfig string `json:"kubeconfig"`
	ObjectstoreConfig string `json:"objectstoreConfig"`
	AvailableUntil metav1.Time `json:"availableUntil"`
	TTL metav1.Duration `json:"ttl"`
}

// FooStatus is the status for a Foo resource
type SnapshotStatus struct {
	Phase string `json:"phase"`
	Reason string `json:"reason"`
	SnapshotResourceVersion string `json:"snapshotResourceVersion"`
	SnapshotTimestamp metav1.Time `json:"snapshotTimestamp"`
	AvailableUntil metav1.Time `json:"availableUntil"`
	TTL metav1.Duration `json:"ttl"`
	Contents []string `json:"contents"`
	StoredFileSize int64 `json:"storedFileSize"`
	StoredTimestamp metav1.Time `json:"storedTimestamp"`
	NumberOfContents int32 `json:"numberOfContents"`
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
	SnapshotName string `json:"snapshotName"`
	Kubeconfig string `json:"kubeconfig"`
	RestorePreferenceName string `json:"restorePreferenceName"`
	AvailableUntil metav1.Time `json:"availableUntil"`
	TTL metav1.Duration `json:"ttl"`
}

// FooStatus is the status for a Foo resource
type RestoreStatus struct {
	Phase string `json:"phase"`
	Reason string `json:"reason"`
	RestoreResourceVersion string `json:"restoreResourceVersion"`
	RestoreTimestamp metav1.Time `json:"restoreTimestamp"`
	AvailableUntil metav1.Time `json:"availableUntil"`
	TTL metav1.Duration `json:"ttl"`
	NumSnapshotContents int32 `json:"numSnapshotContents"`
	NumPreferenceExcluded int32 `json:"numPreferenceExcluded"`
	Excluded []string `json:"excluded"`
	NumExcluded int32 `json:"numExcluded"`
	Created []string `json:"created"`
	NumCreated int32 `json:"numCreated"`
	Updated []string `json:"updated"`
	NumUpdated int32 `json:"numUpdated"`
	AlreadyExisted []string `json:"alreadyExisted"`
	NumAlreadyExisted int32 `json:"numAlreadyExisted"`
	Failed []string `json:"failed"`
	NumFailed int32 `json:"numFailed"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Foo is a specification for a Foo resource
type RestorePreference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestorePreferenceSpec   `json:"spec"`
}

// FooSpec is the spec for a Foo resource
type RestorePreferenceSpec struct {
	ExcludeNamespaces []string `json:"excludeNamespaces"`
	ExcludeCRDs []string `json:"excludeCRDs"`
	ExcludeApiPathes []string `json:"excludeApiPathes"`
	RestoreAppApiPathes []string `json:"restoreAppApiPathes"`
	RestoreNfsStorageClasses []string `json:"restoreNfsStorageClasses"`
	RestoreOptions []string `json:"restoreOptions"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Foo is a specification for a Foo resource
type ObjectstoreConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObjectstoreConfigSpec   `json:"spec"`
}

// FooSpec is the spec for a Foo resource
type ObjectstoreConfigSpec struct {
	Region string `json:"region"`
	Endpoint string `json:"endpoint"`
	CloudCredentialSecret string `json:"cloudCredentialSecret"`
	Bucket string `json:"bucket"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FooList is a list of Foo resources
type SnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Snapshot `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FooList is a list of Foo resources
type RestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Restore `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FooList is a list of Foo resources
type RestorePreferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RestorePreference `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// FooList is a list of Foo resources
type ObjectstoreConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ObjectstoreConfig `json:"items"`
}
