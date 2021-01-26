from requests import *

# 01 CRDs exists

def test_crd_snapshots_exists():
	(code, body) = k8s_api_get('/apis/apiextensions.k8s.io/v1beta1/customresourcedefinitions/snapshots.' + env.groupname)
	assert code == 200

def test_crd_restores_exists():
	(code, body) = k8s_api_get('/apis/apiextensions.k8s.io/v1beta1/customresourcedefinitions/restores.' + env.groupname)
	assert code == 200

def test_crd_objectstoreconfigs_exists():
	(code, body) = k8s_api_get('/apis/apiextensions.k8s.io/v1beta1/customresourcedefinitions/objectstoreconfigs.' + env.groupname)
	assert code == 200

def test_crd_restorepreferences_exists():
	(code, body) = k8s_api_get('/apis/apiextensions.k8s.io/v1beta1/customresourcedefinitions/restorepreferences.' + env.groupname)
	assert code == 200
