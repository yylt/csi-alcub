## csi 
#### 背景
- 存储目前是intree模型, k8s更新才能带来存储的更新
- 存储模型: pv - pvc 
    - pvc 用户声明的存储
    - pv 实际存储, k8s会尽力匹配pvc和pv, 其控制器在controller-manager中 
    
- 动态供应: storageClass, flexVolume
- 容器编排不止 k8s, 还有其他的编排系统,如cloudfoundry
    
#### csi特点
- out-tree
    - 与CO(容器编排)完全解耦
    - SP负责插件设计，开发和测试
- service vs. cli 
    - 相比cli，service更易部署
    - 部署cli需主机root权限
    - 基于FUSE（用户空间文件系统）后端需长时间运行    
    
## kubernets 

#### volume管理
- 异步模型
    - 生产者: 同步当前资源的配置到数据库中, 资源包括实际的yaml和节点实际挂载volume
    - 消费者: 从数据库中获取预期的配置 对比 当前实际配置, 执行attacher/mounter行为

- 主要数据接口是Attacher/Detacher,Mounter/Umounter, BlockMapper/BlockUnmapper
- 每个volume 必须支持mounter,可以选择支持attacher, 
    - attacher主要负责绑定节点, 
    - mounter负责挂载volume 到指定目录  

#### storageClass: 
- 用户态crd(控制器是用户提供)
- 作用: 管理storageClass和pvc的关联关系, 并动态增删pv
- 相关项目
    - github.com/kubernetes-incubator/external-storage
    - github.com/kubernetes-sigs/sig-storage-lib-external-provisioner
- 缺点
    - 只提供创建pv方式,无法控制mount/attach等操作 
    - 创建的pv只能是k8s内支持的volume

#### flexVolume
- cli 使用方式
- 提供attach/mount等操作
    
#### csi volume
- 实现Attacher/Detacher,Mounter/Umounter, BlockMapper/BlockUnmapper 这些接口
- attacher 逻辑
    - 监听pv的创建
    - 根据pv,pvc信息创建volumeattachment cr
    - 等待volumeattachment状态为成功 表明attacher成功
    
- mount 逻辑
    - 根据options获取当前注册的csinode grpc client
    - 创建req 数据结构
    - 调用NodePublishVolume接口
    
#### csi 的注册过程
- plulginManager: 
    - kubelet其中一个控制器,监听固定目录下文件(unix socket)的更新,完成csi handler 注册
- csi handler 会根据grpc 的GetInfo接口 完成以下工作
    - csi node cr更新和创建
    - node信息中的 label, csi字段更新

## csi 

#### 动态创建
- 依赖外部项目 [external-provisioner](github.com/kubernetes-csi/external-provisioner)
- 该项目功能
    - storageclass, pvc, pv, node,csinode 监听
    - storageclass,pv创建
    - 节点 topology 的检查和节点选择 (默认不开启)
        - rr调度

#### 绑定
- 依赖外部项目[external-attacher](https://github.com/kubernetes-csi/external-attacher/)

## 高性能低时延
### 参考文档
- https://wiki.easystack.cn/index.php/%E9%AB%98%E6%80%A7%E8%83%BD%E4%BD%8E%E6%97%B6%E5%BB%B6%E9%85%8D%E7%BD%AE%E5%8C%85%E7%94%B3%E8%AF%B7%E6%B5%81%E7%A8%8B
    - 高性能 对接包制作 (提供信息: 节点组, 网卡信息, master节点信息)
- drbd + escache 方案 (存储组)
    - 持久cache (nvme)
    - drbd (kernel network sync)
    - 要求是三个节点一组
    - 提供api(python), 要求在调api时,需要pool, image参数 
    
### 规划
- external-provisioner + controller 运行在master节点,实现主备,完成功能
    - 创建pool / 复用pool
    - 创建 cinder存储卷/rbd存储卷
        - 所有节点都可以访问
    - 节点选择: 可以交给external-provisioner 或者controller做
        - 组之间均匀
        - 组内均匀
    - 容量监听(not ness): 
        - 创建卷会不会失败
    - 创建与高性能服务的http 封装 (参考陈亮组)
        - flush 
        - 状态
    - csi controller rpc功能
   
- node 运行在有高性能卡的节点,主要完成功能
    - 创建与高性能服务的http 封装 (参考陈亮组)
        - dev_connect 类似 设备发现和绑定
        - flush: 刷新nvme cache到rbd中
        - stop: 原节点执行
    - 监听高性能服务的状态
    - mount/format 存储 到指定路径上
    - csi node rpc功能

### 实际场景
- controller
    - 复用pool
    - 创建rbd image使用
    - 封装高性能接口
        - dev_connect
- node
    - dev format/mount         
    
## 可靠性(todo)
- 封装的http高性能 接口
- rbd status

#### 异常场景
- 断网
    
- 关机    