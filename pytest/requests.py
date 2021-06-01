import urllib.request
import ssl
import json
from jinja2 import Template
from time import sleep
from env import Env

ssl._create_default_https_context = ssl._create_unverified_context
env = Env()
sleepSec = 30

# Get a k8s resource
def k8s_api_get(url_path):
	body = ''
	code = 0
	req = urllib.request.Request(env.url_base + url_path)
	req.add_header('Authorization', 'Bearer ' + env.token)
	try:
		res = urllib.request.urlopen(req)
	except urllib.error.HTTPError as err:
		if err.code == 404:
			return (404, body)
		else:
			raise
	body = res.read()
	code = res.getcode()
	return (code, body)

# Create a k8s resource
def k8s_api_post(url_path, file):
	body = ''
	code = 0
	json = ''
	with open(file) as file_:
		template = Template(file_.read())
		json = template.render(env=env).encode('utf-8')
		req = urllib.request.Request(env.url_base + url_path, json)
		req.add_header('Authorization', 'Bearer ' + env.token)
		req.add_header('Content-type', 'application/json')
		with urllib.request.urlopen(req) as res:
			body = res.read()
			code = res.getcode()
	return (code, body)

# Delete a k8s resource
def k8s_api_delete(url_path):
	body = ''
	code = 0
	req = urllib.request.Request(env.url_base + url_path, method='DELETE')
	req.add_header('Authorization', 'Bearer ' + env.token)
	with urllib.request.urlopen(req) as res:
		body = res.read()
		code = res.getcode()
	return (code, body)

# Wait deployment/statefulset's readyReplicas becomes desired number
def k8s_wait_app_ready(url_path, readyNum):
	ready = False
	for num in range(1, 10):
		(code, body) = k8s_api_get(url_path)
		assert code == 200
		res = json.loads(body.decode('utf-8'))
		if 'status' in res:
			if 'readyReplicas' in res['status'] and res['status']['readyReplicas'] == readyNum:
				ready = True
				break
		print(".", end=" ")
		sleep(sleepSec)
	return ready

# Wait a k8s resource deleted(=404)
def k8s_wait_deleted(url_path):
	ready = False
	for num in range(1, 10):
		(code, body) = k8s_api_get(url_path)
		if code == 404:
			ready = True
			break
		assert code == 200
		print(".", end=" ")
		sleep(sleepSec)
	return ready

# wait all k8s resources in list. url_path must be a request for list
def k8s_wait_all_deleted(url_path):
	ready = False
	for num in range(1, 10):
		(code, body) = k8s_api_get(url_path)
		assert code == 200
		res = json.loads(body.decode('utf-8'))
		if 'items' in res and len(res['items']) == 0:
			ready = True
			sleep(sleepSec)
			break
		print(".", end=" ")
		sleep(sleepSec)
	return ready

# get a pod's IP
def k8s_get_pod_ip(url_path):
	(code, body) = k8s_api_get(url_path)
	assert code == 200
	res = json.loads(body.decode('utf-8'))
	podIp = 'none'
	if 'status' in res and 'podIP' in res['status']:
		podIp = res['status']['podIP']
	return podIp

# simple http request
def http_get(url):
	body = ''
	code = 0
	req = urllib.request.Request(url)
	with urllib.request.urlopen(req) as res:
		body = res.read()
		code = res.getcode()
	return (code, body)

# wait for snapshot/restore's status.phase becomes 'Completed'
def snapshot_wait_completed(url_path):
	ready = False
	for num in range(1, 10):
		(code, body) = k8s_api_get(url_path)
		assert code == 200
		res = json.loads(body.decode('utf-8'))
		if 'status' in res:
			if 'phase' in res['status']:
				if res['status']['phase'] == "Completed":
					ready = True
					break
				if res['status']['phase'] == "Failed":
					break
		print(".", end=" ")
		sleep(sleepSec)
	return ready
