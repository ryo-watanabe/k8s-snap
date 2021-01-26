from requests import *

# 03 deploy Objectstore Mock

def test_create_mock_tls():
	(code, body) = k8s_api_post('/api/v1/namespaces/' + env.namespace + '/secrets', 'resources/mocktlssecret.json')
	assert code == 201

def test_create_mock_deployment():
	(code, body) = k8s_api_post('/apis/apps/v1/namespaces/' + env.namespace + '/deployments', 'resources/mockdeployment.json')
	assert code == 201

def test_create_mock_service():
	(code, body) = k8s_api_post('/api/v1/namespaces/' + env.namespace + '/services', 'resources/mockservice.json')
	assert code == 201

def test_wait_mock_deployment_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.namespace + '/deployments/' + env.mockdeploy, 1)
