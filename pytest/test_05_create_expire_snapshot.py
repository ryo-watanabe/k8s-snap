from requests import *

# 05 create/expire Snapshot

def test_create_snapshot_001():
	(code, body) = k8s_api_post('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots', 'resources/snapshot_001.json')
	assert code == 201

def test_wait_snapshot_001_completed():
	assert snapshot_wait_completed('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots/' + env.targetname + '-001')

def test_wait_snapshot_001_expired():
	assert k8s_wait_deleted('/apis/' + env.groupname + '/v1alpha1/namespaces/' + env.namespace + '/snapshots/' + env.targetname + '-001')
