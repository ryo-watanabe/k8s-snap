package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/runtime"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/objectstore"
	"github.com/ryo-watanabe/k8s-snap/pkg/utils"
)

func (c *Controller) runObjectSyncer() {

	err := c.syncObjects(c.housekeepstore, false, c.validatefileinfo)
	if err != nil {
		runtime.HandleError(err)
	}

}

func (c *Controller) getObjectList(ctx context.Context) ([]objectstore.ObjectInfo, error) {

	objectList := make([]objectstore.ObjectInfo, 0)

	osConfigs, err := c.cbclientset.ClustersnapshotV1alpha1().ObjectstoreConfigs(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("List Objectstore Config error : %s", err.Error())
	}

	for _, os := range osConfigs.Items {

		// Get bucket
		bucket, err := c.getBucket(ctx, c.namespace, os.ObjectMeta.Name, c.kubeclientset, c.cbclientset, c.insecure)
		if err != nil {
			fmt.Errorf("Get bucket error for ObjectstoreConfig %s * %s", os.ObjectMeta.Name, err.Error())
		}

		// Append objects list
		objList, err := bucket.ListObjectInfo()
		if err != nil {
			return nil, fmt.Errorf("List objects error : %s", err.Error())
		}
		objectList = append(objectList, objList...)
	}

	return objectList, nil
}

func (c *Controller) restoreSnapshotFromObject(ctx context.Context, object objectstore.ObjectInfo) error {

	// Download object
	snapshotFile, err := os.Create("/tmp/" + object.Name)
	defer snapshotFile.Close()
	bucket, err := c.getBucket(ctx, c.namespace, object.BucketConfigName, c.kubeclientset, c.cbclientset, c.insecure)
	if err != nil {
		return err
	}
	err = bucket.Download(snapshotFile, object.Name)
	if err != nil {
		return err
	}
	snapshotFile.Close()

	return c.restoreSnapshotFromObjectFile(ctx, object)
}

func (c *Controller) restoreSnapshotFromObjectFile(ctx context.Context, object objectstore.ObjectInfo) error {

	// Read tar.gz
	snapshotFile, err := os.Open("/tmp/" + object.Name)
	if err != nil {
		return err
	}
	tgz, err := gzip.NewReader(snapshotFile)
	if err != nil {
		return err
	}
	defer tgz.Close()

	// Search snapshot.json in tgz file
	tarReader := tar.NewReader(tgz)
	name := strings.TrimSuffix(object.Name, ".tgz")
	found := false
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg {
			path := strings.Replace(header.Name, name, "", 1)
			if path == "/snapshot.json" {
				found = true
				break
			}
		}
	}
	if !found {
		return fmt.Errorf("Cannot find snapshot.json file in %s", object.Name)
	}

	// Load item
	var item unstructured.Unstructured
	bytes, err := ioutil.ReadAll(tarReader)
	if err != nil {
		return err
	}
	err = item.UnmarshalJSON(bytes)
	if err != nil {
		ermsg := err.Error()
		if len(ermsg) > 300 {
			ermsg = ermsg[0:300] + "....."
		}
		// Add kind and apiVersion to older snapshots
		//if strings.Contains(ermsg, "Object 'Kind' is missing") {
		//	snapshotJSON := "{\"kind\":\"Snapshot\",\"apiVersion\":\"clustersnapshot.rywt.io/v1alpha1\","
		//	snapshotJSON += string(bytes)[1:]
		//	err = item.UnmarshalJSON([]byte(snapshotJSON))
		//	if err != nil {
		//		return err
		//	}
		//} else {
		//	return fmt.Errorf("UnmarshalJSON error : %s", ermsg)
		//}
		return fmt.Errorf("UnmarshalJSON error : %s", ermsg)
	}

	// Create item
	item.SetResourceVersion("")
	item.SetUID("")
	gvr := cbv1alpha1.SchemeGroupVersion.WithResource("snapshots")
	_, err = c.dynamic.Resource(gvr).Namespace(c.namespace).Create(ctx, &item, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("Create snapshot error : %s", err.Error())
	}

	// Set object file size and overwrite AvailableUntil when it has less than default TTY
	snapshot, err := c.snapshotLister.Snapshots(c.namespace).Get(name)
	if err != nil {
		return err
	}
	snapshot.Status.StoredFileSize = object.Size
	snapshot.Status.StoredTimestamp = metav1.NewTime(object.Timestamp)
	tmpAvailableUntil := metav1.NewTime(time.Now().Add(24 * 30 * time.Hour))
	if snapshot.Status.AvailableUntil.Before(&tmpAvailableUntil) {
		snapshot.Status.AvailableUntil = tmpAvailableUntil
	}

	// Update snapshot as 'Completed'
	_, err = c.updateSnapshotStatus(ctx, snapshot, "Completed", "")
	if err != nil {
		return err
	}

	return nil
}

func (c *Controller) syncObjects(deleteOrphanObjects, restoreOrphanedSnapshots, validateFileinfo bool) error {

	if !deleteOrphanObjects &&
		!restoreOrphanedSnapshots &&
		!validateFileinfo {
		// Do nothing
		return nil
	}

	// context for sync objects
	ctx := context.TODO()

	// sync objects log
	slog := utils.NewNamedLog("sync objects:")

	// Get object list
	objectList, err := c.getObjectList(ctx)
	if err != nil {
		return fmt.Errorf("List Object error : %s", err.Error())
	}

	// Get resource list
	snapshots, err := c.cbclientset.ClustersnapshotV1alpha1().Snapshots(c.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("List snapshots error : %s", err.Error())
	}

	// Compare to find orphan objects
	orphanObjects := make([]objectstore.ObjectInfo, 0)
	for _, object := range objectList {
		found := false
		for _, snap := range snapshots.Items {
			if snap.ObjectMeta.Name+".tgz" == object.Name {
				found = true
				break
			}
		}
		if !found {
			orphanObjects = append(orphanObjects, object)
			slog.Infof("Orphan object : %s %s %d", object.Name, object.Timestamp, object.Size)
		}
	}

	// Compare to find resources without object or invalid
	objectNotFoundSnaps := make([]cbv1alpha1.Snapshot, 0)
	objectInvalidSnaps := make([]cbv1alpha1.Snapshot, 0)
	validSnaps := make([]cbv1alpha1.Snapshot, 0)
	for _, snap := range snapshots.Items {
		if snap.Status.Phase != "Completed" &&
			snap.Status.Phase != "Failed" {
			continue
		}
		found := false
		valid := false
		for _, object := range objectList {
			if snap.ObjectMeta.Name+".tgz" == object.Name {
				found = true
				t := metav1.NewTime(object.Timestamp).Rfc3339Copy()
				if snap.Status.StoredTimestamp.Equal(&t) &&
					snap.Status.StoredFileSize == object.Size {

					// If the object found in other bucket, update the snapshot with correct config
					if snap.Spec.ObjectstoreConfig != object.BucketConfigName {
						snap.Spec.ObjectstoreConfig = object.BucketConfigName
						updatedSnap, err := c.updateSnapshotStatus(ctx, &snap, snap.Status.Phase, snap.Status.Reason)
						if err != nil {
							return err
						}
						validSnaps = append(validSnaps, *updatedSnap)
					} else {
						validSnaps = append(validSnaps, snap)
					}

					valid = true
				}
				break
			}
		}
		if !found {
			objectNotFoundSnaps = append(objectNotFoundSnaps, snap)
			slog.Infof("Object not found snap : %s %s %d", snap.ObjectMeta.Name, snap.Status.StoredTimestamp, snap.Status.StoredFileSize)
		} else if !valid {
			objectInvalidSnaps = append(objectInvalidSnaps, snap)
			slog.Infof("Object invalid snap   : %s %s %d", snap.ObjectMeta.Name, snap.Status.StoredTimestamp, snap.Status.StoredFileSize)
		}
	}

	// Delete orphan objects
	if deleteOrphanObjects {
		for _, object := range orphanObjects {
			slog.Infof("Deleting orphan object %s", object.Name)
			bucket, err := c.getBucket(ctx, c.namespace, object.BucketConfigName, c.kubeclientset, c.cbclientset, c.insecure)
			if err != nil {
				return err
			}
			err = bucket.Delete(object.Name)
			if err != nil {
				slog.Warningf("- Cannot delete object %s : %s", object.Name, err.Error())
			}
		}

		// Or restore orphaned snapshots
	} else if restoreOrphanedSnapshots {
		for _, object := range orphanObjects {
			slog.Infof("Restoring orphaned snapshot from %s", object.Name)
			err = c.restoreSnapshotFromObject(ctx, object)
			if err != nil {
				slog.Warningf("- Cannot restore snapshot from %s : %s", object.Name, err.Error())
			}
		}
	}

	// Validate (or do not validate) size and timestamp
	if validateFileinfo {
		for _, snap := range objectInvalidSnaps {
			if snap.Status.Phase != "Failed" {
				_, err = c.updateSnapshotStatus(ctx, &snap, "Failed", "Snapshot file size or timestamp not matched")
				if err != nil {
					return err
				}
			}
		}
	} else {
		for _, snap := range objectInvalidSnaps {
			if snap.Status.Phase != "Completed" {
				_, err = c.updateSnapshotStatus(ctx, &snap, "Completed", "")
				if err != nil {
					return err
				}
			}
		}
	}

	// Set 'Failed' for object not found
	if deleteOrphanObjects {
		for _, snap := range objectNotFoundSnaps {
			if snap.Status.Phase != "Failed" {
				_, err = c.updateSnapshotStatus(ctx, &snap, "Failed", "Snapshot file not found")
				if err != nil {
					return err
				}
			}
		}
	}

	// Set 'Completed' for valid snaps
	for _, snap := range validSnaps {
		if snap.Status.Phase != "Completed" {
			_, err = c.updateSnapshotStatus(ctx, &snap, "Completed", "")
			if err != nil {
				return err
			}
		}
	}

	return nil
}

