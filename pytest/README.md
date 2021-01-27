# pytest for testing k8s-snap in a kubernetes cluster

- Bind cluster-admin clusterrole to some service account

- Get token and cacert from the service account's secret

- Set token, cacert, url_base and image in env.py, also clusterip and nfsclusterip must be in the range of ClusterIP CIDR

- Run pytest on a alpine container
```
docker run -d -v $(pwd)/pytest:/pytest --name pytest alpine tail -f /dev/null
docker exec -it pytest /bin/sh
/ # apk add --no-cache coreutils py3-pytest py3-jinja2 py3-urllib3
/ # cd pytest/
/pytest # pytest -s -x -v
```
- Clean up failed test
```
/pytest # pytest -s -v -m clean
```