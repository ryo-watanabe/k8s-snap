package main

import (
	"fmt"
	"os"
	"io"
	"io/ioutil"
	"archive/tar"
	"compress/gzip"
	"strings"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"github.com/ryo-watanabe/k8s-backup/pkg/cluster"
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

func apiPathMatched(path, apiPath string) bool {
	sp := strings.Split(apiPath, ",")
	if len(sp) != 2 {
		return false
	}
	if strings.HasPrefix(path, sp[0]) {
		if sp[1] == "*" || strings.Contains(path, sp[1]) {
			return true
		}
	}
	return false
}

func preferedToRestore(path string, pref *cbv1alpha1.RestorePreference) string {

	// namespace resources
	if strings.HasPrefix(path, "/namespaces/") {
		for _, n := range pref.Spec.ExcludeNamespaces {
			if strings.Contains(path, n) {
				return "Exclude"
			}
		}
		return "Namespace"
	}
	// crds
	if strings.HasPrefix(path, "/crds/") {
		for _, crd := range pref.Spec.ExcludeCRDs {
			if strings.Contains(path, crd) {
				return "Exclude"
			}
		}
		return "CRD"
	}
	// check exclude API pathes
	for _, p := range pref.Spec.ExcludeApiPathes {
		if apiPathMatched(path, p) {
			return "Exclude"
		}
	}
	// check exclude namespaces
	for _, n := range pref.Spec.ExcludeNamespaces {
		if strings.Contains(path, "namespaces/" + n) {
			return "Exclude"
		}
	}
	// check PV/PVC
	if strings.Contains(path, "/persistentvolumes/") {
		return "PV"
	}
	if strings.Contains(path, "/persistentvolumeclaims/") {
		return "PVC"
	}
	// check secrets
	if strings.Contains(path, "/secrets/") {
		return "Secret"
	}
	// check pods
	if strings.Contains(path, "/pods/") {
		return "Pod"
	}
	// check endpoints
	if strings.Contains(path, "/endpoints/") {
		return "Endpoint"
	}
	// check ClusterRoleBindings
	if strings.Contains(path, "/clusterrolebindings/") {
		return "ClusterRoleBinding"
	}
	// check Apps API pathes
	for _, p := range pref.Spec.RestoreAppApiPathes {
		if apiPathMatched(path, p) {
			return "RestoreApp"
		}
	}
	// other resources to restore
	return "Restore"
}

func loadItem(item *unstructured.Unstructured, filepath string) error {
	bytes, err := ioutil.ReadFile(filepath)
	if err != nil {
		return err
	}
	err = item.UnmarshalJSON(bytes)
	if err != nil {
		return err
	}
	return nil
}

// Get resoutrce string from GetSelfLink
func resourceFromSelfLink(selflink string) string {
	s := strings.Split(selflink, "/")
	if len(s) >= 2 {
		return s[len(s) - 2]
	}
	return ""
}

// Create resource
func createItem(item *unstructured.Unstructured, dyn dynamic.Interface) (*unstructured.Unstructured, error) {
	gv, err := schema.ParseGroupVersion(item.GetAPIVersion())
	if err != nil {
		return nil, err
	}
	gvr := gv.WithResource(resourceFromSelfLink(item.GetSelfLink()))
	ns := item.GetNamespace()
	if ns == "" {
		return dyn.Resource(gvr).Create(item, metav1.CreateOptions{})
	}
	return dyn.Resource(gvr).Namespace(ns).Create(item, metav1.CreateOptions{})
}

func isUserNamespace(nsName string, pref *cbv1alpha1.RestorePreference) bool {
	for _, n := range pref.Spec.ExcludeNamespaces {
		if nsName == n {
			return false
		}
	}
	return true
}

// Restore resources according to preferences.
func restoreDir(dir, restorePref string, dyn dynamic.Interface, pref *cbv1alpha1.RestorePreference) error {
	files, err := ioutil.ReadDir(filepath.Join(dir, restorePref))
	if err != nil {
		return err
	}
	for _, f := range files {
		klog.Infof("---- %s", f.Name())
		var item unstructured.Unstructured
		err := loadItem(&item, filepath.Join(dir, restorePref, f.Name()))
		if err != nil {
			return err
		}
		owners := item.GetOwnerReferences()
		if len(owners) > 0 {
			for _, owner := range owners {
				klog.Infof("     [owner]:%s %s", owner.Kind, owner.Name)
			}
			klog.Infof("@@@@@ Excluded : Owned by another resource")
			continue
		}
		if item.GetKind() == "Secret" {
			if item.Object["type"] == "kubernetes.io/service-account-token" {
				klog.Infof("@@@@@ Excluded : service account token secret")
				continue
			}
		}
		if item.GetKind() == "ClusterRoleBinding" {
			subjects, ok := item.Object["subjects"].([]interface{})
			if !ok {
				klog.Infof("@@@@@ Excluded : data not valid")
				continue
			}
			exclude := true
			for _, sub := range subjects {
				s := sub.(map[string]interface{})
				if s["kind"] == "ServiceAccount" {
					if isUserNamespace(s["namespace"].(string), pref) {
						exclude = false
					}
				}
			}
			if exclude {
				klog.Infof("@@@@@ Excluded : not binding to user namespaces")
				continue
			}
		}
		if item.GetKind() == "Endpoint" ||
		   item.GetKind() == "PersistentVolume" ||
		   item.GetKind() == "PersistentVolumeClaim" {
			klog.Infof("@@@@@ Excluded : currently by-passed")
			continue
		}
		item.SetResourceVersion("")
		item.SetUID("")
		_, err = createItem(&item, dyn)
		if err != nil {
			klog.Warningf("@@@@@ Cannot create item : %s", err.Error())
		} else {
			klog.Infof("@@@@@ Restored @@@@@")
		}
	}
	return nil
}

// create a file
func writeFile(filepath string, tarReader *tar.Reader) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := io.Copy(file, tarReader); err != nil {
		return err
	}
	return nil
}

func isIn(str string, finfo []os.FileInfo) bool {
	for _, f := range finfo {
		if str == f.Name() {
			return true
		}
	}
	return false
}

// Restore k8s resources
func (c *Controller) doRestore(restore *cbv1alpha1.Restore) error {
	// preference
	pref, err := c.cbclientset.ClusterbackupV1alpha1().RestorePreferences(c.namespace).Get(
		restore.Spec.RestorePreferenceName, metav1.GetOptions{},
	)
	if err != nil {
		return err
	}

	// Read tar.gz
	backupFile, err := os.Open("/mnt/" + restore.Spec.BackupName + ".tgz")
	if err != nil {
		return err
	}
	tgz, err := gzip.NewReader(backupFile)
	if err != nil {
		return err
	}
	defer tgz.Close()

	tarReader := tar.NewReader(tgz)

	dir, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}

	klog.Info("Extract files in backup tgz :")
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg {
			path := strings.Replace(header.Name, restore.Spec.BackupName, "", 1)
			restorePref := preferedToRestore(path, pref)

			if restorePref == "Exclude" {
				klog.Infof("-- [%s] %s", restorePref, path)
				continue
			}

			// create dir
			fullpath := filepath.Join(dir, restorePref, strings.Replace(path, "/", "|",100))
			err := os.MkdirAll(filepath.Dir(fullpath), header.FileInfo().Mode())
			if err != nil {
				return err
			}

			// create file
			err = writeFile(fullpath, tarReader)
			if err != nil {
				return err
			}
		}
	}

	// kubeClient for exxternal cluster.
	_, dynamicClient, err := cluster.BuildKubeClient(restore.Spec.Kubeconfig)
	if err != nil {
		return err
	}

	// Restore resources from json files
	dirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	// Restore namespaces and CRDs first
	if isIn("Namespace", dirs) {
		klog.Info("Restore Namespaces :")
		err = restoreDir(dir, "Namespace", dynamicClient, pref)
		if err != nil {
			return err
		}
	}
	if isIn("CRD", dirs) {
		klog.Info("Restore CRDs :")
		err = restoreDir(dir, "CRD", dynamicClient, pref)
		if err != nil {
			return err
		}
	}
	// Other resources
	klog.Info("Restore except Apps :")
	for _, d := range dirs {
		if d.Name() == "Namespace" ||
		   d.Name() == "CRD" ||
		   d.Name() == "RestoreApp" {
			continue
		}
		klog.Infof("-- %s", d.Name())
		err = restoreDir(dir, d.Name(), dynamicClient, pref)
		if err != nil {
			return err
		}
	}
	// Restore apps
	if isIn("RestoreApp", dirs) {
		klog.Info("Restore Apps :")
		err = restoreDir(dir, "RestoreApp", dynamicClient, pref)
		if err != nil {
			return err
		}
	}
	// Remove tmp files
	err = os.RemoveAll(dir)
	if err != nil {
		return err
	}

	return nil
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
			// Nothing to do?

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

		// do restore
		err = c.doRestore(restore)
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
		restore, err = c.updateRestoreStatus(restore, "InQueue", "")
		if err != nil {
			return err
		}
	}

	c.recorder.Event(restore, corev1.EventTypeNormal, "Synced", "Restore synced successfully")
	return nil
}

func (c *Controller) updateRestoreStatus(restore *cbv1alpha1.Restore, phase, reason string) (*cbv1alpha1.Restore, error) {
	restoreCopy := restore.DeepCopy()
	restoreCopy.Status.Phase = phase
	restoreCopy.Status.Reason = reason
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
