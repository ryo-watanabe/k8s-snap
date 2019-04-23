/*
Copyright 2017 The Kubernetes Authors.

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

package main

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/api/errors"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"

	clientset "github.com/ryo-watanabe/k8s-backup/pkg/client/clientset/versioned"
	ccscheme "github.com/ryo-watanabe/k8s-backup/pkg/client/clientset/versioned/scheme"
	informers "github.com/ryo-watanabe/k8s-backup/pkg/client/informers/externalversions/clusterbackup/v1alpha1"
	listers "github.com/ryo-watanabe/k8s-backup/pkg/client/listers/clusterbackup/v1alpha1"

	"github.com/ryo-watanabe/k8s-backup/pkg/objectstore"
)

const controllerAgentName = "k8s-backup"

const (
	ErrResourceExists = "ErrResourceExists"
	MessageResourceExists = "Resource %q already exists and is not managed by k8s-backup"
)


// Controller is the controller implementation for Backup and Restore resources
type Controller struct {
	kubeclientset kubernetes.Interface
	cbclientset clientset.Interface

	backupLister listers.BackupLister
	backupsSynced cache.InformerSynced
	restoreLister listers.RestoreLister
	restoresSynced cache.InformerSynced

	backupQueue workqueue.RateLimitingInterface
	restoreQueue workqueue.RateLimitingInterface
	recorder record.EventRecorder

	namespace string
	labels map[string]string
	bucket *objectstore.Bucket
}

// NewController returns a new controller
func NewController(
	kubeclientset kubernetes.Interface,
	cbclientset clientset.Interface,
	backupInformer informers.BackupInformer,
	restoreInformer informers.RestoreInformer,
	namespace string,
	bucket *objectstore.Bucket) *Controller {

	// Create event broadcaster
	// Add k8s-backup-controller types to the default Kubernetes Scheme so Events can be
	// logged for k8s-backup-controller types.
	utilruntime.Must(ccscheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartLogging(klog.V(4).Infof)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		cbclientset:       cbclientset,
		backupLister:      backupInformer.Lister(),
		backupsSynced:     backupInformer.Informer().HasSynced,
		restoreLister:     restoreInformer.Lister(),
		restoresSynced:    restoreInformer.Informer().HasSynced,
		backupQueue:       workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Backups"),
		restoreQueue:      workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Restores"),
		recorder:          recorder,
		namespace:         namespace,
		bucket:            bucket,
		labels:  map[string]string{
			"app":        "k8s-backup",
			"controller": "k8s-backup-controller",
		},
	}

	klog.Info("Setting up event handlers")

	// Set up an event handler for when Backup resources change
	backupInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueBackup,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueBackup(new)
		},
		DeleteFunc: controller.enqueueBackup,
	})

	// Set up an event handler for when Restore resources change
	restoreInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueRestore,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueRestore(new)
		},
		DeleteFunc: controller.enqueueRestore,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(backupthreads, restorethreads int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.backupQueue.ShutDown()
	defer c.restoreQueue.ShutDown()

	//listOptions := metav1.ListOptions{IncludeUninitialized: false}
	//getOptions := metav1.GetOptions{IncludeUninitialized: false}

	klog.Info("Checking objectstore bucket")
	found, err := c.bucket.ChkBucket()
	if err != nil {
		klog.Fatalf("Check bucket error : %s", err.Error())
	}
	if !found {
		klog.Fatalf("Bucket %s not found", c.bucket.BucketName)
	}

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting backup controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.backupsSynced, c.restoresSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")

	// Launch two workers to process Proxy resources
	for i := 0; i < backupthreads; i++ {
		go wait.Until(c.runBackupWorker, time.Second, stopCh)
	}
	go wait.Until(c.runBackupQueuer, time.Second, stopCh)
	for i := 0; i < restorethreads; i++ {
		go wait.Until(c.runRestoreWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}
