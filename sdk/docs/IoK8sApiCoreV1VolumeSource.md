# IoK8sApiCoreV1VolumeSource

## Properties
Name | Type | Description | Notes
------------ | ------------- | ------------- | -------------
**aws_elastic_block_store** | [**IoK8sApiCoreV1AWSElasticBlockStoreVolumeSource**](IoK8sApiCoreV1AWSElasticBlockStoreVolumeSource.md) | AWSElasticBlockStore represents an AWS Disk resource that is attached to a kubelet&#39;s host machine and then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore | [optional] 
**azure_disk** | [**IoK8sApiCoreV1AzureDiskVolumeSource**](IoK8sApiCoreV1AzureDiskVolumeSource.md) | AzureDisk represents an Azure Data Disk mount on the host and bind mount to the pod. | [optional] 
**azure_file** | [**IoK8sApiCoreV1AzureFileVolumeSource**](IoK8sApiCoreV1AzureFileVolumeSource.md) | AzureFile represents an Azure File Service mount on the host and bind mount to the pod. | [optional] 
**cephfs** | [**IoK8sApiCoreV1CephFSVolumeSource**](IoK8sApiCoreV1CephFSVolumeSource.md) | CephFS represents a Ceph FS mount on the host that shares a pod&#39;s lifetime | [optional] 
**cinder** | [**IoK8sApiCoreV1CinderVolumeSource**](IoK8sApiCoreV1CinderVolumeSource.md) | Cinder represents a cinder volume attached and mounted on kubelets host machine More info: https://releases.k8s.io/HEAD/examples/mysql-cinder-pd/README.md | [optional] 
**config_map** | [**IoK8sApiCoreV1ConfigMapVolumeSource**](IoK8sApiCoreV1ConfigMapVolumeSource.md) | ConfigMap represents a configMap that should populate this volume | [optional] 
**downward_api** | [**IoK8sApiCoreV1DownwardAPIVolumeSource**](IoK8sApiCoreV1DownwardAPIVolumeSource.md) | DownwardAPI represents downward API about the pod that should populate this volume | [optional] 
**empty_dir** | [**IoK8sApiCoreV1EmptyDirVolumeSource**](IoK8sApiCoreV1EmptyDirVolumeSource.md) | EmptyDir represents a temporary directory that shares a pod&#39;s lifetime. More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir | [optional] 
**fc** | [**IoK8sApiCoreV1FCVolumeSource**](IoK8sApiCoreV1FCVolumeSource.md) | FC represents a Fibre Channel resource that is attached to a kubelet&#39;s host machine and then exposed to the pod. | [optional] 
**flex_volume** | [**IoK8sApiCoreV1FlexVolumeSource**](IoK8sApiCoreV1FlexVolumeSource.md) | FlexVolume represents a generic volume resource that is provisioned/attached using an exec based plugin. | [optional] 
**flocker** | [**IoK8sApiCoreV1FlockerVolumeSource**](IoK8sApiCoreV1FlockerVolumeSource.md) | Flocker represents a Flocker volume attached to a kubelet&#39;s host machine. This depends on the Flocker control service being running | [optional] 
**gce_persistent_disk** | [**IoK8sApiCoreV1GCEPersistentDiskVolumeSource**](IoK8sApiCoreV1GCEPersistentDiskVolumeSource.md) | GCEPersistentDisk represents a GCE Disk resource that is attached to a kubelet&#39;s host machine and then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk | [optional] 
**git_repo** | [**IoK8sApiCoreV1GitRepoVolumeSource**](IoK8sApiCoreV1GitRepoVolumeSource.md) | GitRepo represents a git repository at a particular revision. DEPRECATED: GitRepo is deprecated. To provision a container with a git repo, mount an EmptyDir into an InitContainer that clones the repo using git, then mount the EmptyDir into the Pod&#39;s container. | [optional] 
**glusterfs** | [**IoK8sApiCoreV1GlusterfsVolumeSource**](IoK8sApiCoreV1GlusterfsVolumeSource.md) | Glusterfs represents a Glusterfs mount on the host that shares a pod&#39;s lifetime. More info: https://releases.k8s.io/HEAD/examples/volumes/glusterfs/README.md | [optional] 
**host_path** | [**IoK8sApiCoreV1HostPathVolumeSource**](IoK8sApiCoreV1HostPathVolumeSource.md) | HostPath represents a pre-existing file or directory on the host machine that is directly exposed to the container. This is generally used for system agents or other privileged things that are allowed to see the host machine. Most containers will NOT need this. More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath | [optional] 
**iscsi** | [**IoK8sApiCoreV1ISCSIVolumeSource**](IoK8sApiCoreV1ISCSIVolumeSource.md) | ISCSI represents an ISCSI Disk resource that is attached to a kubelet&#39;s host machine and then exposed to the pod. More info: https://releases.k8s.io/HEAD/examples/volumes/iscsi/README.md | [optional] 
**nfs** | [**IoK8sApiCoreV1NFSVolumeSource**](IoK8sApiCoreV1NFSVolumeSource.md) | NFS represents an NFS mount on the host that shares a pod&#39;s lifetime More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs | [optional] 
**persistent_volume_claim** | [**IoK8sApiCoreV1PersistentVolumeClaimVolumeSource**](IoK8sApiCoreV1PersistentVolumeClaimVolumeSource.md) | PersistentVolumeClaimVolumeSource represents a reference to a PersistentVolumeClaim in the same namespace. More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims | [optional] 
**photon_persistent_disk** | [**IoK8sApiCoreV1PhotonPersistentDiskVolumeSource**](IoK8sApiCoreV1PhotonPersistentDiskVolumeSource.md) | PhotonPersistentDisk represents a PhotonController persistent disk attached and mounted on kubelets host machine | [optional] 
**portworx_volume** | [**IoK8sApiCoreV1PortworxVolumeSource**](IoK8sApiCoreV1PortworxVolumeSource.md) | PortworxVolume represents a portworx volume attached and mounted on kubelets host machine | [optional] 
**projected** | [**IoK8sApiCoreV1ProjectedVolumeSource**](IoK8sApiCoreV1ProjectedVolumeSource.md) | Items for all in one resources secrets, configmaps, and downward API | [optional] 
**quobyte** | [**IoK8sApiCoreV1QuobyteVolumeSource**](IoK8sApiCoreV1QuobyteVolumeSource.md) | Quobyte represents a Quobyte mount on the host that shares a pod&#39;s lifetime | [optional] 
**rbd** | [**IoK8sApiCoreV1RBDVolumeSource**](IoK8sApiCoreV1RBDVolumeSource.md) | RBD represents a Rados Block Device mount on the host that shares a pod&#39;s lifetime. More info: https://releases.k8s.io/HEAD/examples/volumes/rbd/README.md | [optional] 
**scale_io** | [**IoK8sApiCoreV1ScaleIOVolumeSource**](IoK8sApiCoreV1ScaleIOVolumeSource.md) | ScaleIO represents a ScaleIO persistent volume attached and mounted on Kubernetes nodes. | [optional] 
**secret** | [**IoK8sApiCoreV1SecretVolumeSource**](IoK8sApiCoreV1SecretVolumeSource.md) | Secret represents a secret that should populate this volume. More info: https://kubernetes.io/docs/concepts/storage/volumes#secret | [optional] 
**storageos** | [**IoK8sApiCoreV1StorageOSVolumeSource**](IoK8sApiCoreV1StorageOSVolumeSource.md) | StorageOS represents a StorageOS volume attached and mounted on Kubernetes nodes. | [optional] 
**vsphere_volume** | [**IoK8sApiCoreV1VsphereVirtualDiskVolumeSource**](IoK8sApiCoreV1VsphereVirtualDiskVolumeSource.md) | VsphereVolume represents a vSphere volume attached and mounted on kubelets host machine | [optional] 

[[Back to Model list]](../README.md#documentation-for-models) [[Back to API list]](../README.md#documentation-for-api-endpoints) [[Back to README]](../README.md)


