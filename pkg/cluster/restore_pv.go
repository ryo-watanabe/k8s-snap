package cluster

import (
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"

	cbv1alpha1 "github.com/ryo-watanabe/k8s-snap/pkg/apis/clustersnapshot/v1alpha1"
	"github.com/ryo-watanabe/k8s-snap/pkg/utils"
)

// Check PV status->phase
func isPVBound(ctx context.Context, pvName string, dyn dynamic.Interface, rlog *utils.NamedLog) (bool, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}
	pvItem, err := dyn.Resource(gvr).Get(ctx, pvName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	pvStatus := getUnstructuredMap(pvItem.Object, "status")
	if pvStatus == nil {
		return false, err
	}
	pvPhase := getUnstructuredString(pvStatus, "phase")
	rlog.Infof("     Checking PV:%s status:%s", pvName, pvPhase)
	if pvPhase == "Bound" {
		return true, nil
	}
	return false, nil
}

// Restore PV/PVC boundings one by one
func restorePV(ctx context.Context, dir string, dyn dynamic.Interface, p *preference,
	restore *cbv1alpha1.Restore, rlog *utils.NamedLog) error {

	pvcfiles, err := ioutil.ReadDir(filepath.Join(dir, "PVC"))
	if err != nil {
		return err
	}

	for _, f := range pvcfiles {

		var pvcItem unstructured.Unstructured
		var pvItem unstructured.Unstructured

		// Load PVC item
		resourcePath := strings.Replace(f.Name(), "|", "/", -1)
		err := loadItem(&pvcItem, filepath.Join(dir, "PVC", f.Name()))
		if err != nil {
			return err
		}

		rlog.Infof("---- %s", resourcePath)

		// Check storageClassName
		pvcSpec := getUnstructuredMap(pvcItem.Object, "spec")
		if pvcSpec == nil {
			excludeWithMsg(restore, rlog, resourcePath, "no-pvc-spec")
			continue
		}
		storageClassName := getUnstructuredString(pvcSpec, "storageClassName")
		if storageClassName == "" || !p.isIncludedStorageClass(storageClassName) {
			// Check Annotations
			annotaionStorageClassName := pvcItem.GetAnnotations()["volume.beta.kubernetes.io/storage-class"]
			if annotaionStorageClassName == "" || !p.isIncludedStorageClass(annotaionStorageClassName) {
				excludeWithMsg(restore, rlog, resourcePath, "no-storageclass")
				continue
			}
		}

		// Check bounded and PV name
		volumeName := getUnstructuredString(pvcSpec, "volumeName")
		if volumeName == "" {
			excludeWithMsg(restore, rlog, resourcePath, "not-bounded")
			continue
		}

		// Search the PV to bound in PV dir
		pvfiles, err := ioutil.ReadDir(filepath.Join(dir, "PV"))
		if err != nil {
			return err
		}
		pvFound := false
		pvResourcePath := ""
		for _, pvf := range pvfiles {
			if strings.Contains(pvf.Name(), "|persistentvolumes|"+volumeName+".json") {
				pvResourcePath = strings.Replace(pvf.Name(), "|", "/", -1)
				err := loadItem(&pvItem, filepath.Join(dir, "PV", pvf.Name()))
				if err != nil {
					return err
				}
				pvFound = true
				break
			}
		}
		if !pvFound {
			excludeWithMsg(restore, rlog, resourcePath, "pv-not-found")
			continue
		}

		// Restore PV first
		rlog.Infof("     Restoring PV %s", pvItem.GetName())
		pvSpec := getUnstructuredMap(pvItem.Object, "spec")
		if pvSpec == nil {
			excludeWithMsg(restore, rlog, pvResourcePath, "no-pv-spec")
			continue
		}
		pvSpec["claimRef"] = nil
		pvItem.Object["status"] = nil
		pvItem.SetResourceVersion("")
		pvItem.SetUID("")
		_, err = createItem(ctx, &pvItem, dyn, pvResourcePath)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				alreadyExist(restore, rlog, pvResourcePath)
			} else {
				failedWithMsg(restore, rlog, pvResourcePath, err.Error())
			}
			continue
		} else {
			created(restore, rlog, pvResourcePath)
		}

		// Then restore PVC
		rlog.Infof("     Restoring PVC %s", pvcItem.GetName())
		pvcSpec["volumeName"] = nil
		pvcItem.Object["status"] = nil
		pvcItem.SetResourceVersion("")
		pvcItem.SetUID("")
		annotations := pvcItem.GetAnnotations()
		delete(annotations, "pv.kubernetes.io/bind-completed")
		pvcItem.SetAnnotations(annotations)
		_, err = createItem(ctx, &pvcItem, dyn, resourcePath)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				alreadyExist(restore, rlog, resourcePath)
			} else {
				failedWithMsg(restore, rlog, resourcePath, err.Error())
			}
			continue
		} else {
			created(restore, rlog, resourcePath)
		}

		// Wait for bound
		count := 0
		timeout := 10
		for {
			if count >= timeout {
				return fmt.Errorf("Timeout : waiting for PV/PVC bound %s", pvItem.GetName())
			}
			bound, err := isPVBound(ctx, pvItem.GetName(), dyn, rlog)
			if err != nil {
				return err
			}
			if bound {
				rlog.Infof("     PV:%s - PVC:%s bounded successfully", pvItem.GetName(), pvcItem.GetName())
				break
			}
			time.Sleep(5 * time.Second)
			count = count + 1
		}
	}
	return nil
}
