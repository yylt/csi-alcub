## kubernetes 1.16.6 

### feature
- 增加以下feature,其中默认打开的有四个
    - CSIBlockVolume #支持块设备方式挂载
    - CSIDriverRegistry #注册驱动
    - CSINodeInfo #注册node信息
    - CSIInlineVolume 
```
CSIBlockVolume=true|false (BETA - default=true)
CSIDriverRegistry=true|false (BETA - default=true)
CSIInlineVolume=true|false (BETA - default=true)
CSIMigration=true|false (ALPHA - default=false)
CSIMigrationAWS=true|false (ALPHA - default=false)
CSIMigrationAzureDisk=true|false (ALPHA - default=false)
CSIMigrationAzureFile=true|false (ALPHA - default=false)
CSIMigrationGCE=true|false (ALPHA - default=false)
CSIMigrationOpenStack=true|false (ALPHA - default=false)
CSINodeInfo=true|false (BETA - default=true)
```

### crd spec
- k8s.io/api/storage/v1beta1(v1)/types.go

#### CSIDriverRegistry
- 主要有三个数据结构
- AttachRequired (bool): 要求是否attach 
- PodInfoOnMount (bool): 在调用NodePublishVolume时,将pod信息传入
- VolumeLifecycleModes (list(VolumeLifecycleMode)) 
    - Persistent(默认): 持久化
    - Ephemeral : 同pod同生命周期 

#### CSINodeSpec
- spec 是CSINodeDriver 的列表
- nodedriver.name: 节点名称
- nodedriver.NodeID: 节点id
- nodedriver.TopologyKeys: 拓扑域
- nodedriver.Allocatable: 目前只有可分配数量配置

#### VolumeAttachmentSpec
- csi绑定信息会存储在该cr中
- Attacher: 通过GetPluginName()设置
- NodeName: 应该绑定的节点
- Source.pvname : 真正绑定的数据源
- Source.inlinepvname: pod spec内设置的

## csi logic
- pkg/volume/csi/csi_plugin.go
- Init (pkg/volume/csi/csi_plugin.go:192)
    - 更新nodeinfo, 根据gce,cinder,aws存储这三个支持csimigration(待补充)
       
### attacher
- attacher 函数签名
    - Attach(spec *volume.Spec, nodeName types.NodeName) (string, error)
    - return 节点上设备路径和错误信息 
- WaitForAttach 函数签名
    - WaitForAttach(spec *volume.Spec, _ string, pod *v1.Pod, timeout time.Duration) (string, error)
    - return 节点上设备路径和错误信息 
- Attach
    - 获取pv driver,pv handler和节点名称
    - 生成attachid,通过上述信息
    - 查询pv提供方式,inline还是pv模式
    - 创建volumeattachment spec,其中name就是attachid
    - 创建volumeattachment 并等待同步
- WaitForAttach
    - 监听 volumeattachment 的status变化, 当status成功后返回成功
   
### mounter
- 需要实现两个接口
    - SetUp(mounterArgs volume.MounterArgs) 
    - SetUpAt(dir string, mounterArgs volume.MounterArgs) error  
- csi Mounter实现的Setup会继续调用SetupAt,其中dir是/var/lib/kubelet/pods/{uuid}/volume/kubernetes.io~csi/{volume-name}
- SetUpAt:
    - 检查是否已经mount
    - 获取csiCli,从数据库(csi_drivers_store)中获取,csi_drivers_store是通过pluginmanager初始化
    - 调用csi NodePublishVolume接口

## csi registry
- 借助 pluginManager 管理 , 扫描的默认目录是 
    - /var/lib/kubelet/plugins_registry
    - /var/lib/kubelet/plugins (将被弃用)  
- kubelet初始化时只增加两种pluginhandler
    - csi
    - device (GPU、NIC、FPGA、InfiniBand等设备)
    
### pluginManager
- desiredStateOfWorldPopulator(dsowp): 控制器用于监听文件变化,并更新dsow
- reconciler: 同步dsow到asow, 并根据asow来处理操作
- operationGenerator 和 operationExecutor 用于创建操作和检查操作状态

- 重点说下operationGenerator中的操作,目前只提供注册和注销两个方法创建

#### plugin 注册和注销
- (grpc)调用GetInfo方法获取respInfo信息
- 根据type类型查询handler
- 调用handler的ValidatePlugin和RegisterPlugin 方法

- csi ValidatePlugin
    - 检查版本支持与否
- csi RegisterPlugin
    - 调用cli.GetNodeInfo
    - 将nodeid 写入node annotation信息中,将拓扑写入label中
    - 更新和创建csinode cr
```
respInfo {
Type string //csi 或者device
Name string //对于csi,是CSIDriverRegistry中的name
Endpoint string //用于重分配服务地址,默认是socketPath
SupportedVersions []string
}
```
    