# 用 connector 制作全局计分板

上一章介绍了 Pulsar Function 的使用，每个游戏房间都有对应的 score topic，每个玩家的得分都会被发送到 score topic 中，游戏客户端通过读取这个 topic 的 tableView 来显示局内计分板。

但考虑到一个玩家可能去过多个游戏房间进行游戏，所以我们希望能够对玩家在不同房间中的总分做一个统计，实现类似「全服排行榜」的功能。

想要实现这个全局计分板，简单直接的做法就是创建 Consumer 消费所有 `{roomName}-score-topic` 中的消息，对每个玩家进行总分统计。不过这样搞比较麻烦，每次计算全局分数都要重新计算一次。

毕竟 Pulsar 不是一个主打数据聚合和统计的系统，所以我会考虑把用户的分数信息导出到其他数据系统中，做进一步的分析和统计。

**本文就以 Redis 举例，看看如何通过 Pulsar connector 功能将 Pulsar 中的数据自动导出到其他系统**。

### Pulsar connector 简介

先上官网文档链接：

https://pulsar.apache.org/docs/next/io-overview/

connector 分两种，一种叫 source，另一种叫 sink。顾名思义，source 就是把数据从其他系统导入 Pulsar 中，sink 就是把 Pulsar 中的数据导入到其他系统中。

其实我理解 Pulsar connector 本质就是 Pulsar Function，这个 Function 持有了其他数据系统的客户端，作为 Pulsar 和其他系统之间的桥梁罢了。

我们想把 topic 中的数据导入 Redis，所以需要在 [Pulsar build-in connector](https://pulsar.apache.org/download/) 的下载页面寻找 Redis sink connector，下载之后按照 [Redis sink 的配置文档](https://pulsar.apache.org/docs/next/io-redis-sink/#configuration) 操作即可。

### 计分原理

因为我们发送到 score Topic 中的消息是 `playerName-roomName` 的形式，就可以用 Redis 的 `playerName-*` 的形式拿到某个玩家在所有房间内的分数信息。然后对它们求和即可。