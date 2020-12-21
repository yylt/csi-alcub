### storageclass
- 参数翻译
- Provisioner: 字符串 提供者name
- Parameters: 用于controller在创建volume时的参数
- MountOptions: []string, 如"ro"等
- AllowVolumeExpansion: 是否支持volume扩展
- VolumeBindingMode: 绑定模式,
- AllowedTopologies: 节点扩扑,使用matchSelector方式选择节点


### pv
- 参数翻译
- Capacity: 容量
- AccessModes
- ClaimRef: pvc引用
- PersistentVolumeReclaimPolicy: "Recycle","Delete","Retain"三选一
- StorageClassName:
- MountOptions
- VolumeMode
- NodeAffinity: 节点强亲和设置
- PersistentVolumeSource: 支持目前已有的volume,包括不限于csi,hostpath等,这里在说下csi的参数
    - Driver
    - VolumeHandler
    - ReadOnly
    - FsType:   "ext4", "xfs", "ntfs"之类
    - VolumeAttributes: 卷的属性,这个主要用于自己维护的参数,类型map[string]string
    - ControllerPublishSecretRef
    - NodeStageSecretRef
    - NodePublishSecretRef
    - ControllerExpandSecretRef
    
### pvc
- 参数翻译
- AccessModes: 列表, "ReadWriteOnce","ReadOnlyMany","ReadWriteMany"
- Selector: 
- Resources: 资源配置
- VolumeName: pv名称
- StorageClassName:
- VolumeMode: 字符串, "Block" 或者 "Filesystem"
- DataSource: 比较少用. 