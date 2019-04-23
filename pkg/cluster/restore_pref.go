package cluster

import (
	"strings"
	"os"
	"io/ioutil"
	"path/filepath"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/klog"
	cbv1alpha1 "github.com/ryo-watanabe/k8s-backup/pkg/apis/clusterbackup/v1alpha1"
)

func apiPathMatched(path, apiPath string) bool {
	sp := strings.Split(apiPath, ",")
	if len(sp) == 1 &&
	   strings.HasPrefix(path, sp[0]) {
		return true
	}
	if len(sp) == 2 &&
	   strings.HasPrefix(path, sp[0]) &&
	   strings.Contains(path, sp[1]) {
		return true
	}
	return false
}

type preference struct {
	pref *cbv1alpha1.RestorePreference
	includedClusterRoles []string
	includedClusterRoleBindings []string
	serviceList []string
	dirs []os.FileInfo
	cntRestored int
	cntExcluded int
	cntAlreadyExists int
	cntOtherErrors int
}

func newPreference(pref *cbv1alpha1.RestorePreference) *preference {
	return &preference{
		pref: pref,
		cntRestored: 0,
		cntExcluded: 0,
		cntAlreadyExists: 0,
		cntOtherErrors: 0,
	}
}

// count up funcs
func (p *preference) cntUpExcluded() {
	p.cntExcluded = p.cntExcluded + 1
}
func (p *preference) cntUpRestored() {
	p.cntRestored = p.cntRestored + 1
}
func (p *preference) cntUpCnnotRestore(error string) {
	if strings.Contains(error, "already exists") {
		p.cntAlreadyExists = p.cntAlreadyExists + 1
	} else {
		p.cntOtherErrors = p.cntOtherErrors + 1
	}
}

func (p *preference) preferedToRestore(path string) string {

	// namespace resources
	if strings.HasPrefix(path, "/namespaces/") {
		for _, n := range p.pref.Spec.ExcludeNamespaces {
			if strings.Contains(path, n) {
				return "Exclude"
			}
		}
		return "Namespace"
	}
	// crds
	if strings.HasPrefix(path, "/crds/") {
		for _, crd := range p.pref.Spec.ExcludeCRDs {
			if strings.Contains(path, crd) {
				return "Exclude"
			}
		}
		return "CRD"
	}
	// check exclude API pathes
	for _, p := range p.pref.Spec.ExcludeApiPathes {
		if apiPathMatched(path, p) {
			return "Exclude"
		}
	}
	// check exclude namespaces
	for _, n := range p.pref.Spec.ExcludeNamespaces {
		if strings.Contains(path, "namespaces/" + n) {
			return "Exclude"
		}
	}
	// check storage classes
	if strings.Contains(path, "/storageclasses/") {
		for _, s := range p.pref.Spec.RestoreNfsStorageClasses {
			if strings.Contains(path, "storageclasses/" + s) {
				return "Restore"
			}
		}
		return "Exclude"
	}
	// check PV/PVC
	if strings.Contains(path, "/persistentvolumes/") {
		return "PV"
	}
	if strings.Contains(path, "/persistentvolumeclaims/") {
		return "PVC"
	}
	// check Apps API pathes
	for _, p := range p.pref.Spec.RestoreAppApiPathes {
		if apiPathMatched(path, p) {
			return "App"
		}
	}
	// other resources to restore
	return "Restore"
}

func (p *preference) isUserNamespace(nsName string) bool {
	for _, n := range p.pref.Spec.ExcludeNamespaces {
		if nsName == n {
			return false
		}
	}
	return true
}

func (p *preference) isIn(str string) bool {
	for _, f := range p.dirs {
		if str == f.Name() {
			return true
		}
	}
	return false
}

func (p *preference) initializeByDir(dir string) error {

	finfo, err := ioutil.ReadDir(dir)
	if err != nil {
		return err
	}
	p.dirs = finfo
	p.includedClusterRoles = nil
	p.includedClusterRoleBindings = nil
	p.serviceList = nil

	if p.isIn("Restore") {
		err = p.setIncludedClusterRoles(dir, "Restore")
		if err != nil {
			return err
		}
		err = p.setServiceList(dir, "Restore")
		if err != nil {
			return err
		}
	}
	if p.isIn("App") {
		err = p.setIncludedClusterRoles(dir, "App")
		if err != nil {
			return err
		}
		err = p.setServiceList(dir, "App")
		if err != nil {
			return err
		}
	}

	return nil
}

func (p *preference) setIncludedClusterRoles(dir, restorePref string) error {
	files, err := ioutil.ReadDir(filepath.Join(dir, restorePref))
	if err != nil {
		return err
	}
	klog.Infof("Included ClusterRoles : %s", restorePref)
	for _, f := range files {
		if strings.Contains(f.Name(), "|clusterrolebindings|") {
			// Load item
			var item unstructured.Unstructured
			err := loadItem(&item, filepath.Join(dir, restorePref, f.Name()))
			if err != nil {
				return err
			}
			subjects, ok := item.Object["subjects"].([]interface{})
			if !ok {
				continue
			}
			include := false
			for _, sub := range subjects {
				s := sub.(map[string]interface{})
				if s["kind"] == "ServiceAccount" {
					if p.isUserNamespace(s["namespace"].(string)) {
						include = true
					}
				}
			}
			if include {
				roleref, ok := item.Object["roleRef"].(map[string]interface{})
				if !ok {
					continue
				}
				p.includedClusterRoles = append(p.includedClusterRoles, roleref["name"].(string))
				p.includedClusterRoleBindings = append(p.includedClusterRoleBindings, item.GetName())
				klog.Infof("---- %s referenced in %s", roleref["name"], item.GetName())
			}
		}
	}
	return nil
}

func (p *preference) setServiceList(dir, restorePref string) error {
	files, err := ioutil.ReadDir(filepath.Join(dir, restorePref))
	if err != nil {
		return err
	}
	klog.Infof("Included Services : %s", restorePref)
	for _, f := range files {
		if strings.Contains(f.Name(), "|services|") {
			// Load item
			var item unstructured.Unstructured
			err := loadItem(&item, filepath.Join(dir, restorePref, f.Name()))
			if err != nil {
				return err
			}
			service := item.GetNamespace() + "/" + item.GetName()
			p.serviceList = append(p.serviceList, service)
			klog.Infof("---- %s", service)
		}
	}
	return nil
}

func (p *preference) isIncludedStorageClass(storageClassName string) bool {
	for _, s := range p.pref.Spec.RestoreNfsStorageClasses {
		if strings.HasPrefix(storageClassName, s) {
			return true
		}
	}
	return false
}

// Util: is name in list
func isInList(name string, list []string) bool {
	for _, s := range list {
		if name == s {
			return true
		}
	}
	return false
}
