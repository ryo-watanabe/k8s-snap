# k8s-snap

## Features
- Take a snapshot of all k8s resources which has verbs 'list', 'get', 'create' and 'delete'.
- Restore k8s resources from a snapshot on any k8s cluster.
- Select restoring resources according to 'excludeApiPathes' and 'excludeNamespaces' in preference.
- Backup data stored on S3.
- Run on a k8s with CRDs.

### Restoring ditails
- Restore resources basically by 'create', not by 'update'.
- Restore apps(deployments, statefulsets, daemonsets) after other resources restored.
- Restore PV definitions and PV/PVC boundings for specified storageclasses.
- Do not restore token secrets, resources with owner references, endpoints with same name services.

### TODO
- Overwriting resources for specified api pathes.
- 'Include' contexts in preference. Currently 'Exclude' only.

## Options
````
$ env | grep AWS_
AWS_SECRET_KEY=ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789
AWS_ACCESS_KEY=LMNOPQRSTUVWXYZ0123456

$ /k8s-snap-controller \
--namespace=k8s-snap
````
|param|default| | |
|----|----|----|----|
|kubeconfig| |Path to a kubeconfig. Only required if out-of-cluster|Optional|
|master| |The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.|Optional|
|namespace|k8s-snap|Namespace for k8s-snap|Optional|
|backupthreads|5|Number of backup threads|Optional|
|restorethreads|2|Number of restore threads|Optional|
|housekeepstore|true|Check and clean up orphan files on object store regularly (every 300 seconds)|Optional|
|restoresnapshots|true|Restore snapshot from object store on start|Optional|
|validatefileinfo|true|Validate size and timestamp of files on object store|Optional|
|maxretryelaspsedminutes|5|Max elaspsed minutes to retry snapshot|Optional|

## Deploy
````
$ kubectl apply -f artifacts/crd.yaml
$ kubectl apply -f artifacts/namespace-rbac.yaml
````
Set access/secret key in artifacts/cloud-credential.yaml and create a secret.
````
apiVersion: v1
kind: Secret
metadata:
  namespace: k8s-snap
  name: k8s-snap-ap-northeast-1
data:
  accesskey: [base64 access_key]
  secretkey: [base64 secret_key]

$ kubectl apply -f artifacts/cloud-credential.yaml
````
Make a bucket for snapshots.  
Set object store endpoint and bucket name in artifacts/objectstore-config.yaml and create a config.
````
apiVersion: clustersnapshot.rywt.io/v1alpha1
kind: ObjectstoreConfig
metadata:
  name: k8s-snap-ap-northeast-1
  namespace: k8s-snap
spec:
  region: ap-northeast-1
  endpoint: ap-northeast-1.amazonaws.com
  bucket: k8s-snap
  cloudCredentialSecret: k8s-snap-ap-northeast-1

$ kubectl apply -f artifacts/objectstore-config.yaml
````
Set image and registry key in artifacts/deploy.yaml and deploy.
````
$ kubectl apply -f artifacts/deploy.yaml
````
## To take s snapshot
### Create a snapshot resource
````
apiVersion: clustersnapshot.rywt.io/v1alpha1
kind: Snapshot
metadata:
  name: cluster01-001
  namespace: k8s-snap
spec:
  clusterName: cluster01
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN3akND...
        server: https://cluster01.kubernetes.rywt.io:6443
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
  ttl: 720h
  availableUntil: 2020-07-01T02:03:04Z
````
### Snapshot status
````
$ kubectl get snapshots.clustersnapshot.rywt.io -n k8s-snap
NAME             CLUSTERNAME   BACKUPDATE             STATUS
cluster02-001    cluster02     2019-04-19T08:35:23Z   Completed
cluster02-002    cluster02     2019-04-22T05:04:35Z   InProgress
scluster02-001   sccluster02   2019-04-12T03:49:30Z   Completed
````
|phase|status|
|----|----|
|""(empty string)|Just created|
|InQueue|Waiting for proccess|
|InProgress|Taking snapshot|
|Failed|Error ocuered in taking snapshot|
|Completed|Snapshot done and available for restore|

#### Completed snapshot status example
````
$ kubectl get snapshots.clustersnapshot.rywt.io -n k8s-snap scluster01-001 -o json | jq .status
{
  "availableUntil": "2019-06-19T03:45:08Z",     /*** Snapshot custom resource and data will be deleted on .. ***/
  "contents": [                                 /*** K8s resources backuped in snapshot ***/
    "/api/v1/namespaces/default/configmaps/kubelet-broken-pipe",
    "/api/v1/namespaces/default/endpoints/kubernetes",
    "/api/v1/namespaces/default/persistentvolumeclaims/test-pvc",
    "/api/v1/namespaces/default/secrets/default-token-gj2xd",
    :
  ],
  "numberOfContents": 429,                      /*** Number of backuped k8s resources in snapshot ***/
  "phase": "Completed",                         /*** Status of snapshot ***/
  "reason": "",
  "snapshotResourceVersion": "4521912",         /*** K8s ResourceVersion on which resources in snapshot synced ***/
  "snapshotTimestamp": "2019-05-20T03:45:08Z",  /*** Timestamp corresponding to the ResourceVersion ***/
  "storedFileSize": 138145,                     /*** File size on object store ***/
  "storedTimestamp": "2019-05-20T03:45:08Z"     /*** File timestamp on object store ***/
}
````
#### Failed snapshot status example
````
$ kubectl get snapshots.clustersnapshot.rywt.io -n k8s-snap scluster01-002 -o json | jq .status
{
  "availableUntil": null,
  "contents": null,
  "numberOfContents": 0,
  "phase": "Failed",
  "reason": "Unauthorized",      /*** Error message on snapshot failure including go library error message. Non predictable. ***/
  "snapshotResourceVersion": "",
  "snapshotTimestamp": null,
  "storedFileSize": 0,
  "storedTimestamp": null
}
````
## To restore
### Setup a restore preference
Edit artifacts/preference.yaml and create a preference.

|Preference items| |format|
|----|----|----|
|excludeNamespaces|Namespaces to exclude|match exactly|
|excludeCRDs|CRDs to exclude|contains|
|excludeApiPathes|Api pathes to exclude|prefix,contains or prefix|
|(ToDo) includeNamespaces|Namespaces to include|match exactly|
|(ToDo) includeCRDs|CRDs to include|contains|
|(ToDo) includeApiPathes|Api pathes to include|prefix,contains or prefix|
|restoreAppApiPathes|Api pathes to restore after other resources|prefix,contains or prefix|
|restoreNfsStorageClasses|Storageclasses to rebound PV/PVC|prefix|
|(ToDo) restoreOptions|excludeContext,overwriteExisting,etc.||

* Currently only 'exclude' contexts are valid in preference.

### Create a restore resource
````
apiVersion: clustersnapshot.rywt.io/v1alpha1
kind: Restore
metadata:
  name: cluster02-cluster01-001-001
  namespace: k8s-backup
spec:
  clusterName: cluster02
  backupName: cluster01-001
  restorePreferenceName: exclude-kube-system
  kubeconfig: |
    apiVersion: v1
    clusters:
    - cluster:
        certificate-authority-data: LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUN3akND...
        server: https://cluster02.kubernetes.rywt.io:6443
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
  ttl: 168h
  availableUntil: 2020-07-01T01:02:03Z
````
* Set ttl with time.Duration format h/m/s. If not set, default to 168h0m0s(=7days).
* Spec.TTL will be ignored when Spec.AvailableUntil is set.

### Restore status
````
$ kubectl get restores.clustersnapshot.rywt.io -n k8s-snap
NAME                            CLUSTER      SNAPSHOT         TIMESTAMP              RV        EXCLUDED   CREATED   UPDATED   EXIST     FAILED    STATUS
scluster02-scluster01-001-001   scluster02   scluster01-001   2019-05-20T03:46:15Z   8514809   50         46        0         13        0         Completed
scluster02-scluster01-002-001   scluster02   scluster01-002   <no value>                       0          0         0         0         0         Failed
scluster02-scluster01-001-002   scluster02   scluster01-001   <no value>                       0          0         0         0         0         InProgress
````
|phase|status|
|----|----|
|""(empty string)|Just created|
|InQueue|Waiting for proccess|
|InProgress|Doing restore|
|Failed|Error ocuered in restore|
|Completed|Restore done|

#### Completed restore status example
````
$ kubectl get restores.clustersnapshot.rywt.io -n k8s-snap scluster02-scluster01-001-001 -o json | jq .status
{
  "alreadyExisted": [             /*** K8s resources existed and not tried to update ***/
    "/api/v1/namespaces/default",
    "/api/v1/persistentvolumes/pvc-4ae96dfc-625b-11e9-9d94-005056bc5df9",
    "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/remote-user",
    "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/logfilter-controller",
    :
  ],
  "created": [                   /*** Created k8s resources ***/
    "/api/v1/namespaces/fluent-bit",
    "/apis/rbac.authorization.k8s.io/v1/namespaces/default/rolebindings/clusterrolebinding-dtwx4",
    "/apis/rbac.authorization.k8s.io/v1/namespaces/default/rolebindings/clusterrolebinding-vmsqk",
    "/api/v1/namespaces/fluent-bit/configmaps/cattle-agent-no-such-host",
    :
  ],
  "excluded": [                  /*** K8s resources excluded in restoring by some reason - resource-path,(reason) ***/
    "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/canal-calico,(not-binded-to-ns)",
    "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/canal-flannel,(not-binded-to-ns)",
    "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/cattle-admin-binding,(not-binded-to-ns)",
    "/apis/rbac.authorization.k8s.io/v1/clusterrolebindings/cluster-admin,(not-binded-to-ns)",
    :
  ],
  "failed": null,                /*** K8s resources tried to create but failed - resource-path,error-message(<300chars) ***/
  "numAlreadyExisted": 13,       /*** Number of existed and not tried to update ***/
  "numCreated": 46,              /*** Number of created ***/
  "numExcluded": 50,             /*** Number of excluded in restoring by some reason ***/
  "numFailed": 0,                /*** Number of tried to create but failed ***/
  "numPreferenceExcluded": 319,  /*** Number of excluded in preference ***/
  "numSnapshotContents": 429,    /*** Number of k8s resources in the snapshot ***/
  "numUpdated": 0,               /*** Number of updated ***/
  "phase": "Completed",          /*** Status of restore ***/
  "preserveUntil": "2019-05-27T03:46:15Z",     /*** Restore custom resource will be deleted on .. ***/
  "reason": "",                  /*** Error message on restore failure including go library error message. Non predictable. ***/
  "restoreResourceVersion": "8514809",         /*** K8s ResourceVersion at restore finished ***/
  "restoreTimestamp": "2019-05-20T03:46:15Z",  /*** Timestamp corresponding to the ResourceVersion ***/
  "updated": null                /*** Updated k8s resources ***/
}
````
#### Failed restore status example
````
$ kubectl get restores.clustersnapshot.rywt.io -n k8s-snap scluster02-scluster01-002-001 -o json | jq .status
{
  "alreadyExisted": null,
  "created": null,
  "excluded": null,
  "failed": null,
  "numAlreadyExisted": 0,
  "numCreated": 0,
  "numExcluded": 0,
  "numFailed": 0,
  "numPreferenceExcluded": 0,
  "numSnapshotContents": 0,
  "numUpdated": 0,
  "phase": "Failed",
  "preserveUntil": null,
  "reason": "Snapshot data is not in status 'Completed'",   /*** Error message on restore failure including go library error message. Non predictable. ***/
  "restoreResourceVersion": "",
  "restoreTimestamp": null,
  "updated": null
}
````
## To delete snapshot
Snapshot resources and files on object store automatically deleted when TTL expired.  
You can delete a snapshot manually with:
````
$ kubectl delete snapshots.clustersnapshot.rywt.io -n k8s-snap cluster01-001
````
and also the corresponding file on object store automatically deleted.
