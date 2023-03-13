---
title: '用 Pulsar 开发多人小游戏（七）：用 Pulsar Connector 制作全局计分板'
---

> note：本文是《用 Pulsar 开发多人在线小游戏》的第三篇，配套源码和全部文档参见我的 GitHub 仓库 [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) 以及我的文章列表。

上一章介绍了 Pulsar Function 的使用，每个游戏房间都有对应的 score topic，每个玩家的得分都会被发送到 score topic 中，游戏客户端通过读取这个 topic 的 tableView 来显示局内计分板。

但考虑到一个玩家可能去过多个游戏房间进行游戏，所以我们希望能够对玩家在不同房间中的总分做一个统计，实现类似「全服排行榜」的功能。

想要实现这个全局计分板，简单直接的做法就是创建 Consumer 消费所有 `{roomName}-score-topic` 中的消息，对每个玩家进行总分统计。不过这样搞比较麻烦，每次计算全局分数都要重新计算一次。

毕竟 Pulsar 不是一个主打数据聚合和统计的系统，所以我会考虑把用户的分数信息导出到其他数据系统中，做进一步的分析和统计。

因为我们发送到 score Topic 中的消息其实是键值对，键是 `playerName-roomName` 的形式，值是该玩家在该房间中获得的最新分数。

那么如果我们可以把 `{roomName}-score-topic` 中的消息存储到 Redis 中，就可以用 Redis 的 `playerName-*` 的形式拿到某个玩家在所有房间内的分数信息。然后对它们求和即可。

**本文就以 Redis 举例，看看如何通过 Pulsar connector 功能将 Pulsar 中的数据自动导出到其他系统**。

### Pulsar connector 简介

先上官网文档链接：

https://pulsar.apache.org/docs/next/io-overview/

connector 分两种，一种叫 source，另一种叫 sink。顾名思义，source 就是把数据从其他系统导入 Pulsar 中，sink 就是把 Pulsar 中的数据导入到其他系统中。

其实我理解 Pulsar connector 本质就是 Pulsar Function，这个 Function 持有了其他数据系统的客户端，作为 Pulsar 和其他系统之间的桥梁罢了。

之前介绍 Pulsar Function 时说过，Function 有两种部署形式，可以独立部署为一个服务集群，也可以上传部署到 broker 上，所以 source 和 sink 也可以独立部署或者上传部署到 broker 上。稍后会看到，我会用 `localrun` 在本地启动一个 Redis sink connector，也就是独立部署的形式。

我们想把 topic 中的数据导入 Redis，所以需要在 [Pulsar build-in connector](https://pulsar.apache.org/download/) 的下载页面寻找 Redis sink connector，下载之后按照 [Redis sink 的配置文档](https://pulsar.apache.org/docs/next/io-redis-sink/#configuration) 操作即可。

首先在 Pulsar 的文件夹中创建一个 `connectors` 目录存储所有 Pulsar connector，并下载 Redis sink connector 到这个目录：

```bash
mkdir connectors
curl -O https://www.apache.org/dyn/mirrors/mirrors.cgi?action=download&filename=pulsar/pulsar-2.11.0/connectors/pulsar-io-redis-2.11.0.nar --output-dir ./connectors
```

### 部署 Redis Sink

我们先在本地用 docker 启动一个 Redis 服务器：

```bash
docker pull redis:5.0.5
docker run -d -p 6379:6379 --name my-redis redis:5.0.5 --requirepass "mypassword"
```

我们想把 Pulsar 里面的数据导入到 Redis 里面，所以得给出这个 Redis 服务的必要信息对吧，所以建立一个 `redis-sink-config.yml` 文件：

```yml
# Redis config
configs:
    redisHosts: "localhost:6379"
    redisPassword: "mypassword"
    redisDatabase: 0
    clientMode: "Standalone"
    operationTimeout: 2000
    batchSize: 1
    batchTimeMs: 1000
    connectTimeout: 3000
```

然后，我们用 `localrun` 模式在本地启动一个 Pulsar Connect 服务，让它运行 Redis 的 connector，并且要告诉它要从哪个 topic 读数据存储到 Redis 中：

```bash
bin/pulsar-admin sinks localrun \
    --archive connectors/pulsar-io-redis-2.11.0.nar \
    --tenant public \
    --namespace default \
    --name my-redis-sink \
    --sink-config-file redis-sink-config.yaml \
    --topics-pattern '*-score-topic'
```

这个命令在本地启动了 Redis sink connector，并且指定读取所有后缀为 `-score-topic` 的 topic 数据，存储到 Redis 中。

接下来，就可以通过以下 Redis 命令查询某个玩家的总分了：

```bash
eval "local keys = redis.call('keys',KEYS[1]) ; local sum=0 ; for _,k in ipairs(keys) do sum = sum + tonumber(redis.call('get',k)) end ; return sum" 1 'playName-*'
```