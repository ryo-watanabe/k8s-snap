package cluster

import (
	"fmt"
	"strconv"
	"sort"
	"time"
	"os"
	"path/filepath"
	"archive/tar"
	"compress/gzip"
	"encoding/json"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/discovery"
	"k8s.io/klog"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
	"github.com/ryo-watanabe/k8s-backup/pkg/objectstore"
	"github.com/ryo-watanabe/k8s-backup/pkg/utils"
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

func isOlderValidResourceVersion(rv, refrv string) bool {
	irv, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		return false
	}
	irefrv, err := strconv.ParseInt(refrv, 10, 64)
	if err != nil {
		return false
	}
	return (irv < irefrv)
}

func isNewerValidResourceVersion(rv, refrv string) bool {
	irv, err := strconv.ParseInt(rv, 10, 64)
	if err != nil {
		return false
	}
	irefrv, err := strconv.ParseInt(refrv, 10, 64)
	if err != nil {
		return false
	}
	return (irv > irefrv)
}

// Backup k8s resources
func Backup(backup *cbv1alpha1.Backup, bucket *objectstore.Bucket) error {

	// Backup log
	blog := utils.NewNamedLog("backup:" + backup.ObjectMeta.Name)

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

	blog.Info("Backing up resources")

	eventsWatch := make(map[schema.GroupVersionResource]watch.Interface)
	watchgvr := make(map[schema.GroupVersionResource]string)
	backupList := make([]unstructured.Unstructured, 0)
	watchEventList := make([]watch.Event, 0)

	// Generate marker name
	markerName := "resource-version-marker-" + utils.RandString(10)

	// Get start resource version
	marker, err := ConfigMapMarker(kubeClient, markerName)
	if err != nil {
		return err
	}
	startRV := marker.ObjectMeta.ResourceVersion

	for _, resourceGroup := range resources {
		gv, err := schema.ParseGroupVersion(resourceGroup.GroupVersion)
		if err != nil {
			return fmt.Errorf("unable to parse GroupVersion %s : %s", resourceGroup.GroupVersion, err.Error())
		}
		blog.Infof("- GroupVersion : %s", resourceGroup.GroupVersion)

		for _, resource := range resourceGroup.APIResources {
			// Get list of a resource
			gvr := gv.WithResource(resource.Name)
			unstructuredList, err := dynamicClient.Resource(gvr).List(metav1.ListOptions{})
			if err != nil {
				return err
			}

			// Start watching the resource
			watchgvr[gvr] = resourceGroup.GroupVersion + "/" + resource.Name
			eventsWatch[gvr], err = dynamicClient.Resource(gvr).Watch(metav1.ListOptions{ResourceVersion: startRV})
			if err != nil {
				return err
			}
			go func() {
				klog.V(4).Infof("+++ %s watch started", watchgvr[gvr])
				for e := range eventsWatch[gvr].ResultChan() {
					item, ok := e.Object.(*unstructured.Unstructured)
					if ok {
						switch e.Type {
						case watch.Added:
							klog.V(4).Infof("!!! Resource added : %s - rv:%s", item.GetSelfLink(), item.GetResourceVersion())
						case watch.Modified:
							klog.V(4).Infof("!!! Resource modified : %s - rv:%s", item.GetSelfLink(), item.GetResourceVersion())
						case watch.Deleted:
							klog.V(4).Infof("!!! Resource deleted : %s - rv:%s", item.GetSelfLink(), item.GetResourceVersion())
						}
					}
					watchEventList = append(watchEventList, e)
				}
				klog.V(4).Infof("+++ %s watch exiting", watchgvr[gvr])
			}()

			blog.Infof("-- %3d %s", len(unstructuredList.Items), resource.Name)

			// Join resource list
			backupList = append(backupList, unstructuredList.Items...)
		}
	}

	// Get end resource version
	marker, err = ConfigMapMarker(kubeClient, markerName)
	if err != nil {
		return err
	}
	endRV := marker.ObjectMeta.ResourceVersion
	blog.Infof("Start resource version : %s", startRV)
	blog.Infof("End resource version   : %s", endRV)

	// Stop watch resources
	for _, w := range eventsWatch {
		w.Stop()
	}

	// Sync resources
	blog.Info("Syncing modified resources:")
	for _, e := range watchEventList {
		item, ok := e.Object.(*unstructured.Unstructured)
		if ok {
			message := "unknown type"
			//if item.GetName() == markerName {
			//	message = "ignored (marker resource)"
			//	blog.Infof("-- [%s] rv:%s %s - %s", e.Type, item.GetResourceVersion(), item.GetSelfLink(), message)
			//	continue
			//}
			if !isOlderValidResourceVersion(item.GetResourceVersion(), endRV) {
				message = "ignored (newer than end-rv)"
				blog.Infof("-- [%s] rv:%s %s - %s", e.Type, item.GetResourceVersion(), item.GetSelfLink(), message)
				continue
			}
			targetIndex := -1
			for i, u := range backupList {
				if u.GetSelfLink() == item.GetSelfLink() {
					targetIndex = i
					break
				}
			}
			if targetIndex >= 0 {
				if isNewerValidResourceVersion(item.GetResourceVersion(), backupList[targetIndex].GetResourceVersion()) {
					switch e.Type {
					case watch.Added:
					case watch.Modified:
						backupList[targetIndex] = *item
						message = "applied"
					case watch.Deleted:
						backupList = append(backupList[:targetIndex], backupList[targetIndex+1:]...)
						message = "deleted"
					}
				} else {
					message = "ignored (older rv)"
				}
			} else {
				switch e.Type {
				case watch.Added:
				case watch.Modified:
					backupList = append(backupList, *item)
					message = "added"
				case watch.Deleted:
					message = "ignored (already deleted)"
				}
			}
			blog.Infof("-- [%s] rv:%s %s - %s", e.Type, item.GetResourceVersion(), item.GetSelfLink(), message)
		} else {
			blog.Infof("-- [%s] %#v", e.Type, e.Object)
		}
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

	// Write resources into json
	backup.Status.Contents = nil
	backup.Status.NumberOfContents = 0
	for _, item := range backupList {

		// Resources stored according to api path.
		itempath := item.GetSelfLink()
		// Namespaces and CRDs stored on top level.
		if item.GetKind() == "Namespace" {
			itempath = filepath.Join("/namespaces", item.GetName())
		}
		if item.GetKind() == "CustomResourceDefinition" {
			itempath = filepath.Join("/crds", item.GetName())
		}

		// backup item
		content, err := item.MarshalJSON()
		if err != nil {
			return err
		}
		hdr := &tar.Header{
			Name:     filepath.Join(backup.ObjectMeta.Name, itempath + ".json"),
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

		// Contents
		backup.Status.Contents = append(backup.Status.Contents, itempath)
		backup.Status.NumberOfContents += 1
	}

	blog.Info("Making backup.json")
	backup.Status.BackupTimestamp = marker.ObjectMeta.CreationTimestamp
	backup.Status.AvailableUntil = metav1.NewTime(marker.ObjectMeta.CreationTimestamp.Add(backup.Spec.TTL.Duration))
	backup.Status.BackupResourceVersion = endRV

	// Sort Contents
	sort.Strings(backup.Status.Contents)
	backupCopy := backup.DeepCopy()
	backupCopy.Status.Phase = ""
	backupCopy.ObjectMeta.SetResourceVersion("")
	backupCopy.ObjectMeta.SetUID("")

	// Store backup resource as backup.json
	backupResource, err := json.Marshal(backupCopy)
	if err != nil {
		return err
	}
	hdr := &tar.Header{
		Name:     filepath.Join(backup.ObjectMeta.Name, "backup.json"),
		Size:     int64(len(backupResource)),
		Typeflag: tar.TypeReg,
		Mode:     0755,
		ModTime:  time.Now(),
	}
	if err := tarWriter.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tarWriter.Write(backupResource); err != nil {
		return err
	}

	tarWriter.Close()
	tgz.Close()
	backupFile.Close()

	backupFile, err = os.Open("/tmp/" + backup.ObjectMeta.Name + ".tgz")
	defer backupFile.Close()
	if err != nil {
		return err
	}
	blog.Infof("Uploading file %s", backup.ObjectMeta.Name + ".tgz")
	err = bucket.Upload(backupFile, backup.ObjectMeta.Name + ".tgz")
	if err != nil {
		return err
	}

	objInfo, err := bucket.GetObjectInfo(backup.ObjectMeta.Name + ".tgz")
	if err != nil {
		return err
	}

	// Timestamps and size
	backup.Status.StoredTimestamp = metav1.NewTime(objInfo.Timestamp)
	backup.Status.StoredFileSize = objInfo.Size
	blog.Info("Upload completed")
	blog.Infof("-- resource version : %s", backup.Status.BackupResourceVersion)
	blog.Infof("-- backup timestamp : %s", backup.Status.BackupTimestamp)
	blog.Infof("-- available until  : %s", backup.Status.AvailableUntil)
	blog.Infof("-- num resources    : %d", backup.Status.NumberOfContents)
	blog.Infof("-- stored file size : %d", backup.Status.StoredFileSize)
	blog.Infof("-- stored timestamp : %s", backup.Status.StoredTimestamp)

	return nil
}
