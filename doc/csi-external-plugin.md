  
## external-provisioner
- 作用: 提供动态分配pv方案, storageclass 的控制器需要由存储提供方监听和创建  
- 地址: https://github.com/kubernetes-csi/external-provisioner
    - 第三方库
    - sigs.k8s.io/sig-storage-lib-external-provisioner/v6/controller  
    - 选项有csi sock文件指定; kubeconfig文件; 
- 所需要使用的lister
    - storageclass
    - pvc
    - volumeattacher (存储需要attach/detach功能)
    - node/csinode (节点不对等时)
    - snapshot csi快照功能
- 控制器:
    - csiProvisioner
    - provisionController 
        - 主要用于同步volumeattacher
    - capacityController: 
        - 当启动选项"capacity-controller-deployment-mode = central"时打开
        - 周期性调用csi controller的GetCapacity同步容量        

### provisionController
- sigs.k8s.io/sig-storage-lib-external-provisioner库
- 处理pvc,pv,storageclass 三种资源, 内部调用的 provisioner,需完成下面两个接口
    - Provision(context.Context, ProvisionOptions) (*v1.PersistentVolume, ProvisioningState, error)
        - 创建pv, 如果pv状态是ProvisioningInBackground, 会
    - Delete(context.Context, *v1.PersistentVolume) error
- pvc 处理
    - 同步claim
        - 生成pv名称(pvc-{uuid})
        - 获取pvcRef
        - 检查Provision是否支持挂载(主要测试block挂载方式)
        - 检查 sc 和当前 provisioner 名称是否一致
        - 检查pvc annotations中node key在不在,用于指定特定节点
        - 调用provisioner.Provision()
    - 检测状态是否为ProvisioningFinished / ProvisioningInBackground
    - 同步数据 claimsInProgress
- pv 处理
    - 检测是否需要删除   
    - 调用 provisioner.Delete
    
### csiProvisioner
- 有关启动选项
    - --strict-topology: 默认false, 将节点通过label过滤,将计算得到required,prefer 传递给driver,当immediate-topology为true,会被忽略
    - --immediate-topology: 默认true, 由csi driver自己选择节点
    - --extra-create-metadata: 默认false, 将pvc,pv信息作为参数给driver
    - Topology(feature): 默认false   

### Provision
- 检查pvc annotation,是否已经指定了storage-provisor
- 检查并尝试更新topology
- 更新secret,params 等
- 调用CreateVolume接口


 