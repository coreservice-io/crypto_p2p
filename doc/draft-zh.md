# 功能
## 1. 网络建立与维护
peer发现、peer地址池管理、peer连接建立与管理、peer之间通信
## 2. 网络上分发数据
数据性质定义(用途、来源、格式)、索引方式、校验、存储、网络传输

# 网络建立、维护
大致步骤：
- ● 维护dns域名、种子节点(初次启动)
- ● send version信息：版本信息、区块、时间
- ● recv  version信息
- ● 建立连接：互发verack信息( (握手流程：tcp -> join -> close -> reconnect))
- ● send getadd/addr收集更多节点信息(启动后)
- ● 新连接直到上限inbound 125，outbound 8、长连接ping/pong检测心跳包
- ● 节点信息ip:port落盘

## 1. peer发现
dns seeds 查询获取初始节点地址

节点保存本地，直连
## 2. peer连接建立与管理
监听端口，接收与发起连接

交换peer信息，握手(tracker/dns/地址交换)

通信分发数据

检查超时及状态异常，去旧连新
## 3. peer地址池管理：list or table  (eth为dht定位kad网络维护节点路由)
几类地址均加入地址池

连接管理模块查询选择

记录peer通信记录，问题peer  (黑名单、长连接、局域网穿透nat端口映射)

# 网络分发数据
交易数据：涉及资产安全、去中心化无序

block/tx 二级索引、hash链表

同步：blockHeader网络传播、blockIndex节点内索引(内存)

header+index请求peers，以hash索引下载，校验双花及交易签名，记录数据库

节点交互
- ● 心跳：ping/pong
- ● 请求：getaddr获取对端可用节点列表、inv数据传输
- ● 同步：header/block
