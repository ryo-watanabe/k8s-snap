# k8s-backup

## Summary
- Backup all k8s resources which has verbs - list, get, create, delete.
- Restore resources according to 'excludeApiPathes' and 'excludeNamespaces' in preference.
- Backup data stored on S3.
- Run as a k8s controller with CRDs.
- Backup/Restore k8s resources from/to external clusters.

## Features
### Done
- Restore basically by 'create'.
- Restore apps - deployments, statefulsets, daemonsets etc - after other resources restored.
- Restore PV/PVCs definitions and boundings for specified storageclasses in preference, for NFS backed PV. 
- Do not restore token secrets, resources with owner references, endpoints with same name services.

### ToDo
- Api pathes to admit resource overwriting.
- Syncer for backup resources and files on object store.
- Keep restore logs with each restore.

## Options
````
$ env | grep AWS_
AWS_SECRET_KEY=ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789
AWS_ACCESS_KEY=LMNOPQRSTUVWXYZ0123456

$ /k8s-backup-controller \
--awsregion=ap-northeast-1 \
--bucketname=k8s-backup
````
|param|default| | |
|----|----|----|----|
|kubeconfig| |Path to a kubeconfig. Only required if out-of-cluster|Optional|
|master| |The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.|Optional|
|namespace|k8s-backup|Namespace for k8s-backup|Optional|
|backupthreads|5|Number of backup threads|Optional|
|restorethreads|2|Number of restore threads|Optional|
|awsregion| |Set S3 Region|Required|
|awsendpoint| |Set S3 Endpoint for S3-compatible object stores|Optional|
|awsaccesskey| |Set S3 access key here or set environment variable AWS_ACCESS_KEY|Optional|
|awssecretkey| |Set S3 secret key here or set environment variable AWS_SECRET_KEY|Optional|
|bucketname| |S3 bucket name|Required|

## Deploy
````
$ kubectl apply -f artifacts/crd.yaml
$ kubectl apply -f artifacts/namespace-rbac.yaml
````
Edit aws access/secret key in artifacts/cloud-credential.yaml
````
$ kubectl apply -f artifacts/cloud-credential.yaml
````
Edit image and aws region in artifacts/deploy.yaml
````
$ kubectl apply -f artifacts/deploy.yaml
````
## On backup
### Create a backup resource
````
apiVersion: clusterbackup.ssl/v1alpha1
kind: Backup
metadata:
  name: cluster01-001
  namespace: k8s-backup
spec:
  clusterName: cluster01
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN3akND...
        server: https://111.111.111.111:6443
      name: cluster
    contexts:
    - context:
        cluster: cluster
        user: remote-user
      name: context
    current-context: context
    kind: Config
    preferences: {}
    users:
    - name: remote-user
      user:
        token: eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVz....
````
### Backup status
````
$ kubectl get backups.clusterbackup.ssl -n k8s-backup
NAME             CLUSTERNAME   BACKUPDATE             STATUS
cluster02-001    cluster02     2019-04-19T08:35:23Z   Completed
cluster02-002    cluster02     2019-04-22T05:04:35Z   InProgress
scluster02-001   sccluster02   2019-04-12T03:49:30Z   Completed
````
|phase|status|
|----|----|
|""(empty string)|Just created|
|InQueue|Waiting for proccess|
|InProgress|Doing backup|
|Failed|Error ocuered in backup|
|Completed|Backup done and available for restore|

### To get more for failed backup
````
$ kubectl get backups.clusterbackup.ssl -n k8s-backup scluster01-002 -o json | jq .status
{
  "phase": "Failed",
  "reason": "Unauthorized"
}
````
## On restore
### Setup a restore preference
Edit artifacts/preference.yaml and create one.

|Preference items| |format|
|----|----|----|
|excludeNamespaces|Namespaces to exclude|match exactly|
|excludeCRDs|CRDs to exclude|contains|
|excludeApiPathes|Api pathes to exclude|prefix,contains or prefix|
|restoreAppApiPathes|Api pathes to restore after other resources|prefix,contains or prefix|
|restoreNfsStorageClasses|Storageclasses to rebound PV/PVC|prefix|
|restoreOptions|(not in use)||

### Create a restore resource
````
apiVersion: clusterbackup.ssl/v1alpha1
kind: Restore
metadata:
  name: cluster02-cluster01-001-001
  namespace: k8s-backup
spec:
  clusterName: cluster02
  backupName: cluster01-001
  restorePreferenceName: user-backup
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN3akND...
        server: https://111.111.111.111:6443
      name: cluster
    contexts:
    - context:
        cluster: cluster
        user: remote-user
      name: context
    current-context: context
    kind: Config
    preferences: {}
    users:
    - name: remote-user
      user:
        token: eyJhbGciOiJSUzI1NiIsImtpZCI6IiJ9.eyJpc3MiOiJrdWJlcm5ldGVz....
````
### Restore status
````
$ kubectl get restores.clusterbackup.ssl -n k8s-backup
NAME                            CLUSTERNAME   BACKUPNAME       RESTOREDATE            STATUS
cluster03-cluster02-001-001     cluster03     cluster02-001    2019-04-22T04:04:56Z   Failed
cluster03-cluster02-002-001     cluster03     cluster02-002    2019-04-22T05:10:43Z   Completed
scluster01-scluster02-001-001   scluster01    scluster02-001   2019-04-16T05:15:02Z   Completed
scluster02-scluster01-001-001   scluster02    scluster01-001   2019-05-09T07:55:44Z   InProgress
````
|phase|status|
|----|----|
|""(empty string)|Just created|
|InQueue|Waiting for proccess|
|InProgress|Doing restore|
|Failed|Error ocuered in restore|
|Completed|Restore done|

### To get more for failed restore
````
$ kubectl get restores.clusterbackup.ssl -n k8s-backup cluster03-cluster02-001-001 -o json | jq .status
{
  "phase": "Failed",
  "reason": "Error downloading cluster02-001.tgz from bucket k8s-backup : NoSuchKey: \n\tstatus code: 404, request id: tx000000000000000d690d1-005cbeaeb6-a9c6a83-default, host id: "
}
````
## On delete backup
````
$ kubectl delete backups.clusterbackup.ssl -n k8s-backup scluster01-001
````
Delete backup resource and automatically data on object store deleted.
