# Developing Multiplayer Games with Pulsar (Part 5): What is a Schema and Why Use it?

> Note: This article is part 3 of "Developing Multiplayer Online Games with Pulsar" series. For supporting source code and full documentation, please refer to my GitHub repository [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) and my list of articles.

I recommend chapter 4, "Encoding and Evolvability" in the book, "Designing Data-Intensive Applications".

Encoding and evolution are two different concepts. Encoding is the process of converting a data structure in memory into a byte sequence in some format for transmission or storage. JSON is the most commonly used encoding format. The advantages of JSON are that it is easy to read, and can be handled easily as a string. However, the cost of this simplicity is that the supported data types are not rich enough, and the transmission efficiency is low. To solve these issues, binary encoding protocols, such as XML or Protocol Buffers, have been developed.

Once the encoding method is determined, the producer can serialize the data into the corresponding format and send it into Pulsar. The consumer can decode the data in the same way to get the original data.

The problem is that our programs are constantly changing, which means that the structure of the data itself is likely to change. This is the concept of evolution. For example, suppose I serialized this `User` class as a JSON string and sent it to Pulsar:

```java
class User {
    String name;
    int id;
}
```

But as the business develops, I realize that I can't represent the user ID with an int, and need to change the `id` field to a string type:

```java
class User {
    String name;
    String id;
}
```

In this case, the producer can still serialize this class into JSON and send it to Pulsar. Pulsar doesn't care about the content of the data itself and simply views all data as byte arrays by default, so it will happily accept the message sent by the producer. However, there will be problems when the consumer tries to consume the data.

What if we change both the producer and the consumer code at the same time? Yes, that would work, but there are still many problems. For example:

1. In complex business scenarios, the data processing logic can be very complex, and producers and consumers may span multiple departments, making coordination costly.

2. All consumers must be taken offline before upgrading the code. Otherwise, if a consumer suddenly consumes data in an unrecognizable new format, unexpected errors will occur.

Therefore, making the system more adaptable to change is where the value of a schema lies.

With a schema to describe the format of the data, we can set the compatibility of the data structure. For example, we can set the data to be backwards-compatible, which means that new code can read old versions of the data. Or, we can set it to be forwards-compatible, meaning that old code can read new versions of the data.

### Using Schema in Pulsar

Official documents related to schema:

https://pulsar.apache.org/docs/next/schema-overview/

We can set the compatibility of the schema, as described in the official documents:

https://pulsar.apache.org/docs/next/schema-understand#schema-compatibility-check-strategy

When creating a producer/consumer in Pulsar, we can specify the schema of the message:

```java
// Specify schema of producer
Consumer<User> consumer = client.newConsumer(Schema.AVRO(User.class))
        .subscriptionType(SubscriptionType.Shared)
        .topic(topicName)
        .subscriptionName(subscriptionName)
        .subscribe();

// Specify schema of consumer
Producer<User> producer = client.newProducer(Schema.AVRO(User.class))
        .topic(topicName)
        .create();
```

The evolution information of the schema for each topic is stored in Pulsar. When a producer uses a new schema, Pulsar will check whether the new schema conforms to the current compatibility setting. If it does, the corresponding schema of the topic will be updated, otherwise the message sent by the producer will be rejected. This way, the consumer will not receive data that does not meet its expectations.

In our Bomberman game, players can generate many different types of events. In what format should we send these events to the topic in Pulsar?

My approach is to create an `EventMessage` structure, with a `Type` field to signify the event type:

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

Then I used Avro to define a JSONSchema:

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

This way, we can avoid many potential problems in the future by using the constraints of the schema.