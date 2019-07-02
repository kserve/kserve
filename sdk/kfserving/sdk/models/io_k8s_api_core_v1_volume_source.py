# coding: utf-8

"""
    KFServing

    Python SDK for KFServing  # noqa: E501

    OpenAPI spec version: v0.1
    
    Generated by: https://github.com/swagger-api/swagger-codegen.git
"""


import pprint
import re  # noqa: F401

import six

from kfserving.sdk.models.io_k8s_api_core_v1_aws_elastic_block_store_volume_source import IoK8sApiCoreV1AWSElasticBlockStoreVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_azure_disk_volume_source import IoK8sApiCoreV1AzureDiskVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_azure_file_volume_source import IoK8sApiCoreV1AzureFileVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_ceph_fs_volume_source import IoK8sApiCoreV1CephFSVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_cinder_volume_source import IoK8sApiCoreV1CinderVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_config_map_volume_source import IoK8sApiCoreV1ConfigMapVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_downward_api_volume_source import IoK8sApiCoreV1DownwardAPIVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_empty_dir_volume_source import IoK8sApiCoreV1EmptyDirVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_fc_volume_source import IoK8sApiCoreV1FCVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_flex_volume_source import IoK8sApiCoreV1FlexVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_flocker_volume_source import IoK8sApiCoreV1FlockerVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_gce_persistent_disk_volume_source import IoK8sApiCoreV1GCEPersistentDiskVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_git_repo_volume_source import IoK8sApiCoreV1GitRepoVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_glusterfs_volume_source import IoK8sApiCoreV1GlusterfsVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_host_path_volume_source import IoK8sApiCoreV1HostPathVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_iscsi_volume_source import IoK8sApiCoreV1ISCSIVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_nfs_volume_source import IoK8sApiCoreV1NFSVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_persistent_volume_claim_volume_source import IoK8sApiCoreV1PersistentVolumeClaimVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_photon_persistent_disk_volume_source import IoK8sApiCoreV1PhotonPersistentDiskVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_portworx_volume_source import IoK8sApiCoreV1PortworxVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_projected_volume_source import IoK8sApiCoreV1ProjectedVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_quobyte_volume_source import IoK8sApiCoreV1QuobyteVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_rbd_volume_source import IoK8sApiCoreV1RBDVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_scale_io_volume_source import IoK8sApiCoreV1ScaleIOVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_secret_volume_source import IoK8sApiCoreV1SecretVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_storage_os_volume_source import IoK8sApiCoreV1StorageOSVolumeSource  # noqa: F401,E501
from kfserving.sdk.models.io_k8s_api_core_v1_vsphere_virtual_disk_volume_source import IoK8sApiCoreV1VsphereVirtualDiskVolumeSource  # noqa: F401,E501


class IoK8sApiCoreV1VolumeSource(object):
    """NOTE: This class is auto generated by the swagger code generator program.

    Do not edit the class manually.
    """

    """
    Attributes:
      swagger_types (dict): The key is attribute name
                            and the value is attribute type.
      attribute_map (dict): The key is attribute name
                            and the value is json key in definition.
    """
    swagger_types = {
        'aws_elastic_block_store': 'IoK8sApiCoreV1AWSElasticBlockStoreVolumeSource',
        'azure_disk': 'IoK8sApiCoreV1AzureDiskVolumeSource',
        'azure_file': 'IoK8sApiCoreV1AzureFileVolumeSource',
        'cephfs': 'IoK8sApiCoreV1CephFSVolumeSource',
        'cinder': 'IoK8sApiCoreV1CinderVolumeSource',
        'config_map': 'IoK8sApiCoreV1ConfigMapVolumeSource',
        'downward_api': 'IoK8sApiCoreV1DownwardAPIVolumeSource',
        'empty_dir': 'IoK8sApiCoreV1EmptyDirVolumeSource',
        'fc': 'IoK8sApiCoreV1FCVolumeSource',
        'flex_volume': 'IoK8sApiCoreV1FlexVolumeSource',
        'flocker': 'IoK8sApiCoreV1FlockerVolumeSource',
        'gce_persistent_disk': 'IoK8sApiCoreV1GCEPersistentDiskVolumeSource',
        'git_repo': 'IoK8sApiCoreV1GitRepoVolumeSource',
        'glusterfs': 'IoK8sApiCoreV1GlusterfsVolumeSource',
        'host_path': 'IoK8sApiCoreV1HostPathVolumeSource',
        'iscsi': 'IoK8sApiCoreV1ISCSIVolumeSource',
        'nfs': 'IoK8sApiCoreV1NFSVolumeSource',
        'persistent_volume_claim': 'IoK8sApiCoreV1PersistentVolumeClaimVolumeSource',
        'photon_persistent_disk': 'IoK8sApiCoreV1PhotonPersistentDiskVolumeSource',
        'portworx_volume': 'IoK8sApiCoreV1PortworxVolumeSource',
        'projected': 'IoK8sApiCoreV1ProjectedVolumeSource',
        'quobyte': 'IoK8sApiCoreV1QuobyteVolumeSource',
        'rbd': 'IoK8sApiCoreV1RBDVolumeSource',
        'scale_io': 'IoK8sApiCoreV1ScaleIOVolumeSource',
        'secret': 'IoK8sApiCoreV1SecretVolumeSource',
        'storageos': 'IoK8sApiCoreV1StorageOSVolumeSource',
        'vsphere_volume': 'IoK8sApiCoreV1VsphereVirtualDiskVolumeSource'
    }

    attribute_map = {
        'aws_elastic_block_store': 'awsElasticBlockStore',
        'azure_disk': 'azureDisk',
        'azure_file': 'azureFile',
        'cephfs': 'cephfs',
        'cinder': 'cinder',
        'config_map': 'configMap',
        'downward_api': 'downwardAPI',
        'empty_dir': 'emptyDir',
        'fc': 'fc',
        'flex_volume': 'flexVolume',
        'flocker': 'flocker',
        'gce_persistent_disk': 'gcePersistentDisk',
        'git_repo': 'gitRepo',
        'glusterfs': 'glusterfs',
        'host_path': 'hostPath',
        'iscsi': 'iscsi',
        'nfs': 'nfs',
        'persistent_volume_claim': 'persistentVolumeClaim',
        'photon_persistent_disk': 'photonPersistentDisk',
        'portworx_volume': 'portworxVolume',
        'projected': 'projected',
        'quobyte': 'quobyte',
        'rbd': 'rbd',
        'scale_io': 'scaleIO',
        'secret': 'secret',
        'storageos': 'storageos',
        'vsphere_volume': 'vsphereVolume'
    }

    def __init__(self, aws_elastic_block_store=None, azure_disk=None, azure_file=None, cephfs=None, cinder=None, config_map=None, downward_api=None, empty_dir=None, fc=None, flex_volume=None, flocker=None, gce_persistent_disk=None, git_repo=None, glusterfs=None, host_path=None, iscsi=None, nfs=None, persistent_volume_claim=None, photon_persistent_disk=None, portworx_volume=None, projected=None, quobyte=None, rbd=None, scale_io=None, secret=None, storageos=None, vsphere_volume=None):  # noqa: E501
        """IoK8sApiCoreV1VolumeSource - a model defined in Swagger"""  # noqa: E501

        self._aws_elastic_block_store = None
        self._azure_disk = None
        self._azure_file = None
        self._cephfs = None
        self._cinder = None
        self._config_map = None
        self._downward_api = None
        self._empty_dir = None
        self._fc = None
        self._flex_volume = None
        self._flocker = None
        self._gce_persistent_disk = None
        self._git_repo = None
        self._glusterfs = None
        self._host_path = None
        self._iscsi = None
        self._nfs = None
        self._persistent_volume_claim = None
        self._photon_persistent_disk = None
        self._portworx_volume = None
        self._projected = None
        self._quobyte = None
        self._rbd = None
        self._scale_io = None
        self._secret = None
        self._storageos = None
        self._vsphere_volume = None
        self.discriminator = None

        if aws_elastic_block_store is not None:
            self.aws_elastic_block_store = aws_elastic_block_store
        if azure_disk is not None:
            self.azure_disk = azure_disk
        if azure_file is not None:
            self.azure_file = azure_file
        if cephfs is not None:
            self.cephfs = cephfs
        if cinder is not None:
            self.cinder = cinder
        if config_map is not None:
            self.config_map = config_map
        if downward_api is not None:
            self.downward_api = downward_api
        if empty_dir is not None:
            self.empty_dir = empty_dir
        if fc is not None:
            self.fc = fc
        if flex_volume is not None:
            self.flex_volume = flex_volume
        if flocker is not None:
            self.flocker = flocker
        if gce_persistent_disk is not None:
            self.gce_persistent_disk = gce_persistent_disk
        if git_repo is not None:
            self.git_repo = git_repo
        if glusterfs is not None:
            self.glusterfs = glusterfs
        if host_path is not None:
            self.host_path = host_path
        if iscsi is not None:
            self.iscsi = iscsi
        if nfs is not None:
            self.nfs = nfs
        if persistent_volume_claim is not None:
            self.persistent_volume_claim = persistent_volume_claim
        if photon_persistent_disk is not None:
            self.photon_persistent_disk = photon_persistent_disk
        if portworx_volume is not None:
            self.portworx_volume = portworx_volume
        if projected is not None:
            self.projected = projected
        if quobyte is not None:
            self.quobyte = quobyte
        if rbd is not None:
            self.rbd = rbd
        if scale_io is not None:
            self.scale_io = scale_io
        if secret is not None:
            self.secret = secret
        if storageos is not None:
            self.storageos = storageos
        if vsphere_volume is not None:
            self.vsphere_volume = vsphere_volume

    @property
    def aws_elastic_block_store(self):
        """Gets the aws_elastic_block_store of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        AWSElasticBlockStore represents an AWS Disk resource that is attached to a kubelet's host machine and then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore  # noqa: E501

        :return: The aws_elastic_block_store of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1AWSElasticBlockStoreVolumeSource
        """
        return self._aws_elastic_block_store

    @aws_elastic_block_store.setter
    def aws_elastic_block_store(self, aws_elastic_block_store):
        """Sets the aws_elastic_block_store of this IoK8sApiCoreV1VolumeSource.

        AWSElasticBlockStore represents an AWS Disk resource that is attached to a kubelet's host machine and then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#awselasticblockstore  # noqa: E501

        :param aws_elastic_block_store: The aws_elastic_block_store of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1AWSElasticBlockStoreVolumeSource
        """

        self._aws_elastic_block_store = aws_elastic_block_store

    @property
    def azure_disk(self):
        """Gets the azure_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        AzureDisk represents an Azure Data Disk mount on the host and bind mount to the pod.  # noqa: E501

        :return: The azure_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1AzureDiskVolumeSource
        """
        return self._azure_disk

    @azure_disk.setter
    def azure_disk(self, azure_disk):
        """Sets the azure_disk of this IoK8sApiCoreV1VolumeSource.

        AzureDisk represents an Azure Data Disk mount on the host and bind mount to the pod.  # noqa: E501

        :param azure_disk: The azure_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1AzureDiskVolumeSource
        """

        self._azure_disk = azure_disk

    @property
    def azure_file(self):
        """Gets the azure_file of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        AzureFile represents an Azure File Service mount on the host and bind mount to the pod.  # noqa: E501

        :return: The azure_file of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1AzureFileVolumeSource
        """
        return self._azure_file

    @azure_file.setter
    def azure_file(self, azure_file):
        """Sets the azure_file of this IoK8sApiCoreV1VolumeSource.

        AzureFile represents an Azure File Service mount on the host and bind mount to the pod.  # noqa: E501

        :param azure_file: The azure_file of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1AzureFileVolumeSource
        """

        self._azure_file = azure_file

    @property
    def cephfs(self):
        """Gets the cephfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        CephFS represents a Ceph FS mount on the host that shares a pod's lifetime  # noqa: E501

        :return: The cephfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1CephFSVolumeSource
        """
        return self._cephfs

    @cephfs.setter
    def cephfs(self, cephfs):
        """Sets the cephfs of this IoK8sApiCoreV1VolumeSource.

        CephFS represents a Ceph FS mount on the host that shares a pod's lifetime  # noqa: E501

        :param cephfs: The cephfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1CephFSVolumeSource
        """

        self._cephfs = cephfs

    @property
    def cinder(self):
        """Gets the cinder of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        Cinder represents a cinder volume attached and mounted on kubelets host machine More info: https://releases.k8s.io/HEAD/examples/mysql-cinder-pd/README.md  # noqa: E501

        :return: The cinder of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1CinderVolumeSource
        """
        return self._cinder

    @cinder.setter
    def cinder(self, cinder):
        """Sets the cinder of this IoK8sApiCoreV1VolumeSource.

        Cinder represents a cinder volume attached and mounted on kubelets host machine More info: https://releases.k8s.io/HEAD/examples/mysql-cinder-pd/README.md  # noqa: E501

        :param cinder: The cinder of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1CinderVolumeSource
        """

        self._cinder = cinder

    @property
    def config_map(self):
        """Gets the config_map of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        ConfigMap represents a configMap that should populate this volume  # noqa: E501

        :return: The config_map of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1ConfigMapVolumeSource
        """
        return self._config_map

    @config_map.setter
    def config_map(self, config_map):
        """Sets the config_map of this IoK8sApiCoreV1VolumeSource.

        ConfigMap represents a configMap that should populate this volume  # noqa: E501

        :param config_map: The config_map of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1ConfigMapVolumeSource
        """

        self._config_map = config_map

    @property
    def downward_api(self):
        """Gets the downward_api of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        DownwardAPI represents downward API about the pod that should populate this volume  # noqa: E501

        :return: The downward_api of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1DownwardAPIVolumeSource
        """
        return self._downward_api

    @downward_api.setter
    def downward_api(self, downward_api):
        """Sets the downward_api of this IoK8sApiCoreV1VolumeSource.

        DownwardAPI represents downward API about the pod that should populate this volume  # noqa: E501

        :param downward_api: The downward_api of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1DownwardAPIVolumeSource
        """

        self._downward_api = downward_api

    @property
    def empty_dir(self):
        """Gets the empty_dir of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        EmptyDir represents a temporary directory that shares a pod's lifetime. More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir  # noqa: E501

        :return: The empty_dir of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1EmptyDirVolumeSource
        """
        return self._empty_dir

    @empty_dir.setter
    def empty_dir(self, empty_dir):
        """Sets the empty_dir of this IoK8sApiCoreV1VolumeSource.

        EmptyDir represents a temporary directory that shares a pod's lifetime. More info: https://kubernetes.io/docs/concepts/storage/volumes#emptydir  # noqa: E501

        :param empty_dir: The empty_dir of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1EmptyDirVolumeSource
        """

        self._empty_dir = empty_dir

    @property
    def fc(self):
        """Gets the fc of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        FC represents a Fibre Channel resource that is attached to a kubelet's host machine and then exposed to the pod.  # noqa: E501

        :return: The fc of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1FCVolumeSource
        """
        return self._fc

    @fc.setter
    def fc(self, fc):
        """Sets the fc of this IoK8sApiCoreV1VolumeSource.

        FC represents a Fibre Channel resource that is attached to a kubelet's host machine and then exposed to the pod.  # noqa: E501

        :param fc: The fc of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1FCVolumeSource
        """

        self._fc = fc

    @property
    def flex_volume(self):
        """Gets the flex_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        FlexVolume represents a generic volume resource that is provisioned/attached using an exec based plugin.  # noqa: E501

        :return: The flex_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1FlexVolumeSource
        """
        return self._flex_volume

    @flex_volume.setter
    def flex_volume(self, flex_volume):
        """Sets the flex_volume of this IoK8sApiCoreV1VolumeSource.

        FlexVolume represents a generic volume resource that is provisioned/attached using an exec based plugin.  # noqa: E501

        :param flex_volume: The flex_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1FlexVolumeSource
        """

        self._flex_volume = flex_volume

    @property
    def flocker(self):
        """Gets the flocker of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        Flocker represents a Flocker volume attached to a kubelet's host machine. This depends on the Flocker control service being running  # noqa: E501

        :return: The flocker of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1FlockerVolumeSource
        """
        return self._flocker

    @flocker.setter
    def flocker(self, flocker):
        """Sets the flocker of this IoK8sApiCoreV1VolumeSource.

        Flocker represents a Flocker volume attached to a kubelet's host machine. This depends on the Flocker control service being running  # noqa: E501

        :param flocker: The flocker of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1FlockerVolumeSource
        """

        self._flocker = flocker

    @property
    def gce_persistent_disk(self):
        """Gets the gce_persistent_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        GCEPersistentDisk represents a GCE Disk resource that is attached to a kubelet's host machine and then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk  # noqa: E501

        :return: The gce_persistent_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1GCEPersistentDiskVolumeSource
        """
        return self._gce_persistent_disk

    @gce_persistent_disk.setter
    def gce_persistent_disk(self, gce_persistent_disk):
        """Sets the gce_persistent_disk of this IoK8sApiCoreV1VolumeSource.

        GCEPersistentDisk represents a GCE Disk resource that is attached to a kubelet's host machine and then exposed to the pod. More info: https://kubernetes.io/docs/concepts/storage/volumes#gcepersistentdisk  # noqa: E501

        :param gce_persistent_disk: The gce_persistent_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1GCEPersistentDiskVolumeSource
        """

        self._gce_persistent_disk = gce_persistent_disk

    @property
    def git_repo(self):
        """Gets the git_repo of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        GitRepo represents a git repository at a particular revision. DEPRECATED: GitRepo is deprecated. To provision a container with a git repo, mount an EmptyDir into an InitContainer that clones the repo using git, then mount the EmptyDir into the Pod's container.  # noqa: E501

        :return: The git_repo of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1GitRepoVolumeSource
        """
        return self._git_repo

    @git_repo.setter
    def git_repo(self, git_repo):
        """Sets the git_repo of this IoK8sApiCoreV1VolumeSource.

        GitRepo represents a git repository at a particular revision. DEPRECATED: GitRepo is deprecated. To provision a container with a git repo, mount an EmptyDir into an InitContainer that clones the repo using git, then mount the EmptyDir into the Pod's container.  # noqa: E501

        :param git_repo: The git_repo of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1GitRepoVolumeSource
        """

        self._git_repo = git_repo

    @property
    def glusterfs(self):
        """Gets the glusterfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        Glusterfs represents a Glusterfs mount on the host that shares a pod's lifetime. More info: https://releases.k8s.io/HEAD/examples/volumes/glusterfs/README.md  # noqa: E501

        :return: The glusterfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1GlusterfsVolumeSource
        """
        return self._glusterfs

    @glusterfs.setter
    def glusterfs(self, glusterfs):
        """Sets the glusterfs of this IoK8sApiCoreV1VolumeSource.

        Glusterfs represents a Glusterfs mount on the host that shares a pod's lifetime. More info: https://releases.k8s.io/HEAD/examples/volumes/glusterfs/README.md  # noqa: E501

        :param glusterfs: The glusterfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1GlusterfsVolumeSource
        """

        self._glusterfs = glusterfs

    @property
    def host_path(self):
        """Gets the host_path of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        HostPath represents a pre-existing file or directory on the host machine that is directly exposed to the container. This is generally used for system agents or other privileged things that are allowed to see the host machine. Most containers will NOT need this. More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath  # noqa: E501

        :return: The host_path of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1HostPathVolumeSource
        """
        return self._host_path

    @host_path.setter
    def host_path(self, host_path):
        """Sets the host_path of this IoK8sApiCoreV1VolumeSource.

        HostPath represents a pre-existing file or directory on the host machine that is directly exposed to the container. This is generally used for system agents or other privileged things that are allowed to see the host machine. Most containers will NOT need this. More info: https://kubernetes.io/docs/concepts/storage/volumes#hostpath  # noqa: E501

        :param host_path: The host_path of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1HostPathVolumeSource
        """

        self._host_path = host_path

    @property
    def iscsi(self):
        """Gets the iscsi of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        ISCSI represents an ISCSI Disk resource that is attached to a kubelet's host machine and then exposed to the pod. More info: https://releases.k8s.io/HEAD/examples/volumes/iscsi/README.md  # noqa: E501

        :return: The iscsi of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1ISCSIVolumeSource
        """
        return self._iscsi

    @iscsi.setter
    def iscsi(self, iscsi):
        """Sets the iscsi of this IoK8sApiCoreV1VolumeSource.

        ISCSI represents an ISCSI Disk resource that is attached to a kubelet's host machine and then exposed to the pod. More info: https://releases.k8s.io/HEAD/examples/volumes/iscsi/README.md  # noqa: E501

        :param iscsi: The iscsi of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1ISCSIVolumeSource
        """

        self._iscsi = iscsi

    @property
    def nfs(self):
        """Gets the nfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        NFS represents an NFS mount on the host that shares a pod's lifetime More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs  # noqa: E501

        :return: The nfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1NFSVolumeSource
        """
        return self._nfs

    @nfs.setter
    def nfs(self, nfs):
        """Sets the nfs of this IoK8sApiCoreV1VolumeSource.

        NFS represents an NFS mount on the host that shares a pod's lifetime More info: https://kubernetes.io/docs/concepts/storage/volumes#nfs  # noqa: E501

        :param nfs: The nfs of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1NFSVolumeSource
        """

        self._nfs = nfs

    @property
    def persistent_volume_claim(self):
        """Gets the persistent_volume_claim of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        PersistentVolumeClaimVolumeSource represents a reference to a PersistentVolumeClaim in the same namespace. More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims  # noqa: E501

        :return: The persistent_volume_claim of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1PersistentVolumeClaimVolumeSource
        """
        return self._persistent_volume_claim

    @persistent_volume_claim.setter
    def persistent_volume_claim(self, persistent_volume_claim):
        """Sets the persistent_volume_claim of this IoK8sApiCoreV1VolumeSource.

        PersistentVolumeClaimVolumeSource represents a reference to a PersistentVolumeClaim in the same namespace. More info: https://kubernetes.io/docs/concepts/storage/persistent-volumes#persistentvolumeclaims  # noqa: E501

        :param persistent_volume_claim: The persistent_volume_claim of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1PersistentVolumeClaimVolumeSource
        """

        self._persistent_volume_claim = persistent_volume_claim

    @property
    def photon_persistent_disk(self):
        """Gets the photon_persistent_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        PhotonPersistentDisk represents a PhotonController persistent disk attached and mounted on kubelets host machine  # noqa: E501

        :return: The photon_persistent_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1PhotonPersistentDiskVolumeSource
        """
        return self._photon_persistent_disk

    @photon_persistent_disk.setter
    def photon_persistent_disk(self, photon_persistent_disk):
        """Sets the photon_persistent_disk of this IoK8sApiCoreV1VolumeSource.

        PhotonPersistentDisk represents a PhotonController persistent disk attached and mounted on kubelets host machine  # noqa: E501

        :param photon_persistent_disk: The photon_persistent_disk of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1PhotonPersistentDiskVolumeSource
        """

        self._photon_persistent_disk = photon_persistent_disk

    @property
    def portworx_volume(self):
        """Gets the portworx_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        PortworxVolume represents a portworx volume attached and mounted on kubelets host machine  # noqa: E501

        :return: The portworx_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1PortworxVolumeSource
        """
        return self._portworx_volume

    @portworx_volume.setter
    def portworx_volume(self, portworx_volume):
        """Sets the portworx_volume of this IoK8sApiCoreV1VolumeSource.

        PortworxVolume represents a portworx volume attached and mounted on kubelets host machine  # noqa: E501

        :param portworx_volume: The portworx_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1PortworxVolumeSource
        """

        self._portworx_volume = portworx_volume

    @property
    def projected(self):
        """Gets the projected of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        Items for all in one resources secrets, configmaps, and downward API  # noqa: E501

        :return: The projected of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1ProjectedVolumeSource
        """
        return self._projected

    @projected.setter
    def projected(self, projected):
        """Sets the projected of this IoK8sApiCoreV1VolumeSource.

        Items for all in one resources secrets, configmaps, and downward API  # noqa: E501

        :param projected: The projected of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1ProjectedVolumeSource
        """

        self._projected = projected

    @property
    def quobyte(self):
        """Gets the quobyte of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        Quobyte represents a Quobyte mount on the host that shares a pod's lifetime  # noqa: E501

        :return: The quobyte of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1QuobyteVolumeSource
        """
        return self._quobyte

    @quobyte.setter
    def quobyte(self, quobyte):
        """Sets the quobyte of this IoK8sApiCoreV1VolumeSource.

        Quobyte represents a Quobyte mount on the host that shares a pod's lifetime  # noqa: E501

        :param quobyte: The quobyte of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1QuobyteVolumeSource
        """

        self._quobyte = quobyte

    @property
    def rbd(self):
        """Gets the rbd of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        RBD represents a Rados Block Device mount on the host that shares a pod's lifetime. More info: https://releases.k8s.io/HEAD/examples/volumes/rbd/README.md  # noqa: E501

        :return: The rbd of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1RBDVolumeSource
        """
        return self._rbd

    @rbd.setter
    def rbd(self, rbd):
        """Sets the rbd of this IoK8sApiCoreV1VolumeSource.

        RBD represents a Rados Block Device mount on the host that shares a pod's lifetime. More info: https://releases.k8s.io/HEAD/examples/volumes/rbd/README.md  # noqa: E501

        :param rbd: The rbd of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1RBDVolumeSource
        """

        self._rbd = rbd

    @property
    def scale_io(self):
        """Gets the scale_io of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        ScaleIO represents a ScaleIO persistent volume attached and mounted on Kubernetes nodes.  # noqa: E501

        :return: The scale_io of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1ScaleIOVolumeSource
        """
        return self._scale_io

    @scale_io.setter
    def scale_io(self, scale_io):
        """Sets the scale_io of this IoK8sApiCoreV1VolumeSource.

        ScaleIO represents a ScaleIO persistent volume attached and mounted on Kubernetes nodes.  # noqa: E501

        :param scale_io: The scale_io of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1ScaleIOVolumeSource
        """

        self._scale_io = scale_io

    @property
    def secret(self):
        """Gets the secret of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        Secret represents a secret that should populate this volume. More info: https://kubernetes.io/docs/concepts/storage/volumes#secret  # noqa: E501

        :return: The secret of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1SecretVolumeSource
        """
        return self._secret

    @secret.setter
    def secret(self, secret):
        """Sets the secret of this IoK8sApiCoreV1VolumeSource.

        Secret represents a secret that should populate this volume. More info: https://kubernetes.io/docs/concepts/storage/volumes#secret  # noqa: E501

        :param secret: The secret of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1SecretVolumeSource
        """

        self._secret = secret

    @property
    def storageos(self):
        """Gets the storageos of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        StorageOS represents a StorageOS volume attached and mounted on Kubernetes nodes.  # noqa: E501

        :return: The storageos of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1StorageOSVolumeSource
        """
        return self._storageos

    @storageos.setter
    def storageos(self, storageos):
        """Sets the storageos of this IoK8sApiCoreV1VolumeSource.

        StorageOS represents a StorageOS volume attached and mounted on Kubernetes nodes.  # noqa: E501

        :param storageos: The storageos of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1StorageOSVolumeSource
        """

        self._storageos = storageos

    @property
    def vsphere_volume(self):
        """Gets the vsphere_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501

        VsphereVolume represents a vSphere volume attached and mounted on kubelets host machine  # noqa: E501

        :return: The vsphere_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :rtype: IoK8sApiCoreV1VsphereVirtualDiskVolumeSource
        """
        return self._vsphere_volume

    @vsphere_volume.setter
    def vsphere_volume(self, vsphere_volume):
        """Sets the vsphere_volume of this IoK8sApiCoreV1VolumeSource.

        VsphereVolume represents a vSphere volume attached and mounted on kubelets host machine  # noqa: E501

        :param vsphere_volume: The vsphere_volume of this IoK8sApiCoreV1VolumeSource.  # noqa: E501
        :type: IoK8sApiCoreV1VsphereVirtualDiskVolumeSource
        """

        self._vsphere_volume = vsphere_volume

    def to_dict(self):
        """Returns the model properties as a dict"""
        result = {}

        for attr, _ in six.iteritems(self.swagger_types):
            value = getattr(self, attr)
            if isinstance(value, list):
                result[attr] = list(map(
                    lambda x: x.to_dict() if hasattr(x, "to_dict") else x,
                    value
                ))
            elif hasattr(value, "to_dict"):
                result[attr] = value.to_dict()
            elif isinstance(value, dict):
                result[attr] = dict(map(
                    lambda item: (item[0], item[1].to_dict())
                    if hasattr(item[1], "to_dict") else item,
                    value.items()
                ))
            else:
                result[attr] = value
        if issubclass(IoK8sApiCoreV1VolumeSource, dict):
            for key, value in self.items():
                result[key] = value

        return result

    def to_str(self):
        """Returns the string representation of the model"""
        return pprint.pformat(self.to_dict())

    def __repr__(self):
        """For `print` and `pprint`"""
        return self.to_str()

    def __eq__(self, other):
        """Returns true if both objects are equal"""
        if not isinstance(other, IoK8sApiCoreV1VolumeSource):
            return False

        return self.__dict__ == other.__dict__

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        return not self == other
