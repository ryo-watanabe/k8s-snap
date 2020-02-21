package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Snapshot is a specification for a Snapshot resource
type Snapshot struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   SnapshotSpec   `json:"spec"`
	Status SnapshotStatus `json:"status"`
}

// SnapshotSpec is the spec for a Snapshot resource
type SnapshotSpec struct {
	ClusterName       string          `json:"clusterName"`
	Kubeconfig        string          `json:"kubeconfig"`
	ObjectstoreConfig string          `json:"objectstoreConfig"`
	AvailableUntil    metav1.Time     `json:"availableUntil"`
	TTL               metav1.Duration `json:"ttl"`
}

// SnapshotStatus is the status for a Snapshot resource
type SnapshotStatus struct {
	Phase                   string          `json:"phase"`
	Reason                  string          `json:"reason"`
	SnapshotResourceVersion string          `json:"snapshotResourceVersion"`
	SnapshotTimestamp       metav1.Time     `json:"snapshotTimestamp"`
	AvailableUntil          metav1.Time     `json:"availableUntil"`
	TTL                     metav1.Duration `json:"ttl"`
	Contents                []string        `json:"contents"`
	StoredFileSize          int64           `json:"storedFileSize"`
	StoredTimestamp         metav1.Time     `json:"storedTimestamp"`
	NumberOfContents        int32           `json:"numberOfContents"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Restore is a specification for a Restore resource
type Restore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   RestoreSpec   `json:"spec"`
	Status RestoreStatus `json:"status"`
}

// RestoreSpec is the spec for a Restore resource
type RestoreSpec struct {
	ClusterName           string          `json:"clusterName"`
	SnapshotName          string          `json:"snapshotName"`
	Kubeconfig            string          `json:"kubeconfig"`
	RestorePreferenceName string          `json:"restorePreferenceName"`
	AvailableUntil        metav1.Time     `json:"availableUntil"`
	TTL                   metav1.Duration `json:"ttl"`
}

// RestoreStatus is the status for a Restore resource
type RestoreStatus struct {
	Phase                  string          `json:"phase"`
	Reason                 string          `json:"reason"`
	RestoreResourceVersion string          `json:"restoreResourceVersion"`
	RestoreTimestamp       metav1.Time     `json:"restoreTimestamp"`
	AvailableUntil         metav1.Time     `json:"availableUntil"`
	TTL                    metav1.Duration `json:"ttl"`
	NumSnapshotContents    int32           `json:"numSnapshotContents"`
	NumPreferenceExcluded  int32           `json:"numPreferenceExcluded"`
	Excluded               []string        `json:"excluded"`
	NumExcluded            int32           `json:"numExcluded"`
	Created                []string        `json:"created"`
	NumCreated             int32           `json:"numCreated"`
	Updated                []string        `json:"updated"`
	NumUpdated             int32           `json:"numUpdated"`
	AlreadyExisted         []string        `json:"alreadyExisted"`
	NumAlreadyExisted      int32           `json:"numAlreadyExisted"`
	Failed                 []string        `json:"failed"`
	NumFailed              int32           `json:"numFailed"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RestorePreference is a specification for a RestorePreference resource
type RestorePreference struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec RestorePreferenceSpec `json:"spec"`
}

// RestorePreferenceSpec is the spec for a RestorePreference resource
type RestorePreferenceSpec struct {
	ExcludeNamespaces        []string `json:"excludeNamespaces"`
	ExcludeCRDs              []string `json:"excludeCRDs"`
	ExcludeAPIPathes         []string `json:"excludeApiPathes"`
	RestoreAppAPIPathes      []string `json:"restoreAppApiPathes"`
	RestoreNfsStorageClasses []string `json:"restoreNfsStorageClasses"`
	RestoreOptions           []string `json:"restoreOptions"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ObjectstoreConfig is a specification for a ObjectstoreConfig resource
type ObjectstoreConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec ObjectstoreConfigSpec `json:"spec"`
}

// ObjectstoreConfigSpec is the spec for a ObjectstoreConfig resource
type ObjectstoreConfigSpec struct {
	Region                string `json:"region"`
	Endpoint              string `json:"endpoint"`
	CloudCredentialSecret string `json:"cloudCredentialSecret"`
	Bucket                string `json:"bucket"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// SnapshotList is a list of Snapshot resources
type SnapshotList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Snapshot `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RestoreList is a list of Restore resources
type RestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []Restore `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// RestorePreferenceList is a list of RestorePreference resources
type RestorePreferenceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []RestorePreference `json:"items"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// ObjectstoreConfigList is a list of ObjectstoreConfig resources
type ObjectstoreConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []ObjectstoreConfig `json:"items"`
}