# Pulsar 的生产和消费

我在刚开始使用 Pulsar 时经常出现一些和预期不符的问题，我以为是 Pulsar 的 bug，实际上却是因为自己误解或忽略了某些默认配置。

所以本文介绍 Pulsar 生产者和消费者的一些底层原理，同时介绍一些最常用的配置参数。Pulsar 的生产/消费者配置虽然多，但只要了解一些底层原理，就可以很容易明白它们的作用了。

### 前置知识

**1、Pulsar 中一个完整的 topic 名长这样**：

```
persistent://tenant-name/namespace-name/topic-name
```

这个字符串包含了该 topic 所属的命名空间（namespace）和租户（tenant），并指明该 topic 是一个持久化（persistent）的 topic，会将发来的消息持久化到磁盘上。

Pulsar 中有一个默认租户名叫 `public`，其中的默认命名空间名叫 `default`，如果你不指定持久化类型、租户和命名空间的名称，只给出一个 `topic-name`，那么 Pulsar 会默认在 `public/default` 给你创建一个名为 `topic-name` 的持久化 topic：

```
persistent://public/default/topic-name
```

在我们炸弹人游戏的例子中，并不会用到租户、命名空间和非持久化的 topic，所以代码中只指定 topic 名称，其他信息直接用默认的就 OK。

**2、Pulsar 中的 topic 分为 non-partitioned topic 和 partitioned topic**。

举例很容易理解。创建一个名为 `topic-name` 的 non-partitioned topic，实际上就是创建了一个 topic：

```
persistent://public/default/topic-name
```

而创建 partitioned topic 呢，其实就是在 topic 名字后面加上了 `-partition-xx` 的后缀。比如说创建分区数量为 3 的名为 `topic-name` 的 partitioned topic，实际上就是创建了如下的三个 topic：

```
persistent://public/default/topic-name-partition-0
persistent://public/default/topic-name-partition-1
persistent://public/default/topic-name-partition-2
```

再我们的炸弹人游戏中，不会用到 partitioned topic，所以这里就简单提一下。如果要手动创建和管理 topic，需要用到 Admin API，具体可查看官网 [Pulsar Admin API](https://pulsar.apache.org/docs/next/admin-api-overview/)。

**3、默认配置下，Pulsar 会自动创建 non-partitioned topic**，如果消费者或生产者试图读写的 topic 不存在，Pulsar 会自动创建一个非分区 topic。

如果你想让 Pulsar 默认创建带分区的 topic，可以在 [broker 的配置文件](./pulsar介绍及启动.md) 中修改如下参数：

```conf
# The type of topic that is allowed to be automatically created.(partitioned/non-partitioned)
allowAutoTopicCreationType=non-partitioned

# The number of partitioned topics that is allowed to be automatically created if allowAutoTopicCreationType is partitioned.
defaultNumPartitions=1
```

你也可以直接禁止 Pulsar 自动创建不存在的 topic，可以在 [broker 的配置文件](./pulsar介绍及启动.md) 中修改如下参数：

```conf
# Enable topic auto creation if new producer or consumer connected (disable auto creation with value false)
allowAutoTopicCreation=true
```

再我们的炸弹人游戏不需要带分区的 topic，但需要自动创建不存在的 topic，所以保持默认配置不要改动就可以。

**4、默认配置下，Pulsar 会自动删除不活跃的 topic**。

在 [broker 的配置文件](./pulsar介绍及启动.md) 中默认开启了自动删除不活跃 topic：

```conf
# Enable the deletion of inactive topics. This parameter need to cooperate with the allowAutoTopicCreation parameter.
# If brokerDeleteInactiveTopicsEnabled is set to true, we should ensure that allowAutoTopicCreation is also set to true.
brokerDeleteInactiveTopicsEnabled=true

# How often to check for inactive topics
brokerDeleteInactiveTopicsFrequencySeconds=60

# Set the inactive topic delete mode. Default is delete_when_no_subscriptions
# 'delete_when_no_subscriptions' mode only delete the topic which has no subscriptions and no active producers
# 'delete_when_subscriptions_caught_up' mode only delete the topic that all subscriptions has no backlogs(caught up)
# and no active producers/consumers
brokerDeleteInactiveTopicsMode=delete_when_no_subscriptions
```

注释里说得很清楚，如果开启了自动创建 topic 的功能，自动删除 topic 的功能才会生效。这几个配置确定了 broker 自动删除 topic 的行为，默认是每 60 秒扫描所有 topic，如果没有 subscription 或活跃的生产者时判定该 topic 为非活跃，执行自动删除。

我在第一次用 Pulsar 时发现一个问题：生产者向某个 topic 中发消息后，消费者竟然收不到任何消息。其原因就是因为我是过了一段时间才启动消费者的，在消费者试图消费消息之前，这个 topic 已经被判定为不活跃被自动删除了，那么消费者其实相当于重新创建了一个新的 topic，当然无法消费之前的数据了。

**在学习 Pulsar client 的用法时，建议优先查看 [Java client 的文档](https://pulsar.apache.org/docs/next/client-libraries-java/)**，因为 Java client 是功能最齐全的，可以很容易地对应到其他语言，也有助于更好地理解 Pulsar。

所以之后生产消费的部分就用 Java client 为例讲解。

### 生产消息

Pulsar 的生产者这一侧并没有什么坑，只要指定 topic 名称即可向对应 topic 发送消息。

```java
PulsarClient client = PulsarClient.builder()
        .serviceUrl("pulsar://localhost:6650")
        .build();

Producer<byte[]> producer = client.newProducer()
        .topic("topic-name")
        .create();

MessageId msgID = producer.send("hello world".getBytes());
```

`send` 方法是同步发送消息，该条消息会立即发给 broker，在 bookie 节点上持久化到磁盘，并返回该条消息的 `msgID`。

如果你想批量发送消息，可以在创建 producer 的时候设置 batch 相关的参数，把多个消息攒到一起发送提高效率；如果你想发送大消息，可以在创建 producer 的时候设置 chunk 相关的参数，自动将大消息分块传输。更多详细信息可以查看 [官网对 producer 的介绍](https://pulsar.apache.org/docs/next/client-libraries-java/#producer)，我在这里就不浪费篇幅了。

不过我想着重说一下，producer 有一个 Access Mode 的概念，每个 producer 的默认 Access Mode 是 `Shared`，也就是说可以有多个 producer 向同一个 topic 发送消息。

我们可以给 producer 设置其他 Access Mode，比如设置成 `Exclusive`，这样就能保证只有一个 producer 向某个 topic 发送消息：

```java
Producer<byte[]> producer = client.newProducer()
        .topic("topic-name")
        .accessMode(ProducerAccessMode.Exclusive)
        .create();
```

更多具体的配置可以查看 [官网关于 Access Mode 的文档](https://pulsar.apache.org/docs/next/concepts-messaging/#access-mode)。

### 消费消息

首先要知道 Pulsar 的消费者有一个 Subscription 的概念（有点类似 kafka 中「消费者组」的概念）。

在创建 consumer 的时候，处理要指定 topic 名字，还要指定一个 `subscriptionName`：

```java
Consumer<byte[]> consumer = client.newConsumer()
        .topic("topic-name")
        .subscriptionName("my-subscription")
        .subscribe();
```

订阅了同一个 topic 且指定了相同 `subscriptionName` 的 consumer 就是加入了同一个 Subscription 下的消费者，可以共同消费这个 topic 的数据。

怎么共同消费呢？你可以通过 `subscriptionType` 配置它们的行为：

```java
Consumer<byte[]> consumer1 = client.newConsumer()
        .topic("topic-name")
        .subscriptionName("subscriptionName")
        .subscriptionType(SubscriptionType.Shared)
        .subscribe();

Consumer<byte[]> consumer2 = client.newConsumer()
        .topic("topic-name")
        .subscriptionName("subscriptionName")
        .subscriptionType(SubscriptionType.Shared)
        .subscribe();

Consumer<byte[]> consumer3 = client.newConsumer()
        .topic("topic-name")
        .subscriptionName("subscriptionName")
        .subscriptionType(SubscriptionType.Shared)
        .subscribe();
```

`subscriptionType` 总共有四种模式，分别是 `Shared, Exclusive, Failover, Key_Shared`，下面贴一张官网的图：

![](../images/sub-type.png)

`Shared` 模式的 Subscription 允许任意数量的 consumer 加入，对应 topic 的消息会被负载均衡算法分发给所有 consumer。

`Key_Shared` 模式类似 `Shared` 模式，区别是能够根据消息的 key 进行负载均衡。

`Failover` 模式的 Subscription 也允许任意数量的 consumer 加入，但只允许一个 consumer 独占消费 topic 中的数据，其他 consumer 都是替补，不会被分发消息。如果那个独占 consumer 下线，才会从替补 consumer 中选一个继续消费消息。

`Exclusive` 模式就更简单粗暴了，也是只能有第一个 consumer 能独占该 Subscription，但是后续试图连接该 Subscription 的 consumer 会被直接拒绝。





