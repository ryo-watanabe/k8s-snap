package cluster

import (
	"fmt"
	"time"
	"os"
	"path/filepath"
	"archive/tar"
	"compress/gzip"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"github.com/ryo-watanabe/k8s-backup/pkg/objectstore"
)

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
func Backup(backup *cbv1alpha1.Backup, bucket *objectstore.Bucket) error {

	// kubeClient for exxternal cluster.
	kubeClient, err := buildKubeClient(backup.Spec.Kubeconfig)
	if err != nil {
		return err
	}

	discoveryClient := kubeClient.Discovery()

	spr, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return err
	}
	resources := discovery.FilteredBy(discovery.ResourcePredicateFunc(matchVerbs), spr)

	// DynamicClient for exxternal cluster.
	dynamicClient, err := buildDynamicClient(backup.Spec.Kubeconfig)
	if err != nil {
		return err
	}

	// backup file
	backupFile, err := os.Create("/tmp/" + backup.ObjectMeta.Name + ".tgz")
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

	backupFile, err = os.Open("/tmp/" + backup.ObjectMeta.Name + ".tgz")
	defer backupFile.Close()
	if err != nil {
		return err
	}
	klog.Infof("Uploading file %s", backup.ObjectMeta.Name + ".tgz")
	err = bucket.Upload(backupFile, backup.ObjectMeta.Name + ".tgz")
	if err != nil {
		return err
	}

	return nil
}
