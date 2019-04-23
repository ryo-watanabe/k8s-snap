/*
Copyright The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by informer-gen. DO NOT EDIT.

package v1alpha1

import (
	time "time"

	clusterbackupv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	versioned "github.com/ryo-watanabe/k8s-backup/pkg/client/clientset/versioned"
	internalinterfaces "github.com/ryo-watanabe/k8s-backup/pkg/client/informers/externalversions/internalinterfaces"
	v1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/client/listers/clusterbackup/v1alpha1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
	watch "k8s.io/apimachinery/pkg/watch"
	cache "k8s.io/client-go/tools/cache"
)

// BackupInformer provides access to a shared informer and lister for
// Backups.
type BackupInformer interface {
	Informer() cache.SharedIndexInformer
	Lister() v1alpha1.BackupLister
}

type backupInformer struct {
	factory          internalinterfaces.SharedInformerFactory
	tweakListOptions internalinterfaces.TweakListOptionsFunc
	namespace        string
}

// NewBackupInformer constructs a new informer for Backup type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewBackupInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers) cache.SharedIndexInformer {
	return NewFilteredBackupInformer(client, namespace, resyncPeriod, indexers, nil)
}

// NewFilteredBackupInformer constructs a new informer for Backup type.
// Always prefer using an informer factory to get a shared informer instead of getting an independent
// one. This reduces memory footprint and number of connections to the server.
func NewFilteredBackupInformer(client versioned.Interface, namespace string, resyncPeriod time.Duration, indexers cache.Indexers, tweakListOptions internalinterfaces.TweakListOptionsFunc) cache.SharedIndexInformer {
	return cache.NewSharedIndexInformer(
		&cache.ListWatch{
			ListFunc: func(options v1.ListOptions) (runtime.Object, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ClusterbackupV1alpha1().Backups(namespace).List(options)
			},
			WatchFunc: func(options v1.ListOptions) (watch.Interface, error) {
				if tweakListOptions != nil {
					tweakListOptions(&options)
				}
				return client.ClusterbackupV1alpha1().Backups(namespace).Watch(options)
			},
		},
		&clusterbackupv1alpha1.Backup{},
		resyncPeriod,
		indexers,
	)
}

func (f *backupInformer) defaultInformer(client versioned.Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	return NewFilteredBackupInformer(client, f.namespace, resyncPeriod, cache.Indexers{cache.NamespaceIndex: cache.MetaNamespaceIndexFunc}, f.tweakListOptions)
}

func (f *backupInformer) Informer() cache.SharedIndexInformer {
	return f.factory.InformerFor(&clusterbackupv1alpha1.Backup{}, f.defaultInformer)
}

func (f *backupInformer) Lister() v1alpha1.BackupLister {
	return v1alpha1.NewBackupLister(f.Informer().GetIndexer())
}
