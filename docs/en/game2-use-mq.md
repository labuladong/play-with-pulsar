# Developing Multiplayer Games with Apache Pulsar (2): Using a Pure Message Queue as a Game Backend

> Note: This article is the third in the series "Developing Multiplayer Online Games with Apache Pulsar." For source code and complete documentation, please check out my GitHub repository [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) and my article list.

As we mentioned before, each game client contains a Pulsar producer and a Pulsar consumer.

All player actions in the game will be abstracted into an event. The game client will listen for local keyboard actions and generate the corresponding event, which will be sent by the producer to the Pulsar message queue. At the same time, each game client's consumer will continuously retrieve events from Pulsar and apply them locally, ensuring that the views of all players are synchronized.

But synchronization between players is only the most basic requirement of a multiplayer game. I previously mentioned many features such as rooms and scoreboards. Let's see how we can implement them using the various features of Pulsar.

### How to Implement Game Rooms

Our game needs the concept of "rooms," where players in the same room can fight together, and different rooms cannot affect each other.

This requirement can be implemented using Pulsar's topic. A game room is a topic, and players in the same room will connect to the same topic. All event production and consumption will take place on the same topic, thus achieving isolation between different rooms.

### How to Implement Bomb Pushing

To enhance the difficulty and fun of the game, we allow players to push bombs.

![](https://labuladong.github.io/pictures/pulsar-game/pushbomb.gif)

This is actually like allowing bombs to move, just like player movement. We can also abstract bomb movement into an event:

```go
// Event for bomb movement
type BombMoveEvent struct {
	bombName string
	pos      Position
}
```

When a player collides with a bomb, continuous bomb movement events should be sent to the message queue.

### How to Update the Room Map on a Schedule

Obstacles in the map are randomly generated, and obstacles come in two types: destructible and non-destructible. Given that destructible obstacles can be blown up by players, we need to update each room's map at regular intervals.

![](https://labuladong.github.io/pictures/pulsar-game/mapupdate.gif)

This feature is a bit difficult to handle. You might say that we can also abstract the action of updating the map as an event (in fact, that's what I did):

```go
type UpdateMapEvent struct {
    // This list stores the positions of all obstacles
	Obstacles []Position
}
```

However, there are two problems with this:

**1. Who updates the map?**

We only have a Pulsar message queue in the backend, so you cannot write code in the backend to implement a timer that regularly sends messages to the topic.

> Tip: In fact, Pulsar can also provide some simple computing capabilities, called Pulsar Function, which I will introduce later.

So we can only write the map update logic on the front end (game client). But there is another problem here. Suppose there are three online clients, and each client sends a map update command every three minutes. In that case, it updates the map once a minute, which is obviously unreasonable.

Therefore, we need to implement logic similar to "election of leaders" between multiple clients, ensuring that only one leader client has the permission to update the map, and only this client will send the update map `Event` periodically. Moreover, if this client goes offline, another client needs to take over the leader's position and periodically update the map.

**2. How to ensure that new players can properly initialize the map?**

Because the new player's created consumer needs to start consuming from the latest message in the topic, if the map update events and other events are mixed together, new players cannot find the latest map update event from the historical message, so they cannot initialize the map:

![](https://labuladong.github.io/pictures/pulsar-game/4.jpeg)

Of course, in addition to providing the `Producer, Consumer` interface, Pulsar also provides the `Reader` interface, which can read messages in order from a specific position.

But `Reader` cannot solve this problem because we do not know the exact position of the most recent map update event, unless we traverse all events from the beginning, which is clearly inefficient.

In fact, we can make a slight change to solve these two problems.

First, in addition to the event topic that records player operation events, we can create another topic `map topic` specifically for storing map update events. This way, the latest map update event is the last message, which can be read by using the `Reader` to initialize the map for new players:

![](https://labuladong.github.io/pictures/pulsar-game/5.jpeg)

In addition, when creating a producer, Pulsar has an `AccessMode` parameter, which, as long as set to `WaitForExclusive`, ensures that only one producer successfully connects to the corresponding topic, and other producers are queued as backups.

This way, the requirement for regularly updating maps can be perfectly solved.

### How to Implement Room Scoreboards

Each game room needs a scoreboard to display the scoring situation of each player in the room.

![](https://labuladong.github.io/pictures/pulsar-game/scoreboard.jpg)

This requirement may seem simple, but it is slightly more complex to implement and requires the ability of **Pulsar Function** and **Pulsar tableview**. I will specifically discuss the development of Pulsar Function in later chapters, but briefly mention it here.

Pulsar Function allows you to write functions to process data in topics. The input of the function is one or more messages in the topic, and the return value of the function can be sent to other topics.

A diagram from the official Pulsar website makes it clear:

![](https://pulsar.apache.org/assets/images/function-overview-df56ee014ed344f64e7e0f807bd576c2.svg)

Pulsar Function supports Stateful Storage, and the official website provides an example of a word counter:

![](https://pulsar.apache.org/assets/images/pulsar-functions-word-count-f7b0d99f0a0e03e0b20fd0aa0ff6ef48.png)

In our Bomberman game, a player's death is also abstracted into an event and sent to the topic:

```go
type UserDeadEvent struct {
    // Dead player name
	playerName string
    // Killer player name
	killerName string
}
```

Similar to the word counter, we can also implement a Pulsar Function that filters out `UserDeadEvent` events and then counts the number of times `killerName` appears as the player's score.

Of course, we need to real-time update the scores of players in the room. Therefore, in addition to the event topic and map topic, each game room needs a score topic. Pulsar Function will output score update events to the score topic and use the tableview feature of Pulsar client to present the data in a better way.

We will skip the specific usage of Pulsar Function and tableview here and explain them in detail later.

### How to Implement Global Scoreboard

In addition to the scores in the current game room, we also need a global scoreboard that can rank the total scores of all players in different rooms.

Now that we can already implement the scoreboard in the room, there must be various ways to implement the global scoreboard.

Previously, we used Pulsar Function to calculate the scores of players in each room, which are basically `playerName -> score` key-value pairs. Therefore, we only need to traverse all the key-value pairs stored in Pulsar Function to accumulate the total score of a certain `playerName`. However, unfortunately, Pulsar Function does not provide an interface to traverse all key-value pairs, so we must find alternative solutions.

Actually, when it comes to ranking table requirements, the first thing that comes to my mind is Redis. Can we export the player score related statistical data to Redis? This will also facilitate more diversified processing of these data in the future.

Certainly, this is feasible. As previously mentioned, Pulsar Function can take messages from multiple topics as inputs, so we only need to include a Redis client in Pulsar Function to write data to Redis.

However, we don't need to write the Function code for data export to Redis ourselves. Pulsar provides a ready-made tool, namely **Pulsar Connector**.

As the name implies, connectors are connectors between Pulsar and other data systems, which can import data from other data systems into Pulsar or export the data within Pulsar to other data systems.

We only need to download the Redis connector and do some simple configurations to put it into use. Once the data is exported to Redis, it is easy to perform aggregation and sorting operations. We will introduce Pulsar Connector in detail in later chapters.

### How to Implement Game Replay

Suppose we will host important tournaments and need to support game "recording" for game replay.

Because we store all events generated by players in the topic, and the final state obtained by replaying these events from the same initial state is the same, we only need to read all messages from the header of the event topic to the back to replay the entire game process, which is equivalent to game replay.

Of course, Pulsar will enable some data expiration deletion policies by default to delete relatively old data in topics. We can disable this function or use **Pulsar Offloader** to unload relatively old data to other storage media.

The actual use case of Pulsar Offloader is to reduce the cost of massive data storage by unloading the old data to storage media with lower read and write efficiency but lower cost, and leave high-performance storage media to new data.

We will also experience the use of Offload later, and we will skip it here.