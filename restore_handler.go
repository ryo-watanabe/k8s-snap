package main

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	ccv1alpha1 "gitlab.devops.nifcloud.net/x_nke/hatoba-backup-controller/pkg/apis/customercluster/v1alpha1"
	"gitlab.devops.nifcloud.net/x_nke/hatoba-backup-controller/pkg/arkresources"
)

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runRestoreWorker() {
	for c.processNextRestoreItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextRestoreItem() bool {
	// Proccess restore queue
	obj, shutdown := c.restoreQueue.Get()
	if shutdown {
		return false
	}
	err := func(obj interface{}) error {
		defer c.restoreQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.restoreQueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := c.restoreSyncHandler(key); err != nil {
			c.restoreQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.restoreQueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)
	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// backupSyncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Backup resource
// with the current status of the resource.
func (c *Controller) restoreSyncHandler(key string) error {

	getOptions := metav1.GetOptions{IncludeUninitialized: false}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Restore resource with this namespace/name.
	restore, err := c.restoreLister.Restores(namespace).Get(name)

	// if deleted.
	if err != nil {
		if errors.IsNotFound(err) {
			// Nothing to do?

			// if deleted ok, exit sync handler here.
			return nil
		} else {
			return err
		}
	}

	if restore.Status.Phase == "" {
		c.updateRestoreStatus(restore, "InProgress", "")
		time.Sleep(120 * time.Second)
		c.updateRestoreStatus(restore, "Completed", "")
	}

	c.recorder.Event(restore, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	return nil
}

func (c *Controller) updateRestoreStatus(restore *ccv1alpha1.Restore, phase, reason string) error {
	restoreCopy := restore.DeepCopy()
	restoreCopy.Status.Phase = phase
	restoreCopy.Status.Reason = reason
	_, err := c.ccclientset.CustomerclusterV1alpha1().Restores(restore.Namespace).Update(restoreCopy)
	if err != nil {
		runtime.HandleError(fmt.Errorf("Failed to update restore status for " + restore.ObjectMeta.Name))
	}
	return err
}

// enqueueRestore takes a Restore resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Restore.
func (c *Controller) enqueueRestore(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.restoreQueue.AddRateLimited(key)
}
