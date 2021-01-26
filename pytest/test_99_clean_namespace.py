from requests import *
import pytest

# 99 vi delete Namespace

@pytest.mark.clean
def test_get_controller_pod_name():
	(code, body) = k8s_api_get('/api/v1/namespaces/' + env.namespace + '/pods?labelSelector=app%3d' + env.deploy)
	assert code == 200
	res = json.loads(body.decode('utf-8'))
	podname = 'None'
	if 'items' in res and len(res['items']) > 0:
		if 'metadata' in res['items'][0] and 'name' in res['items'][0]['metadata']:
			podname = res['items'][0]['metadata']['name']
	env.set('podname', podname)
	print("podname:" + env.podname, end = " ")

@pytest.mark.clean
def test_get_controller_log():
	(code, body) = k8s_api_get('/api/v1/namespaces/' + env.namespace + '/pods/' + env.podname + '/log')
	assert code == 200
	print(body.decode('utf-8'))

@pytest.mark.clean
def test_delete_namespace():
	(code, body) = k8s_api_delete('/api/v1/namespaces/' + env.namespace)
	assert code == 200

@pytest.mark.clean
def test_wait_namespace_pods_cleaned():
	assert k8s_wait_all_deleted('/api/v1/namespaces/' + env.namespace + '/pods')

@pytest.mark.clean
def test_delete_clusterrole_binding():
	(code, body) = k8s_api_delete('/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/' + env.namespace)
	assert code == 200
