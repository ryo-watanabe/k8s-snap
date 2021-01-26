
class Env:
	token = "TOKEN"
	groupname = 'clustersnapshot.rywt.io'
	namespace = 'snapshot-test'
	secret = 'cloud-credential'
	accesskey_encoded = "bWluaW8="
	secretkey_encoded = "bWluaW8xMjM="
	url_base = 'https://111.10.0.1:6443'
	objectstoreconfig = "objectstoreconfig"
	region = "jp-east-2"
	bucket = "k8s-snap"
	image = "IMAGE"
	deploy = "k8s-snap"
	targetname = "target-cluster"
	accesskey = "minio"
	secretkey = "minio123"
	mockdeploy = "minio"
	clusterip = "10.43.0.100"
	mockimage = "minio/minio:RELEASE.2019-09-05T23-24-38Z"
	cacert = "CACERT"
	testappns = "restore-test-nginx"
	testappimage = "nginx"
	testappdeploy = "nginx-test"
	preference = "exclude-existing"
	command = "k8s-snap"
	nfsdeploy = "nfs"
	nfsclusterip = "10.43.0.101"
	nfsimage = "itsthenetwork/nfs-server-alpine"
	pvtestappns = "restore-nginx-pv-test"
	pvname = "test-nfs-pv"
	storageclass = "test-nfs-storage"

	def get(self, key):
		return getattr(self, key)

	def set(self, key, value):
		setattr(self, key, value)
