package cluster

import (
	"fmt"
	"time"
	"io/ioutil"
	"strings"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/klog"
)

// Check PV status->phase
func isPVBound(pvName string, dyn dynamic.Interface) (bool, error) {
	gvr := schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}
	pv_item, err := dyn.Resource(gvr).Get(pvName, metav1.GetOptions{})
	if err != nil {
		return false, err
	}
	pv_status := pv_item.Object["status"].(map[string]interface{})
	klog.Infof("     Checking PV:%s status:%s", pvName, pv_status["phase"].(string))
	if pv_status["phase"] == "Bound" {
		return true, nil
	}
	return false, nil
}

// Restore PV/PVC boundings one by one
func restorePV(dir string, dyn dynamic.Interface, p *preference) error {
	pvcfiles, err := ioutil.ReadDir(filepath.Join(dir, "PVC"))
	if err != nil {
		return err
	}
	for _, f := range pvcfiles {
		klog.Infof("---- %s", f.Name())

		var pvc_item unstructured.Unstructured
		var pv_item unstructured.Unstructured

		// Load PVC item
		err := loadItem(&pvc_item, filepath.Join(dir, "PVC", f.Name()))
		if err != nil {
			return err
		}

		// Check storageClassName
		pvc_spec := pvc_item.Object["spec"].(map[string]interface{})
		storageClassName := pvc_spec["storageClassName"].(string)
		if !p.isIncludedStorageClass(storageClassName) {
			klog.Infof("@@@@ storageclass %s not included", storageClassName)
			p.cntUpExcluded()
			continue
		}

		// Check bounded and PV name
		volumeName := pvc_spec["volumeName"].(string)
		if volumeName == "" {
			klog.Infof("@@@@ PV not bounded")
			p.cntUpExcluded()
			continue
		}

		// Search the PV to bound in PV dir
		pvfiles, err := ioutil.ReadDir(filepath.Join(dir, "PV"))
		if err != nil {
			return err
		}
		pv_found := false
		for _, pvf := range pvfiles {
			if strings.Contains(pvf.Name(), "|persistentvolumes|" + volumeName + ".json") {
				err := loadItem(&pv_item, filepath.Join(dir, "PV", pvf.Name()))
				if err != nil {
					return err
				}
				pv_found = true
				break
			}
		}
		if !pv_found {
			klog.Infof("@@@@ PV not found")
			p.cntUpExcluded()
			continue
		}

		// Restore PV first
		klog.Infof("    Restoring PV %s", pv_item.GetName())
		pv_spec := pv_item.Object["spec"].(map[string]interface{})
		pv_spec["claimRef"] = nil
		pv_item.Object["status"] = nil
		pv_item.SetResourceVersion("")
		pv_item.SetUID("")
		_, err = createItem(&pv_item, dyn)
		if err != nil {
			klog.Warningf("@@@@@ Cannot create item : %s", err.Error())
			p.cntUpCnnotRestore(err.Error())
			continue
		} else {
			klog.Infof("     @@@@@ Restored @@@@@")
			p.cntUpRestored()
		}

		// Then restore PVC
		klog.Infof("    Restoring PVC %s", pvc_item.GetName())
		pvc_spec["volumeName"] = nil
		pvc_item.Object["status"] = nil
		pvc_item.SetResourceVersion("")
		pvc_item.SetUID("")
		annotations := pvc_item.GetAnnotations()
		delete(annotations, "pv.kubernetes.io/bind-completed")
		pvc_item.SetAnnotations(annotations)
		_, err = createItem(&pvc_item, dyn)
		if err != nil {
			klog.Warningf("@@@@@ Cannot create item : %s", err.Error())
			p.cntUpCnnotRestore(err.Error())
			continue
		} else {
			klog.Infof("     @@@@@ Restored @@@@@")
			p.cntUpRestored()
		}

		// Wait for bound
		count := 0
		timeout := 10
		for {
			if count >= timeout {
				return fmt.Errorf("Timeout : waiting for PV/PVC bound %s", pv_item.GetName())
			}
			bound, err := isPVBound(pv_item.GetName(), dyn)
			if err != nil {
				return err
			}
			if bound {
				klog.Infof("     @@@@@ PV:%s - PVC:%s bounded @@@@@", pv_item.GetName(), pvc_item.GetName())
				break
			}
			time.Sleep(5 * time.Second)
			count = count + 1
		}
	}

	return nil
}
