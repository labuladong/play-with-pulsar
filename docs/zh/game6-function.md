---
title: '用 Pulsar 开发多人小游戏（六）：用 Pulsar Function 制作房间计分板'
---

> note：本文是《用 Pulsar 开发多人在线小游戏》的第三篇，配套源码和全部文档参见我的 GitHub 仓库 [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) 以及我的文章列表。

Pulsar Function 允许你编写函数对 topic 中的数据进行一些处理，函数的输入就是一个或多个 topic 中的消息，函数的返回值可以发送到其他 topic 中。

官网的一张图就能看明白了：

![](https://labuladong.github.io/pictures/pulsar-game/function.jpg)

比方说，发送到 `topicA` 中的消息都是英文单词，我想把这些英文单词都转化成大写并转发到 `topicB` 中，那么就可以写一个 Pulsar function 做这个事情。

Pulsar Function 还支持 Stateful Storage，简单来说就是键值对的存储服务。

比如官网给了一个单词计数器的例子：

![](https://labuladong.github.io/pictures/pulsar-game/function2.png)

这个 Pulsar Function 会从一个 topic 中读取句子并切分成单词，然后统计每个单词出现的频率。

单词频率其实是以键值对的形式存储在这个 Function 中的，可以通过 admin API 来读取键对应的值，官网文档：

https://pulsar.apache.org/docs/next/functions-quickstart/#start-stateful-functions

Pulsar Function 可以单独部署成服务，也可以上传到 broker 上，作为 broker 的一部分。不过目前社区的建议是部署单独的 Function 集群。

目前 Pulsar 支持使用 Python、Go、Java 来开发 Function，API 文档：

https://pulsar.apache.org/docs/next/functions-develop-api/

文档给出的例子比较少，可以直接看 [Pulsar Function examples](https://github.com/apache/pulsar/tree/master/pulsar-functions/java-examples/src/main/java/org/apache/pulsar/functions/api/examples)，直接根据需求选择合适的 Function 进行开发就行了。

本文就以炸弹人游戏为例，利用 Pulsar Function 开发游戏房间的计分板功能。

在我们的炸弹人游戏中，玩家的死亡也会被抽象成事件发送到 topic 中：

```go
type UserDeadEvent struct {
    // 被炸死的玩家名
	playerName string
    // 杀手玩家名
	killerName string
}
```

类似单词计数器，我们这里也可以实现一个 Pulsar Function，专门过滤玩家死亡的 `UserDeadEvent` 事件，然后统计 `killerName` 出现的次数，就可以作为该玩家的分数了。

当然，我们需要实时更新房间内玩家的分数，所以每个游戏房间除了 event topic 和 map topic 之外，我们还需要一个 score topic，让 Pulsar Function 把分数更新事件输出到 score topic，并且利用 Pulsar client 的 tableview 功能做一个比较好的展现。

那么现在需要实现的 Pulsar Function 有如下需求：

1、因为玩家产生的事件都发到了格式为 `{roomName}-event-topic` 的 topic 中，所以函数应该接收所有这些 topic 的消息。

2、读取这些消息的 `Type` 字段，过滤出 `UserDeadEvent` 事件，并读取 `playerName` 和 `killerName`，`killerName` 出现的次数就是该玩家获得的分数。

3、还需要把玩家分数输出到另一个格式为 `{roomName}-score-topic` 的 topic 中。

下面开始开发。

### Function 的开发

先贴官网文档：

https://pulsar.apache.org/docs/next/functions-develop-api/#use-sdk-for-javapythongo

首先需要设置 Pulsar Function 开发相关的 Maven 依赖：

```xml
<dependency>
    <groupId>org.apache.pulsar</groupId>
    <artifactId>pulsar-functions-api</artifactId>
    <version>${pulsar.version}</version>
</dependency>
```

然后就可以开始开发了，完整的源码在 [function-code](https://github.com/labuladong/play-with-pulsar/tree/master/function-code) 目录：

```java
public class ScoreboardFunction implements Function<GenericJsonRecord, Void> {

    @Override
    public Void process(GenericJsonRecord input, Context context) {

        String type = (String) input.getField("type");
        if (type.equals("UserDeadEvent")) {
            String player = (String) input.getField("name");
            String killer = (String) input.getField("comment");
           if (player.equals(killer)) {
               // kill himself
               return null;
           }

            // get the source topic of this message
            Optional<String> inputTopic = context.getCurrentRecord().getTopicName();
            if (inputTopic.isEmpty()) {
                return null;
            }
            // calculate the corresponding topic to send score
            Optional<String> outputTopic = changeEventTopicNameToScoreTopicName(inputTopic.get());
            if (outputTopic.isEmpty()) {
                return null;
            }
            // roomName-playerName as the stateful key  /
            // store the score in stateful function
            String killerKey = parseRoomName(inputTopic.get()).get() + "-" + killer;
            context.incrCounter(killerKey, 1);

            // send the score messages to score topic
            long score = context.getCounter(killerKey);
            try {
                // player name as the key, score as the value
                context.newOutputMessage(outputTopic.get(), Schema.STRING)
                        .key(killer)
                        .value(score + "")
                        .send();
            } catch (PulsarClientException e) {
                // todo: ignore error for now
                e.printStackTrace();
            }
        }

        return null;
    }
}
```

因为我们前文给 topic 中的消息设置了 JSON Schema，所以这里设置 topic 中的消息类型为 `GenericJsonRecord`。

这段代码的逻辑应该不难理解，`input` 就是发到 event topic 的消息，通过 Pulsar Function 的 `context` 可以拿到这个 event topic 的名字，由于 event topic 名字包含游戏房间名，所以只要修改 event topic 名称后缀即可得到 score topic 的名字。

函数的主要工作是过滤出 `UserDeadEvent`，读取 `killerName`。考虑到不能把不同房间的击杀事件混在一起，我把 `{roomName}-{killerName}` 作为 Function 的键，并递增计数器记录玩家的分数，最后调用 `context.newOutputMessage` 把玩家的分数发送到房间对应的 score topic 中。

### Function 的调试

可以参考这篇官网文档，用 localrun 模式在本地调试 Function：

https://pulsar.apache.org/docs/next/functions-debug-localrun/

localrun 模式相当于直接在本地起了一个 Function worker，能够连接到 Pulsar，并运行我们刚才开发的 Function 代码。

完整的源码在 [function-code](https://github.com/labuladong/play-with-pulsar/tree/master/function-code) 目录，注意我们要对 Function 进行正确的配置，比如 Function 类以及作为输入的 topic 名称等等：

```java
 String inputTopic = ".*-event-topic";
// enable regex support to subscribe multiple topics
HashMap<String, ConsumerConfig> inputSpecs = new HashMap<>();
ConsumerConfig consumerConfig = ConsumerConfig.builder().isRegexPattern(true).build();
inputSpecs.put(inputTopic, consumerConfig);
functionConfig.setInputSpecs(inputSpecs);

functionConfig.setClassName(ScoreboardFunction.class.getName());
```

配置完 `functionConfig` 后可以启动一个本地的 Function worker：


```java
LocalRunner localRunner = LocalRunner.builder()
        .brokerServiceUrl("pulsar://localhost:6650")
        .stateStorageServiceUrl("bk://localhost:4181")
        .functionConfig(functionConfig)
        .build();

localRunner.start(false);
```

其中 `brokerServiceUrl` 是 Pulsar broker 的连接地址，`stateStorageServiceUrl` 是提供 stateStorage 的 bookkeeper 地址，默认情况下在 4181 端口。

这样，只要启动 main 函数，就会启动 local runner，并加载我们刚开发的 Function，把所有后缀为 `-event-topic` 的 topic 中的消息输入给 Function。

### 计分板的开发

我们刚才开发的 Function 会把玩家名称和该玩家获得的分数作为一条消息的键和值发送到 `{roomName}-score-topic` 中，那么玩家客户端如何获取这些信息呢？这就要用到之前介绍的 tableView 功能了。

可以在游戏客户端代码中看到 tableView 的使用：

```go
tableView, err := client.CreateTableView(pulsar.TableViewOptions{
    Topic:           roomName + "-score-topic",
    Schema:          pulsar.NewStringSchema(nil),
    SchemaValueType: reflect.TypeOf(""),
})
```

我们在游戏数据中维护一个名为 `scores` 的 lru 缓存，存储最近的最多 5 名玩家的分数信息，同时利用 tableView 的 `ForEachAndListen` 方法更新 lru 缓存：

```go
client.tableView.ForEachAndListen(func(playerName string, i interface{}) error {
    score := *i.(*string)
    g.scores.Add(playerName, score)
    return nil
})
```

这样，当玩家分数更新时，lru 缓存中的数据就会更新，我们只要把对应的分数数据显示到游戏界面上即可。