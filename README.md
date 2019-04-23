# k8s-backup

## Summary
- Backup all k8s resources which has verbs - list, get, create, delete.
- Restore resources according to 'excludeApiPathes' and 'excludeNamespaces' in preference.
- Backup data stored on S3.
- Run as a controller with CRDs.
- Backup/Restore k8s resources from/to external clusters.

## Features
### Done
- Restore basically by 'create'.
- Restore apps - deployments, statefulsets, daemonsets etc - after other resources restored.
- Restore PV/PVCs definitions and boundings for specified storageclasses in preference, for NFS backed PV. 
- Do not restore token secrets, resources with owner references, endpoints with same name services.

### TODO
- Api pathes to admit resource overwriting.
- Syncer for backup resources and files on object store.
- Keep restore logs with each restore.

## Options
````
/k8s-backup-controller \
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
|awsregion| |Set S3 Region|Reuired|
|awsendpoint| |Set S3 Endpoint for S3-compatible object stores|Optional|
|awsaccesskey| |Set S3 access key here or set environment variable AWS_ACCESS_KEY|Optional|
|awssecretkey| |Set S3 secret key here or set environment variable AWS_SECRET_KEY|Optional|
|bucketname| |S3 bucket name|Reuired|
