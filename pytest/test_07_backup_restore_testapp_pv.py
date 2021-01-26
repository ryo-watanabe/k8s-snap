from requests import *
import pytest

# 07 backup/restore Test App with PV

# 07-01 store namespaces

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

# 07-02 deploy nfs server

def test_create_nfs_server_deployment():
	(code, body) = k8s_api_post('/apis/apps/v1/namespaces/' + env.namespace + '/deployments', 'resources/test_07/nfsdeployment.json')
	assert code == 201

def test_create_nfs_server_service():
	(code, body) = k8s_api_post('/api/v1/namespaces/' + env.namespace + '/services', 'resources/test_07/nfsservice.json')
	assert code == 201

def test_wait_nfs_deployment_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.namespace + '/deployments/' + env.nfsdeploy, 1)

def test_create_storage_class():
	(code, body) = k8s_api_post('/apis/storage.k8s.io/v1/storageclasses', 'resources/test_07/storageclass.json')
	assert code == 201

# 07-03 create PVs

def test_create_pv1():
	(code, body) = k8s_api_post('/api/v1/persistentvolumes', 'resources/test_07/pv01.json')
	assert code == 201

def test_create_pv2():
	(code, body) = k8s_api_post('/api/v1/persistentvolumes', 'resources/test_07/pv02.json')
	assert code == 201

def test_create_pv3():
	(code, body) = k8s_api_post('/api/v1/persistentvolumes', 'resources/test_07/pv03.json')
	assert code == 201

# 07-04 deploy PV Test App

def test_create_pv_testapp_namespace():
	(code, body) = k8s_api_post('/api/v1/namespaces', 'resources/test_07/pvtestappns.json')
	assert code == 201

def test_create_pv_testapp_statefulset():
	(code, body) = k8s_api_post('/apis/apps/v1/namespaces/' + env.pvtestappns + '/statefulsets', 'resources/test_07/pvtestappstatefulset.json')
	assert code == 201

def test_wait_pv_testapp_statefulset_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.pvtestappns + '/statefulsets/' + env.testappdeploy, 3)

def test_get_pod0_content():
	ip = k8s_get_pod_ip('/api/v1/namespaces/' + env.pvtestappns + '/pods/' + env.testappdeploy + '-0')
	print('pod0 IP:' + ip, end=" ")
	(code, body) = http_get('http://' + ip)
	assert code == 200
	env.set('pod0content', body.decode('utf-8'))
	print('pod0 content:' + env.pod0content, end=" ")

def test_get_pod1_content():
	ip = k8s_get_pod_ip('/api/v1/namespaces/' + env.pvtestappns + '/pods/' + env.testappdeploy + '-1')
	print('pod1 IP:' + ip, end=" ")
	(code, body) = http_get('http://' + ip)
	assert code == 200
	env.set('pod1content', body.decode('utf-8'))
	print('pod1 content:' + env.pod1content, end=" ")

def test_get_pod2_content():
	ip = k8s_get_pod_ip('/api/v1/namespaces/' + env.pvtestappns + '/pods/' + env.testappdeploy + '-2')
	print('pod2 IP:' + ip, end=" ")
	(code, body) = http_get('http://' + ip)
	assert code == 200
	env.set('pod2content', body.decode('utf-8'))
	print('pod2 content:' + env.pod2content, end=" ")

# 07-05 take Snapshot

def test_create_snapshot_003():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots', 'resources/test_07/snapshot_003.json')
	assert code == 201

def test_wait_snapshot_003_completed():
	assert snapshot_wait_completed('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots/' + env.targetname + '-003')

# 07-06 delete PV Test App

def test_delete_pv_testapp_namespace():
	(code, body) = k8s_api_delete('/api/v1/namespaces/' + env.pvtestappns)
	assert code == 200

def test_wait_pv_testapp_pods_deleted():
	assert k8s_wait_all_deleted('/api/v1/namespaces/' + env.pvtestappns + '/pods')

def test_delete_pv1():
	(code, body) = k8s_api_delete('/api/v1/persistentvolumes/' + env.pvname + '-01')
	assert code == 200

def test_delete_pv2():
	(code, body) = k8s_api_delete('/api/v1/persistentvolumes/' + env.pvname + '-02')
	assert code == 200

def test_delete_pv3():
	(code, body) = k8s_api_delete('/api/v1/persistentvolumes/' + env.pvname + '-03')
	assert code == 200

# 07-07 restore PV Test App

def test_create_pv_preference():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/restorepreferences', 'resources/test_07/pvpreference.json')
	assert code == 201

def test_create_restore_003():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/restores', 'resources/test_07/restore_003.json')
	assert code == 201

def test_wait_restore_003_completed():
	assert snapshot_wait_completed('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/restores/' + env.targetname + '-003-restore-01')

def test_wait_restored_pv_testapp_statefulset_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.pvtestappns + '/statefulsets/' + env.testappdeploy, 3)

def test_get_restored_pod0_content():
	ip = k8s_get_pod_ip('/api/v1/namespaces/' + env.pvtestappns + '/pods/' + env.testappdeploy + '-0')
	print('pod0 IP:' + ip, end=" ")
	(code, body) = http_get('http://' + ip)
	assert code == 200
	content = body.decode('utf-8')
	print('pod0 content:' + content, end=" ")
	assert content == env.pod0content

def test_get_restored_pod1_content():
	ip = k8s_get_pod_ip('/api/v1/namespaces/' + env.pvtestappns + '/pods/' + env.testappdeploy + '-1')
	print('pod1 IP:' + ip, end=" ")
	(code, body) = http_get('http://' + ip)
	assert code == 200
	content = body.decode('utf-8')
	print('pod1 content:' + content, end=" ")
	assert content == env.pod1content

def test_get_restored_pod2_content():
	ip = k8s_get_pod_ip('/api/v1/namespaces/' + env.pvtestappns + '/pods/' + env.testappdeploy + '-2')
	print('pod2 IP:' + ip, end=" ")
	(code, body) = http_get('http://' + ip)
	assert code == 200
	content = body.decode('utf-8')
	print('pod2 content:' + content, end=" ")
	assert content == env.pod2content

# 07-99 clean PV Test App

@pytest.mark.clean
def test_clean_pv_testapp_namespace():
	(code, body) = k8s_api_delete('/api/v1/namespaces/' + env.pvtestappns)
	assert code == 200

@pytest.mark.clean
def test_wait_pv_testapp_pods_cleaned():
	assert k8s_wait_all_deleted('/api/v1/namespaces/' + env.pvtestappns + '/pods')

@pytest.mark.clean
def test_clean_pv1():
	(code, body) = k8s_api_delete('/api/v1/persistentvolumes/' + env.pvname + '-01')
	assert code == 200

@pytest.mark.clean
def test_clean_pv2():
	(code, body) = k8s_api_delete('/api/v1/persistentvolumes/' + env.pvname + '-02')
	assert code == 200

@pytest.mark.clean
def test_clean_pv3():
	(code, body) = k8s_api_delete('/api/v1/persistentvolumes/' + env.pvname + '-03')
	assert code == 200

@pytest.mark.clean
def test_clean_storageclass():
	(code, body) = k8s_api_delete('/apis/storage.k8s.io/v1/storageclasses/' + env.storageclass)
	assert code == 200
