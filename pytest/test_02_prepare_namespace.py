from requests import *

# 02 prepare Namespace

def test_create_namespace():
	(code, body) = k8s_api_post('/api/v1/namespaces', 'resources/namespace.json')
	assert code == 201

def test_create_cloud_credential():
	(code, body) = k8s_api_post('/api/v1/namespaces/' + env.namespace + '/secrets', 'resources/cloudcredential.json')
	assert code == 201

def test_create_objectstore_config():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/objectstoreconfigs', 'resources/objectstoreconfig.json')
	assert code == 201

def test_create_clusterrole_binding():
	(code, body) = k8s_api_post('/apis/rbac.authorization.k8s.io/v1/clusterrolebindings', 'resources/clusterrolebinding.json')
	assert code == 201

def test_create_registry_key():
	(code, body) = k8s_api_post('/api/v1/namespaces/' + env.namespace + '/secrets', 'resources/registrykeysecret.json')
	assert code == 201
