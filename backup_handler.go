package main

import (
	"fmt"
	"time"
	"os"
	"path/filepath"
	"archive/tar"
	"compress/gzip"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	//"k8s.io/apimachinery/pkg/api/meta"
	//runtimeobj "k8s.io/apimachinery/pkg/runtime"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/cache"
	//"k8s.io/client-go/restmapper"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"github.com/ryo-watanabe/k8s-backup/pkg/cluster"
)

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runBackupWorker() {
	for c.processNextBackupItem(false) {
	}
}

func (c *Controller) runBackupQueuer() {
	for c.processNextBackupItem(true) {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextBackupItem(queueonly bool) bool {
	// Proccess backup queue
	obj, shutdown := c.backupQueue.Get()
	if shutdown {
		return false
	}
	err := func(obj interface{}) error {
		defer c.backupQueue.Done(obj)
		var key string
		var ok bool
		if key, ok = obj.(string); !ok {
			c.backupQueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		if err := c.backupSyncHandler(key, queueonly); err != nil {
			c.backupQueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		c.backupQueue.Forget(obj)
		klog.V(4).Infof("Successfully synced '%s'", key)
		return nil
	}(obj)
	if err != nil {
		runtime.HandleError(err)
		return true
	}

	return true
}

func matchVerbs(groupVersion string, r *metav1.APIResource) bool {
	return discovery.SupportsAllVerbs{Verbs: []string{"list", "create", "get", "delete"}}.Match(groupVersion, r)
}

func makeResourcePath(backupName, group, version, resourceName, namespace, name string) string {

	// Namespace resources stored on top level.
	if resourceName == "namespaces" {
		return filepath.Join(backupName, "namespaces", name)
	}

	// Other resources stored same as api path.
	nspath := ""
	if namespace != "" {
		nspath = "namespaces"
	}
	if group == "" {
		return filepath.Join(backupName, "api", version, nspath, namespace, resourceName, name)
	} else {
		return filepath.Join(backupName, "apis", group, version, nspath, namespace, resourceName, name)
	}
}

// Backup k8s resources
func (c *Controller) doBackup(backup *cbv1alpha1.Backup) error {
	// kubeClient for exxternal cluster.
	kubeClient, dynamicClient, err := cluster.BuildKubeClient(backup.Spec.Kubeconfig)
	if err != nil {
		return err
	}

	discoveryClient := kubeClient.Discovery()

	spr, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return err
	}
	resources := discovery.FilteredBy(discovery.ResourcePredicateFunc(matchVerbs), spr)

	// backup file
	backupFile, err := os.Create("/mnt/" + backup.ObjectMeta.Name + ".tgz")
	if err != nil {
		return err
	}
	tgz := gzip.NewWriter(backupFile)
	defer tgz.Close()

	tarWriter := tar.NewWriter(tgz)
	defer tarWriter.Close()

	klog.Info("Resources : ")
	resourcesMap := make(map[schema.GroupVersionResource]metav1.APIResource)

	for _, resourceGroup := range resources {
		gv, err := schema.ParseGroupVersion(resourceGroup.GroupVersion)
		if err != nil {
			return fmt.Errorf("unable to parse GroupVersion %s : %s", resourceGroup.GroupVersion, err.Error())
		}
		klog.Info("- GroupVersion : " + resourceGroup.GroupVersion)

		for _, resource := range resourceGroup.APIResources {
			gvr := gv.WithResource(resource.Name)
			resourcesMap[gvr] = resource
			klog.Info("-- " + resource.Name)
			unstructuredList, err := dynamicClient.Resource(gvr).List(metav1.ListOptions{})
			if err != nil {
				return err
			}
			for _, item := range unstructuredList.Items {

				// Resources stored according to api path.
				itempath := filepath.Join(backup.ObjectMeta.Name, item.GetSelfLink())
				// Namespaces and CRDs stored on top level.
				if resource.Name == "namespaces" {
					itempath = filepath.Join(backup.ObjectMeta.Name, "namespaces", item.GetName())
				}
				if resource.Name == "customresourcedefinitions" {
					itempath = filepath.Join(backup.ObjectMeta.Name, "crds", item.GetName())
				}

				// backup item
				content, err := item.MarshalJSON()
				if err != nil {
					return err
				}
				hdr := &tar.Header{
					Name:     itempath + ".json",
					Size:     int64(len(content)),
					Typeflag: tar.TypeReg,
					Mode:     0755,
					ModTime:  time.Now(),
				}
				if err := tarWriter.WriteHeader(hdr); err != nil {
					return err
				}
				if _, err := tarWriter.Write(content); err != nil {
					return err
				}
			}
		}
	}
	tarWriter.Close()
	tgz.Close()
	backupFile.Close()

	return nil
}

// backupSyncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Backup resource
// with the current status of the resource.
func (c *Controller) backupSyncHandler(key string, queueonly bool) error {

	//getOptions := metav1.GetOptions{IncludeUninitialized: false}

	// Convert the namespace/name string into a distinct namespace and name
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// Get the Backup resource with this namespace/name.
	backup, err := c.backupLister.Backups(namespace).Get(name)

	// if deleted.
	if err != nil {
		if errors.IsNotFound(err) {
			// TODO delete backhp data.

			// In deleting, exit sync handler here.
			return nil
		} else {
			return err
		}
	}

	if !queueonly && backup.Status.Phase == "InQueue" {
		backup, err = c.updateBackupStatus(backup, "InProgress", "")
		if err != nil {
			return err
		}

		// do backup
		err = c.doBackup(backup)
		if err != nil {
			backup, err = c.updateBackupStatus(backup, "Failed", err.Error())
			if err != nil {
				return err
			}
			return nil
		}

		backup, err = c.updateBackupStatus(backup, "Completed", "")
		if err != nil {
			return err
		}
	}

	if backup.Status.Phase == "" {
		backup, err = c.updateBackupStatus(backup, "InQueue", "")
		if err != nil {
			return err
		}
	}

	c.recorder.Event(backup, corev1.EventTypeNormal, "Synced", "Backup synced successfully")
	return nil
}

func (c *Controller) updateBackupStatus(backup *cbv1alpha1.Backup, phase, reason string) (*cbv1alpha1.Backup, error) {
	backupCopy := backup.DeepCopy()
	backupCopy.Status.Phase = phase
	backupCopy.Status.Reason = reason
	backup, err := c.cbclientset.ClusterbackupV1alpha1().Backups(backup.Namespace).Update(backupCopy)
	if err != nil {
		return backup, fmt.Errorf("Failed to update backup status for %s : %s", backup.ObjectMeta.Name, err.Error())
	}
	return backup, err
}

// enqueueBackup takes a Backup resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Backup.
func (c *Controller) enqueueBackup(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		runtime.HandleError(err)
		return
	}
	c.backupQueue.AddRateLimited(key)
}
