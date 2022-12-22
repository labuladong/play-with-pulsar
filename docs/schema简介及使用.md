# 什么是 schema？为什么要用 schema？

我推荐《数据密集型应用系统设计》这本书的第四章：编码与演化（[在线阅读地址](http://ddia.vonng.com/#/ch4)）。

编码（encoding）和演化（evolution）是两个不同的概念，我举例说明一下。

**编码是把内存中的数据结构转化成某种格式的字节序列，方便传输或存储**。

我们最常见的编码方式就是 JSON 了，JSON 的好处就是字符串处理起来很简单，且易于人类阅读；但问题是支持的数据类型不够丰富，且传输效率不高。为了解决 JSON 的这些问题，就出现了 XML 格式或者 protobuf 这样的二进制编码协议。

确定了编码方式，生产者就可以愉快地把数据编码成对应的格式发送进 Pulsar，消费者只要按照按照同样的协议解码，就能得到原始数据了。

**但问题是，我们开发的程序是在不断变化的，所以数据本身的结构很可能发生变化，这也就是演化的概念**。

比如说一开始我把这个 `User` 类序列化成 JSON 字符串然后发到 Pulsar 中：

```java
class User {
    String name;
    int id;
}
```

但是随着业务发展，我发现用 int 类型已经无法表示用户 ID 了，需要把 `id` 这个字段改为字符串类型：

```java
class User {
    String name;
    String id;
}
```

这种情况下，生产者依然可以把这个类序列化成 JSON 然后发到 Pulsar 中，Pulsar 不关心数据本身的内容，默认把所有数据都视为字节数组，所以会欣然接受生产者发来的消息，但消费者那边消费数据时肯定会出问题。

那你说，我同时改生产者和消费者的代码还不行吗？可以，但依然存在很多问题，比如说：

1、在复杂的业务场景中，数据处理的逻辑可以非常复杂，生产者和消费者可能横跨多个部门，协调成本很高。

2、必须先下线所有消费者，代码升级完之后才能重新上线。否则消费者突然消费到无法识别的新的数据格式，会产生不可预期的错误。

**所以，让系统能够更加灵活地适应变化，就是 schema 能够给我们提供的价值**。

有了 schema 来描述数据的格式，我们就可以设置数据结构的兼容性（compatibility），比方说我们可以设置数据向后兼容，即新代码可以读取旧格式的数据；或者设置向前兼容，即旧代码可以读取新格式的数据。

### 在 Pulsar 中使用 schema

schema 相关的官网文档：

https://pulsar.apache.org/docs/next/schema-overview/

我们可以设置 schema 的兼容性，官网文档：

https://pulsar.apache.org/docs/next/schema-understand#schema-compatibility-check-strategy

我们在创建 Pulsar 的生产者/消费者时可以指定消息的 schema：

```java
// 指定生产者的 schema
Consumer<User> consumer = client.newConsumer(Schema.AVRO(User.class))
        .subscriptionType(SubscriptionType.Shared)
        .topic(topicName)
        .subscriptionName(subscriptionName)
        .subscribe();

// 指定消费者的 schema
Producer<User> producer = client.newProducer(Schema.AVRO(User.class))
        .topic(topicName)
        .create();
```

每个 topic 的 schema 演化信息都会存在 Pulsar 当中，这样当生产者使用新的 schema 时，Pulsar 会判断新的 schema 是否符合当前的兼容性设置，如果符合则更新 topic 对应的 schema，否则的话则会拒绝生产者发来的消息，这样消费者就不会收到不符合预期的数据了。

在我们的炸弹人游戏中，玩家可以产生很多不同类型的事件，这些事件应该以什么格式发送到 Pulsar 的 topic 中呢？

我的做法是创建了一个 `EventMessage` 结构，用 `Type` 字段标识事件的类型：

```go
type EventMessage struct {
	// Event type
	Type   string `json:"type"`
	Name   string `json:"name"`
	Avatar string `json:"avatar"`
	// Comment stores extra data
	Comment string `json:"comment"`
	X       int    `json:"x"`
	Y       int    `json:"y"`
	Alive   bool   `json:"alive"`
	List    []int  `json:"list"`
}
```

然后用 Avro 的方式定义了一个 JSONSchema：

```go
const eventJsonSchemaDef = `
{
  "type": "record",
  "name": "EventMessage",
  "namespace": "game",
  "fields": [
    {
      "name": "Type",
      "type": "string"
    },
    {
      "name": "Name",
      "type": "string"
    },
    {
      "name": "Avatar",
      "type": "string"
    },
    {
      "name": "Comment",
      "type": "string",
	  "default": ""
    },
    {
      "name": "X",
      "type": "int"
    },
    {
      "name": "Y",
      "type": "int"
    },
	{
      "name": "Alive",
      "type": "boolean"
    },
    {
      "name": "List",
		"type": {
			"type": "array",
			"items" : {
				"type":"int"
			}
		}
    }
  ]
}
`

// player event topicName
producer, err := client.CreateProducer(pulsar.ProducerOptions{
    Topic:           topicName,
    // use schema to confirm the structure of message
    Schema: pulsar.NewJSONSchema(eventJsonSchemaDef, nil),
})
```

这样一来，就可以通过 Schema 的约束避免未来可能产生的很多问题。