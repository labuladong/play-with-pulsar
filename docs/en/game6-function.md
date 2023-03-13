# Developing Room Scoreboard with Pulsar Functions

> Note: This article is the sixth part of "Developing Multiplayer Online Games with Pulsar". For source code and other documents, see my GitHub repository [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) and article list.

Pulsar Function allows you to write functions to process data in topics. The input of the function is one or more messages in topics, and the return value of the function can be sent to other topics.

A picture from the official website makes this clear:

![](https://labuladong.github.io/pictures/pulsar-game/function.jpg)

For example, if all the messages sent to `topicA` are English words, and you want to convert these English words to uppercase and forward them to `topicB`, you can write a Pulsar function to achieve this.

Pulsar Function also supports Stateful Storage, which is a key-value storage service.

For example, the official website provides an example of a word counter:

![](https://labuladong.github.io/pictures/pulsar-game/function2.png)

This Pulsar Function will read a sentence from a topic and divide it into words, then count the frequency of each word.

The word frequency is actually stored in this Function in the form of key-value pairs, and the value corresponding to the key can be read through the admin API. Documentation on the official website:

https://pulsar.apache.org/docs/next/functions-quickstart/#start-stateful-functions

Pulsar Function can be deployed as a separate service or uploaded to the broker as part of the broker. However, the current recommendation from the community is to deploy a separate Function cluster.

Currently, Pulsar supports developing Function using Python, Go, and Java. API documentation:

https://pulsar.apache.org/docs/next/functions-develop-api/

There are few examples in the documentation, and you can directly refer to the [Pulsar Function examples](https://github.com/apache/pulsar/tree/master/pulsar-functions/java-examples/src/main/java/org/apache/pulsar/functions/api/examples) and choose the appropriate Function based on your needs for development.

In this article, we will take the bombing man game as an example and use Pulsar Function to develop the scoreboard function of game rooms.

In our bomberman game, the player's death is also abstracted as an event sent to a topic:

```go
type UserDeadEvent struct {
    // Player name killed
    playerName string
    // Killer player name
    killerName string
}
```

Similar to the word counter, here we can also implement a Pulsar Function that specifically filters the `UserDeadEvent` event of player death and then calculates the number of times `killerName` appears as the score for that player.

Of course, we need to update the player's score in real-time in each game room, so in addition to the event topic and map topic, each game room also needs a score topic, so that the Pulsar Function can output the score update event to the score topic, and use the tableview function of the Pulsar client to present it better.

So the Pulsar Functions that need to be implemented now have the following requirements:

1. Since the events produced by players are all sent to topics of the format `{roomName}-event-topic`, the function should receive messages from all these topics.
2. Filter out `UserDeadEvent` events and read `playerName` and `killerName`. The number of times `killerName` appears is the score the player gets.
3. The player's score also needs to be output to another topic of the format `{roomName}-score-topic`.

Let's start with the development of the function.

### Developing the Function

First, let's look at the official documentation:

https://pulsar.apache.org/docs/next/functions-develop-api/#use-sdk-for-javapythongo

You need to set Maven dependencies related to Pulsar Function development:

```xml
<dependency>
    <groupId>org.apache.pulsar</groupId>
    <artifactId>pulsar-functions-api</artifactId>
    <version>${pulsar.version}</version>
</dependency>
```

Then you can start developing, the complete source code is in the [function-code](https://github.com/labuladong/play-with-pulsar/tree/master/function-code) directory:

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

Since we set JSON Schema for the messages in the topic in the previous article, the message type in the topic is set to `GenericJsonRecord`.

The logic of this code should be easy to understand. The `input` is the message sent to the event topic. Through Pulsar Function's `context`, we can obtain the name of this event topic. Since the name of the event topic contains the game room name, we only need to modify the event topic name suffix to get the name of the score topic.

The main function of the Function is to filter out `UserDeadEvent` events, read `killerName`. Considering that the kill events of different rooms cannot be mixed together, I use `{roomName}-{killerName}` as the key, increment the counter to record each player's score, and finally call `context.newOutputMessage` to send the player's score to the corresponding score topic of the room.

### Debugging the Function

You can refer to this official website document to debug the Function locally using the localrun mode:

https://pulsar.apache.org/docs/next/functions-debug-localrun/

The localrun mode is equivalent to starting a Function worker locally, which can connect to Pulsar and run the Function code we just developed.

The complete source code is in the [function-code](https://github.com/labuladong/play-with-pulsar/tree/master/function-code) directory. Note that we need to configure the Function correctly, such as the Function class and the name of the input topic:

```java
 String inputTopic = ".*-event-topic";
// enable regex support to subscribe multiple topics
HashMap<String, ConsumerConfig> inputSpecs = new HashMap<>();
ConsumerConfig consumerConfig = ConsumerConfig.builder().isRegexPattern(true).build();
inputSpecs.put(inputTopic, consumerConfig);
functionConfig.setInputSpecs(inputSpecs);

functionConfig.setClassName(ScoreboardFunction.class.getName());
```

After configuring the `functionConfig`, you can start a local Function worker:


```java
LocalRunner localRunner = LocalRunner.builder()
        .brokerServiceUrl("pulsar://localhost:6650")
        .stateStorageServiceUrl("bk://localhost:4181")
        .functionConfig(functionConfig)
        .build();

localRunner.start(false);
```

The `brokerServiceUrl` is the connection address of the Pulsar broker, and the `stateStorageServiceUrl` is the address of the BookKeeper that provides stateStorage. By default, it is on port 4181.

Therefore, by simply starting the `main` function, the local runner will start, and the Function we just developed will be loaded, which will input messages from topics that end with `-event-topic` into the Function.

### Development of the scoreboard

The Function we just developed will send the player's name and score as the key-value pair of a message to `{roomName}-score-topic`. How can the player client obtain this information? This requires the use of the tableView feature introduced earlier.

The use of tableView can be seen in the game client code:

```go
tableView, err := client.CreateTableView(pulsar.TableViewOptions{
    Topic:           roomName + "-score-topic",
    Schema:          pulsar.NewStringSchema(nil),
    SchemaValueType: reflect.TypeOf(""),
})
```

We maintain an LRU cache named `scores` in the game data, which stores score information for up to 5 recent players. At the same time, we use the `ForEachAndListen` method of the tableView to update the LRU cache:

```go
client.tableView.ForEachAndListen(func(playerName string, i interface{}) error {
    score := *i.(*string)
    g.scores.Add(playerName, score)
    return nil
})
```

Therefore, when a player's score is updated, the data in the LRU cache will also be updated, and we only need to display the corresponding score data on the game interface.