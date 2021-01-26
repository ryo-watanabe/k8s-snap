# pytest for testing k8s-snap in a kubernetes cluster

- Bind cluster-admin clusterrole to some service account

- Get token and cacert from service account's secret

- Set token, cacert, url_base and image in env.py

- Run pytest on a alpine container
```
docker run -d -v $(pwd)/pytest:/pytest alpine tail -f /dev/null
docker exec -it e3764a2c6a84 /bin/sh
/ # cd pytest/
/pytest # pytest -s -x -v
```
- Clean up failed test
```
/pytest # pytest -s -v -m clean
```