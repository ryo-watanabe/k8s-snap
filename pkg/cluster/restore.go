package cluster

import (
	"os"
	"io"
	"io/ioutil"
	"archive/tar"
	"compress/gzip"
	"strings"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"github.com/ryo-watanabe/k8s-backup/pkg/objectstore"
)

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

// Restore resources according to preferences.
func restoreDir(dir, restorePref string, dyn dynamic.Interface, p *preference) error {

	files, err := ioutil.ReadDir(filepath.Join(dir, restorePref))
	if err != nil {
		return err
	}
	for _, f := range files {

		klog.Infof("---- %s", f.Name())

		// Load item
		var item unstructured.Unstructured
		err := loadItem(&item, filepath.Join(dir, restorePref, f.Name()))
		if err != nil {
			return err
		}

		// Check owner
		owners := item.GetOwnerReferences()
		if len(owners) > 0 {
			klog.Infof("@@@@@ Excluded : Owned by another resource")
			for _, owner := range owners {
				klog.Infof("  owner : %s %s", owner.Kind, owner.Name)
			}
			p.cntUpExcluded()
			continue
		}

		// Operation for each resources
		switch item.GetKind() {
		case "Secret":
			if item.Object["type"] == "kubernetes.io/service-account-token" {
				klog.Infof("@@@@@ Excluded : service account token secret")
				p.cntUpExcluded()
				continue
			}
		case "ClusterRole":
			if !isInList(item.GetName(), p.includedClusterRoles) {
				klog.Infof("@@@@@ Excluded : not binding to user namespaces")
				p.cntUpExcluded()
				continue
			}
		case "ClusterRoleBinding":
			if !isInList(item.GetName(), p.includedClusterRoleBindings) {
				klog.Infof("@@@@@ Excluded : not binding to user namespaces")
				p.cntUpExcluded()
				continue
			}
		case "PersistentVolume":
		case "PersistentVolumeClaim":
			klog.Warningf("@@@@@ Warning : Excluded : PVs/PVCs must not be included here")
			continue
		case "Endpoints":
			if isInList(item.GetNamespace() + "/" + item.GetName(), p.serviceList) {
				klog.Infof("@@@@@ Excluded : a same name service exists")
				p.cntUpExcluded()
				continue
			}
		}

		// Restore item
		item.SetResourceVersion("")
		item.SetUID("")
		_, err = createItem(&item, dyn)
		if err != nil {
			klog.Warningf("@@@@@ Cannot create item : %s", err.Error())
			p.cntUpCnnotRestore(err.Error())
		} else {
			klog.Infof("@@@@@ Restored @@@@@")
			p.cntUpRestored()
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

// Restore k8s resources
func Restore(restore *cbv1alpha1.Restore, pref *cbv1alpha1.RestorePreference, bucket *objectstore.Bucket) error {

	p := newPreference(pref)

	// Download
	klog.Infof("Downloading file %s", restore.Spec.BackupName + ".tgz")
	backupFile, err := os.Create("/tmp/" + restore.Spec.BackupName + ".tgz")
	defer backupFile.Close()
	err = bucket.Download(backupFile, restore.Spec.BackupName + ".tgz")
	if err != nil {
		return err
	}
	backupFile.Close()

	// Read tar.gz
	backupFile, err = os.Open("/tmp/" + restore.Spec.BackupName + ".tgz")
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

			if path == "/backup.json" {
				klog.Infof("-- [Backup resource file] %s", path)
				continue
			}

			restorePref := p.preferedToRestore(path)
			if restorePref == "Exclude" {
				klog.Infof("-- [%s] %s", restorePref, path)
				p.cntUpExcluded()
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

	// DynamicClient for exxternal cluster.
	dynamicClient, err := buildDynamicClient(restore.Spec.Kubeconfig)
	if err != nil {
		return err
	}

	// Initialize preference
	err = p.initializeByDir(dir)
	if err != nil {
		return err
	}

	// Restore namespaces
	if p.isIn("Namespace") {
		klog.Info("Restore Namespaces :")
		err = restoreDir(dir, "Namespace", dynamicClient, p)
		if err != nil {
			return err
		}
	}
	// Restore CRDs
	if p.isIn("CRD") {
		klog.Info("Restore CRDs :")
		err = restoreDir(dir, "CRD", dynamicClient, p)
		if err != nil {
			return err
		}
	}
	// Restore PV/PVC
	if p.isIn("PV") && p.isIn("PVC") {
		klog.Info("Restore PV/PVC :")
		err = restorePV(dir, dynamicClient, p)
		if err != nil {
			return err
		}
	}
	// Other resources
	if p.isIn("Restore") {
		klog.Info("Restore resources except Apps :")
		err = restoreDir(dir, "Restore", dynamicClient, p)
		if err != nil {
			return err
		}
	}
	// Restore apps
	if p.isIn("App") {
		klog.Info("Restore Apps :")
		err = restoreDir(dir, "App", dynamicClient, p)
		if err != nil {
			return err
		}
	}
	// Remove tmp files
	err = os.RemoveAll(dir)
	if err != nil {
		return err
	}

	// result
	klog.Info("Restore completed ======")
	klog.Infof("Excluded       : %d", p.cntExcluded)
	klog.Infof("Restored       : %d", p.cntRestored)
	klog.Infof("Already exists : %d", p.cntAlreadyExists)
	klog.Infof("Other errors   : %d", p.cntOtherErrors)

	return nil
}
