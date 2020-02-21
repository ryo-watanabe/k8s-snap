package cluster

import (
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
func isPVBound(pvName string, dyn dynamic.Interface, rlog *utils.NamedLog) (bool, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}
	pv_item, err := dyn.Resource(gvr).Get(pvName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	pv_status := getUnstructuredMap(pv_item.Object, "status")
	if pv_status == nil {
		return false, err
	}
	pv_phase := getUnstructuredString(pv_status, "phase")
	rlog.Infof("     Checking PV:%s status:%s", pvName, pv_phase)
	if pv_phase == "Bound" {
		return true, nil
	}
	return false, nil
}

// Restore PV/PVC boundings one by one
func restorePV(dir string, dyn dynamic.Interface, p *preference,
	restore *cbv1alpha1.Restore, rlog *utils.NamedLog) error {

	pvcfiles, err := ioutil.ReadDir(filepath.Join(dir, "PVC"))
	if err != nil {
		return err
	}

	for _, f := range pvcfiles {

		var pvc_item unstructured.Unstructured
		var pv_item unstructured.Unstructured

		// Load PVC item
		err := loadItem(&pvc_item, filepath.Join(dir, "PVC", f.Name()))
		if err != nil {
			return err
		}

		rlog.Infof("---- %s", pvc_item.GetSelfLink())

		// Check storageClassName
		pvc_spec := getUnstructuredMap(pvc_item.Object, "spec")
		if pvc_spec == nil {
			excludeWithMsg(restore, rlog, pvc_item.GetSelfLink(), "no-pvc-spec")
			continue
		}
		storageClassName := getUnstructuredString(pvc_spec, "storageClassName")
		if storageClassName == "" || !p.isIncludedStorageClass(storageClassName) {
			// Check Annotations
			annotaionStorageClassName := pvc_item.GetAnnotations()["volume.beta.kubernetes.io/storage-class"]
			if annotaionStorageClassName == "" || !p.isIncludedStorageClass(annotaionStorageClassName) {
				excludeWithMsg(restore, rlog, pvc_item.GetSelfLink(), "no-storageclass")
				continue
			}
		}

		// Check bounded and PV name
		volumeName := getUnstructuredString(pvc_spec, "volumeName")
		if volumeName == "" {
			excludeWithMsg(restore, rlog, pvc_item.GetSelfLink(), "not-bounded")
			continue
		}

		// Search the PV to bound in PV dir
		pvfiles, err := ioutil.ReadDir(filepath.Join(dir, "PV"))
		if err != nil {
			return err
		}
		pv_found := false
		for _, pvf := range pvfiles {
			if strings.Contains(pvf.Name(), "|persistentvolumes|"+volumeName+".json") {
				err := loadItem(&pv_item, filepath.Join(dir, "PV", pvf.Name()))
				if err != nil {
					return err
				}
				pv_found = true
				break
			}
		}
		if !pv_found {
			excludeWithMsg(restore, rlog, pvc_item.GetSelfLink(), "pv-not-found")
			continue
		}

		// Restore PV first
		rlog.Infof("     Restoring PV %s", pv_item.GetName())
		pv_spec := getUnstructuredMap(pv_item.Object, "spec")
		if pv_spec == nil {
			excludeWithMsg(restore, rlog, pvc_item.GetSelfLink(), "no-pv-spec")
			continue
		}
		pv_spec["claimRef"] = nil
		pv_item.Object["status"] = nil
		pv_item.SetResourceVersion("")
		pv_item.SetUID("")
		_, err = createItem(&pv_item, dyn)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				alreadyExist(restore, rlog, pv_item.GetSelfLink())
			} else {
				failedWithMsg(restore, rlog, pv_item.GetSelfLink(), err.Error())
			}
			continue
		} else {
			created(restore, rlog, pv_item.GetSelfLink())
		}

		// Then restore PVC
		rlog.Infof("     Restoring PVC %s", pvc_item.GetName())
		pvc_spec["volumeName"] = nil
		pvc_item.Object["status"] = nil
		pvc_item.SetResourceVersion("")
		pvc_item.SetUID("")
		annotations := pvc_item.GetAnnotations()
		delete(annotations, "pv.kubernetes.io/bind-completed")
		pvc_item.SetAnnotations(annotations)
		_, err = createItem(&pvc_item, dyn)
		if err != nil {
			if strings.Contains(err.Error(), "already exists") {
				alreadyExist(restore, rlog, pvc_item.GetSelfLink())
			} else {
				failedWithMsg(restore, rlog, pvc_item.GetSelfLink(), err.Error())
			}
			continue
		} else {
			created(restore, rlog, pvc_item.GetSelfLink())
		}

		// Wait for bound
		count := 0
		timeout := 10
		for {
			if count >= timeout {
				return fmt.Errorf("Timeout : waiting for PV/PVC bound %s", pv_item.GetName())
			}
			bound, err := isPVBound(pv_item.GetName(), dyn, rlog)
			if err != nil {
				return err
			}
			if bound {
				rlog.Infof("     PV:%s - PVC:%s bounded successfully", pv_item.GetName(), pvc_item.GetName())
				break
			}
			time.Sleep(5 * time.Second)
			count = count + 1
		}
	}
	return nil
}