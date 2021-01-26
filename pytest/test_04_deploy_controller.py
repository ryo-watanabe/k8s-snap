from requests import *

# 04 deploy controller

def test_create_controller_deployment():
	(code, body) = k8s_api_post('/apis/apps/v1/namespaces/' + env.namespace + '/deployments', 'resources/controllerdeployment.json')
	assert code == 201

def test_wait_controller_deployment_ready():
	assert k8s_wait_app_ready('/apis/apps/v1/namespaces/' + env.namespace + '/deployments/' + env.deploy, 1)
