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

### Producer

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

### Consumer

**Pulsar 的消费者这一侧比生产者复杂得多，首先需要搞明白 topic、subscription、consumer 三个概念**。

topic 之前已经说过了，我们可以把一个 topic 理解成一个**数组**，生产者发来的消息以 append 的方式加在数组的尾部。

subscription 呢，可以理解为是数组上的一个**索引指针**，指向数组中的某个元素。这个指针之前的所有元素是已经被消费的（已经被 ack 的），这个指针之后的所有元素是还未被消费的。

一个数组上可以有多个指针，每个指针可以指向数组的不同位置。不同指针用不同的 `subscriptionName` 进行区分。

一个 subscription 下面可以有多个 consumer，subscription 指针会读取后续数组中未被消费的元素分发给这些 consumer。如果 consumer 消费（ack）了这些数据，则 subscription 指针向前移动，继续读取新的数据分发给这些 consumer。

那问题来了，subscription 指针的初始位置应该指向哪里呢？如果指向数组的开头，就能读取数组中的所有数据，如果指向数组的末尾，就可以直接处理新来的数据，这两个场景都很常见，我们可以通过 `subscriptionInitialPosition` 参数来设置新建的 subscription 指针的初始位置。

那么 subscription 到底是怎么给 consumer 分发数据的呢？是平均分配雨露均沾？还是先来的独享后来的靠边站？这也可以在创建 subscription 时通过 `subscriptionType` 参数指定，目前 Pulsar 支持 4 种订阅模式，分别是 `Shared, Exclusive, Failover, Key_Shared`，下面贴一张官网的图：

![](../images/sub-type.png)

`Shared` 模式的 Subscription 允许任意数量的 consumer 加入，对应 topic 的消息会被负载均衡算法分发给所有 consumer。

`Key_Shared` 模式类似 `Shared` 模式，区别是能够根据消息的 key 进行负载均衡，保证 key 相同的消息一定会被发到同一个 consumer。

`Failover` 模式的 Subscription 也允许任意数量的 consumer 加入，但只允许一个 consumer 独占消费 topic 中的数据，其他 consumer 都是替补，不会被分发消息。如果那个独占 consumer 下线，才会从替补中选一个新的 consumer 继续消费消息。

`Exclusive` 模式就更简单粗暴了，也是只能有第一个 consumer 能独占该 Subscription，但是后续试图连接该 Subscription 的 consumer 会被直接拒绝。

现在你应该搞明白 topic、subscription、consumer 三者之间的关系了，现在可以看看如何创建 consumer 了：

```java
Consumer<byte[]> consumer1 = client.newConsumer()
        .consumerName("consumer-1")
        .topic("topic-1")
        .subscriptionName("subscription-1")
        .subscriptionInitialPosition(SubscriptionInitialPosition.Earliest)
        .subscriptionType(SubscriptionType.Shared)
        .subscribe();
```

这段代码其实做了两件事：

1、试图在 `topic-1` 上创建一个名为 `subscription-1` 的 `Shared` 类型的 subscription。

如果 `topic-1` 上面还没有一个名为 `subscription-1` 的 subscription，则创建成功，该 subscription 初始位置指向 `topic-1` 中的第一条消息。

如果 `topic-1` 上面已经有一个名为 `subscription-1` 的 subscription，且这个已存在的 subscription 也是 `Shared` 类型，则什么都不做。

2、创建一个名为 `consumer-1` 的消费者，加入名为 `subscription-1` 的 subscription 中。其实 `consumerName` 并不重要，一般可以不设置，Pulsar 会给我们随机创建一个全局唯一的 `consumerName`。

**回到我们的炸弹人游戏中，房间内所有玩家的操作事件都会发到同一个 topic 中，每个游戏客户端肯定要创建一个 consumer 去消费事件**。大家可以想一想，我们应该如何给这个 consumer 设置参数呢？

不难想到，每个玩家都需要独立的 subscription 消费 topic 中的数据，且每个玩家只对进入房间的那一刻之后的事件数据感兴趣，所以我们可以这样创建 consumer：

```java
String roomEventTopic = roomName + "-event-topic";
Consumer<byte[]> consumer = client.newConsumer()
        .topic(roomEventTopic)
        // 从最新的事件开始消费
        .subscriptionInitialPosition(SubscriptionInitialPosition.Latest)
        // 每个玩家都有自己的 subscription
        .subscriptionName(playerName)
        // 设置 subscription 为排他模式，
        // 如果有同名的 player 同时登录就会报错
        .subscriptionType(SubscriptionType.Exclusive)
        .subscribe();
```

需要注意的是，每次玩家退出游戏时要调让 consumer 取消订阅：

```java
consumer.unsubscribe();
```

如果取消订阅，则当前这个 subscription 就会被 broker 删除，下次该玩家登录时重新创建新的 subscription，指向最新的消息。

如果不取消订阅，则这个 subscription 还会保留在这个 topic 上，那么下次玩家登录创建的 consumer 还会加到这个 subscription 下，但由于 topic 中的数据还在增加，这个 subscription 指向的位置已经不再是最新消息了，consumer 需要把下线这段时间的所有消息都消费掉才能接收最新的消息。

所以玩家将看到类似放电影的场景，必须观看完下线期间其他玩家的对战，自己才能进行操作，这不是预期的行为，所以需要我们通过 `unsubscribe` 来删除不需要的 subscription。

### Reader

根据 [官网的描述](https://pulsar.apache.org/docs/next/client-libraries-java/#reader)，Reader 接口主要提供「从某个具体的 messageID 开始读取消息」的功能，但我觉得 Reader 的一个重要作用是**读取 topic 中的最后一条消息**。

创建 consumer 时即便你指定 `subscriptionInitialPosition` 为 `Latest`，你也只能收到新发送到 topic 中的消息，而不能读取到已经发送到 topic 中的最后一条消息。

而 Reader 接口可以做到这一点：

```java
Reader reader = pulsarClient.newReader()
        .topic(topic)
        // 指向最后一条消息
        .startMessageId(MessageId.earliest)
        // 包含那条消息
        .startMessageIdInclusive()
        .create();

// 读到当前 topic 中的最后一条消息
Message lastMsg = reader.readNext();
```

在我们的炸弹人游戏中，更新地图的事件会发到一个单独的 topic 中，所以新加入的玩家需要读取该 topic 中的最后一条消息，目前只能通过 Reader 接口做到这一点。

### TableView

Pulsar 还提供一种消息读取方式叫做 TableView，官网文档：

https://pulsar.apache.org/docs/next/client-libraries-java/#tableview

使用起来是这样的：

```java
TableView<String> tv = client.newTableViewBuilder(Schema.STRING)
        .topic("my-tableview")
        .create()

tv.forEach((key, value) -> /*operations on all existing messages*/)
```

TableView 主要针对带有 key 的消息，官网的这个图很形象：

![](https://pulsar.apache.org/assets/images/tableview-a5bea774c5591395d61725e720ebf908.png)

你就可以把 TableView 理解成一个存储键值对的 map，这里面的键是 topic 中的所有消息使用过的 key，值是持有这些 key 的最近的一条消息的 payload。

TableView 在某些场景下挺有用，不过底层的实现原理其实特别简单，就是创建了一个 Reader 读取了 topic 中的所有消息，最后计算出所有的键值对。