---
title: '用 Pulsar 开发多人小游戏（四）：Pulsar 安装使用简介'
---

> note：本文是《用 Pulsar 开发多人在线小游戏》的第三篇，配套源码和全部文档参见我的 GitHub 仓库 [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) 以及我的文章列表。

最详尽的部署方法参见官网：

https://pulsar.apache.org/

这里我介绍下 Pulsar 的架构原理，搞明白之后就能很容易理解 Pulsar 的各种部署方式了。

### Pulsar 集群的关键组件

如下图所示，Pulsar 集群包含一个或多个 broker 节点，一个或多个 bookie 节点，以及一个或多个 zookeeper 节点：

![](https://labuladong.github.io/pictures/pulsar-game/pulsar-cluster.jpeg)

broker 节点负责计算（比如客户端的连接、数据的缓存/分块等工作），bookie 节点负责存储（数据的持久化和一致性存储），zookeeper 存储这些节点的元数据，就这么简单。

如果你想快速验证一下我们的炸弹人小游戏，可以参照官网 [Run a standalone Pulsar cluster locally](https://pulsar.apache.org/docs/next/getting-started-standalone/) 的步骤，在本地启动一个 standalone 模式的 Pulsar 集群：

```shell
./bin/pulsar standalone
```

你应该能猜到，standalone 模式其实就是把 bookie, broker, zookeeper 打包起来，帮你一次性把这些服务都启动了。

不过，最新版的 Pulsar 安装包包含了 [PIP-117](https://github.com/apache/pulsar/issues/13302) 对 standalone 模式的优化，可以避免启动 zookeeper 从而优化 standalone 模式的性能。

但是，因为我们是以学习 Pulsar 为目的，所以我建议还是要把 zookeeper 起起来，这样方便我们通过 zookeeper 的相关工具查看元数据。

我们可以通过设置 `PULSAR_STANDALONE_USE_ZOOKEEPER` 环境变量避免 PIP-117 的优化，启动 zookeeper 存储 Pulsar 集群的元数据：

```shell
PULSAR_STANDALONE_USE_ZOOKEEPER=1 ./bin/pulsar standalone
```

### Pulsar 集群的配置文件

上面说到，Pulsar 集群是由 broker 节点、bookie 节点、zookeeper 节点共同组成的，所以每一种节点都有自己的配置文件。

broker 节点的配置文件是 `conf/broker.conf`，bookie 节点的配置文件是 `conf/bookkeeper.conf`，zookeeper 节点的配置文件是 `conf/zookeeper.conf`。

standalone 模式的 Pulsar 有它自己的配置文件 `conf/standalone.conf`。因为 standalone 模式相当于一次性启动多种节点，所以可以理解为 `conf/standalone.conf` 就是把上述三个配置文件结合起来了。

配置文件中有这么多配置字段，都是什么意思呢？这里我们暂且不提，等到后续游戏开发中我们遇到具体的问题时再回头来看。