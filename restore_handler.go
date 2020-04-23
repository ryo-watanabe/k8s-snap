package main

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
)

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runRestoreWorker() {
	for c.processNextRestoreItem(false) {
	}
}
func (c *Controller) runRestoreQueuer() {
	for c.processNextRestoreItem(true) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextRestoreItem(queueonly bool) bool {
	// Process restore queue
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
		if err := c.restoreSyncHandler(key, queueonly); err != nil {
			c.restoreQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.restoreQueue.Forget(obj)
		klog.V(4).Infof("Successfully synced '%s'", key)
		return nil
	}(obj)
	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

// snapshotSyncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Snapshot resource
// with the current status of the resource.
func (c *Controller) restoreSyncHandler(key string, queueonly bool) error {

	//getOptions := metav1.GetOptions{IncludeUninitialized: false}

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
			// if deleted ok, exit sync handler here.
			return nil
		}
		return err
	}

	if !queueonly && restore.Status.Phase == "InQueue" {
		restore, err = c.updateRestoreStatus(restore, "InProgress", "")
		if err != nil {
			return err
		}

		// snapshot
		snapshot, err := c.cbclientset.ClustersnapshotV1alpha1().Snapshots(c.namespace).Get(
			restore.Spec.SnapshotName, metav1.GetOptions{})
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}
		if snapshot.Status.Phase != "Completed" {
			restore, err = c.updateRestoreStatus(restore, "Failed", "Snapshot data is not in status 'Completed'")
			if err != nil {
				return err
			}
			return nil
		}
		restore.Status.NumSnapshotContents = snapshot.Status.NumberOfContents

		// bucket
		osConfig, err := c.cbclientset.ClustersnapshotV1alpha1().ObjectstoreConfigs(c.namespace).Get(
			snapshot.Spec.ObjectstoreConfig, metav1.GetOptions{})
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}

		// cloud credentials secret
		cred, err := c.kubeclientset.CoreV1().Secrets(c.namespace).Get(
			osConfig.Spec.CloudCredentialSecret, metav1.GetOptions{})
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}
		bucket := objectstore.NewBucket(osConfig.ObjectMeta.Name, string(cred.Data["accesskey"]),
			string(cred.Data["secretkey"]), osConfig.Spec.Endpoint, osConfig.Spec.Region, osConfig.Spec.Bucket, c.insecure)

		// preference
		pref, err := c.cbclientset.ClustersnapshotV1alpha1().RestorePreferences(c.namespace).Get(
			restore.Spec.RestorePreferenceName, metav1.GetOptions{},
		)
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}

		// do restore
		err = c.clusterCmd.Restore(restore, pref, bucket)
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}

		restore, err = c.updateRestoreStatus(restore, "Completed", "")
		if err != nil {
			return err
		}
	}

	nowTime := metav1.NewTime(time.Now())

	if restore.Status.Phase == "" {
		// Chack AvailableUntil
		if restore.Spec.AvailableUntil.IsZero() {
			// Check TTL string
			if restore.Spec.TTL.Duration == 0 {
				restore.Spec.TTL.Duration = 24 * 7 * time.Hour
			}
		} else if restore.Spec.AvailableUntil.Before(&nowTime) {
			restore, err = c.updateRestoreStatus(restore, "Failed", "AvailableUntil is set as past.")
			if err != nil {
				return err
			}
			// When the restore failed, exit sync handler here.
			return nil
		}
		restore, err = c.updateRestoreStatus(restore, "InQueue", "")
		if err != nil {
			return err
		}
	}

	// expiration for failed restore
	if restore.Status.Phase == "Failed" && restore.Status.AvailableUntil.IsZero() {
		if !restore.Spec.AvailableUntil.IsZero() {
			restore.Status.AvailableUntil = restore.Spec.AvailableUntil
			restore.Status.TTL.Duration = restore.Status.AvailableUntil.Time.Sub(restore.ObjectMeta.CreationTimestamp.Time)
		} else {
			restore.Status.AvailableUntil = metav1.NewTime(restore.ObjectMeta.CreationTimestamp.Add(restore.Spec.TTL.Duration))
			restore.Status.TTL = restore.Spec.TTL
		}
		restore, err = c.updateRestoreStatus(restore, restore.Status.Phase, restore.Status.Reason)
		if err != nil {
			return err
		}
	}

	// expiration edited
	if restore.Status.Phase == "Completed" || restore.Status.Phase == "Failed" {
		if !restore.Spec.AvailableUntil.IsZero() && !restore.Spec.AvailableUntil.Equal(&restore.Status.AvailableUntil) {
			restore.Status.AvailableUntil = restore.Spec.AvailableUntil
			restore, err = c.updateRestoreStatus(restore, restore.Status.Phase, restore.Status.Reason)
			if err != nil {
				return err
			}
		}
	}

	// delete expired
	if !restore.Status.AvailableUntil.IsZero() && restore.Status.AvailableUntil.Before(&nowTime) {
		err := c.cbclientset.ClustersnapshotV1alpha1().Restores(c.namespace).Delete(name, &metav1.DeleteOptions{})
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
		}
		klog.Infof("restore:%s expired - deleted", name)
		// When the snapshot deleted, exit sync handler here.
		return nil
	}

	c.recorder.Event(restore, corev1.EventTypeNormal, "Synced", "Restore synced successfully")
	return nil
}

func (c *Controller) updateRestoreStatus(restore *cbv1alpha1.Restore, phase, reason string) (*cbv1alpha1.Restore, error) {
	restoreCopy := restore.DeepCopy()
	restoreCopy.Status.Phase = phase
	restoreCopy.Status.Reason = reason
	klog.Infof("restore:%s status %s => %s : %s", restore.ObjectMeta.Name, restore.Status.Phase, phase, reason)
	restore, err := c.cbclientset.ClustersnapshotV1alpha1().Restores(restore.Namespace).Update(restoreCopy)
	if err != nil {
		return nil, fmt.Errorf("Failed to update restore status for " + restore.ObjectMeta.Name + " : " + err.Error())
	}
	return restore, err
}

// enqueueRestore takes a Restore resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Restore.
func (c *Controller) enqueueRestore(obj interface{}) {
	var key string
	var err error

	// queue only restores in our namespace
	meta, err := meta.Accessor(obj)
	if err != nil {
		runtime.HandleError(fmt.Errorf("object has no meta: %v", err))
		return
	}
	if meta.GetNamespace() != c.namespace {
		return
	}

	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.restoreQueue.AddRateLimited(key)
}
