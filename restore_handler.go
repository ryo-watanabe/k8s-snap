package main

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"github.com/ryo-watanabe/k8s-backup/pkg/cluster"
	"github.com/ryo-watanabe/k8s-backup/pkg/objectstore"
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


// backupSyncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Backup resource
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
		} else {
			return err
		}
	}

	if !queueonly && restore.Status.Phase == "InQueue" {
		restore, err = c.updateRestoreStatus(restore, "InProgress", "")
		if err != nil {
			return err
		}

		// backup
		backup, err := c.cbclientset.ClusterbackupV1alpha1().Backups(c.namespace).Get(
			restore.Spec.BackupName, metav1.GetOptions{})
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}
		if backup.Status.Phase != "Completed" {
			restore, err = c.updateRestoreStatus(restore, "Failed", "Backup data is not in status 'Completed'")
			if err != nil {
				return err
			}
			return nil
		}
		restore.Status.NumBackupContents = backup.Status.NumberOfContents

		// bucket
		osConfig, err := c.cbclientset.ClusterbackupV1alpha1().ObjectstoreConfigs(c.namespace).Get(
			backup.Spec.ObjectstoreConfig, metav1.GetOptions{})
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
			string(cred.Data["secretkey"]), osConfig.Spec.Endpoint, osConfig.Spec.Region, osConfig.Spec.Bucket)

		// preference
		pref, err := c.cbclientset.ClusterbackupV1alpha1().RestorePreferences(c.namespace).Get(
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
		err = cluster.Restore(restore, pref, bucket)
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

	if restore.Status.Phase == "" {
		// Check TTL string
		if restore.Spec.TTL.Duration == 0 {
			restore.Spec.TTL.Duration = 24*7*time.Hour
		}
		restore, err = c.updateRestoreStatus(restore, "InQueue", "")
		if err != nil {
			return err
		}
	}

	// delete expired
	if !restore.Status.PreserveUntil.IsZero() && restore.Status.PreserveUntil.Before(&metav1.Time{time.Now()}) {
		err := c.cbclientset.ClusterbackupV1alpha1().Restores(c.namespace).Delete(name, &metav1.DeleteOptions{})
		if err != nil {
			restore, err = c.updateRestoreStatus(restore, "Failed", err.Error())
			if err != nil {
				return err
			}
		}
		klog.Infof("restore:%s expired - deleted", name)
		// When the backup deleted, exit sync handler here.
		return nil
	}

	c.recorder.Event(restore, corev1.EventTypeNormal, "Synced", "Restore synced successfully")
	return nil
}

func (c *Controller) updateRestoreStatus(restore *cbv1alpha1.Restore, phase, reason string) (*cbv1alpha1.Restore, error) {
	restoreCopy := restore.DeepCopy()
	restoreCopy.Status.Phase = phase
	restoreCopy.Status.Reason = reason
	klog.Infof("Restore:%s status %s => %s : %s", restore.ObjectMeta.Name, restore.Status.Phase, phase, reason)
	restore, err := c.cbclientset.ClusterbackupV1alpha1().Restores(restore.Namespace).Update(restoreCopy)
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
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.restoreQueue.AddRateLimited(key)
}
