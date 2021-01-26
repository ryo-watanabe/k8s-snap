from requests import *
import pytest

# 06 backup/restore Test App

# 06-01 store Namespaces

def test_get_residential_namespaces():
	(code, body) = k8s_api_get('/api/v1/namespaces')
	assert code == 200
	res = json.loads(body.decode('utf-8'))
	namespaces = ""
	if 'items' in res and len(res['items']) > 0:
		for ns in res['items']:
			if 'metadata' in ns and 'name' in ns['metadata']:
				namespaces += '"' + ns['metadata']['name'] + '",'
		namespaces = namespaces[:-1]
	env.set('residentnamespaces', namespaces)
	print("residential namespaces:" + env.get('residentnamespaces'), end="")
	return

# 06-02 deploy Test App

def test_create_testapp_namespace():
	(code, body) = k8s_api_post('/api/v1/namespaces', 'resources/test_06/testappns.json')
	assert code == 201

def test_create_testapp_deployment():
	(code, body) = k8s_api_post('/apis/apps/v1/namespaces/' + env.testappns + '/deployments', 'resources/test_06/testappdeploy.json')
	assert code == 201

def test_wait_testapp_deployment_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.testappns + '/deployments/' + env.testappdeploy, 3)

# 06-03 take Snapshot

def test_create_snapshot_002():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots', 'resources/test_06/snapshot_002.json')
	assert code == 201

def test_wait_snapshot_002_completed():
	assert snapshot_wait_completed('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots/' + env.targetname + '-002')

# 06-04 delete Test App

def test_delete_testapp_namespace():
	(code, body) = k8s_api_delete('/api/v1/namespaces/' + env.testappns)
	assert code == 200

def test_wait_testapp_pods_deleted():
	assert k8s_wait_all_deleted('/api/v1/namespaces/' + env.testappns + '/pods')

# 06-05 restore Test App

def test_create_preference():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/restorepreferences', 'resources/test_06/preference.json')
	assert code == 201

def test_create_restore_002():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/restores', 'resources/test_06/restore_002.json')
	assert code == 201

def test_wait_restore_002_completed():
	assert snapshot_wait_completed('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/restores/' + env.targetname + '-002-restore-01')

def test_wait_restored_testapp_deployment_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.testappns + '/deployments/' + env.testappdeploy, 3)

# 06-99 clean Test App

@pytest.mark.clean
def test_clean_testapp_namespace():
	(code, body) = k8s_api_delete('/api/v1/namespaces/' + env.testappns)
	assert code == 200

def test_wait_testapp_pods_cleaned():
	assert k8s_wait_all_deleted('/api/v1/namespaces/' + env.testappns + '/pods')
