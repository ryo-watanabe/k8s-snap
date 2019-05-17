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
	ObjectstoreConfig string `json:"objectstoreConfig"`
	TTL metav1.Duration `json:"ttl"`
}

// FooStatus is the status for a Foo resource
type BackupStatus struct {
	Phase string `json:"phase"`
	Reason string `json:"reason"`
	BackupResourceVersion string `json:"backupResourceVersion"`
	BackupTimestamp metav1.Time `json:"backupTimestamp"`
	AvailableUntil metav1.Time `json:"availableUntil"`
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
	BackupName string `json:"backupName"`
	Kubeconfig string `json:"kubeconfig"`
	RestorePreferenceName string `json:"restorePreferenceName"`
	TTL metav1.Duration `json:"ttl"`
}

// FooStatus is the status for a Foo resource
type RestoreStatus struct {
	Phase string `json:"phase"`
	Reason string `json:"reason"`
	RestoreResourceVersion string `json:"restoreResourceVersion"`
	RestoreTimestamp metav1.Time `json:"restoreTimestamp"`
	PreserveUntil metav1.Time `json:"preserveUntil"`
	NumBackupContents int32 `json:"numBackupContents"`
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
