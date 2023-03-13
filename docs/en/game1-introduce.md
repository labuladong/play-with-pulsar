# Developing Multiplayer Game with Pulsar (Part 1): Requirements and Challenges for Bomberman Game Development

I previously wrote an article about how I implemented a simple Bomberman game using the Pulsar messaging system in the article [I Built an Online Game with Message Queue](https://mp.weixin.qq.com/s/kI0HUTFVr4YEBpLRZWLEDg), and many readers showed interest in this game, even encountering some readers in the Pulsar technical group.

Therefore, I decided to develop more features for this game and provide more detailed documentation, specifically outlining the development approach, technology components, and algorithms used.

Even if you haven't read the previous article, it's fine. In this article, I will introduce this project in more detail, and in subsequent articles, I will explain the implementation approach for each feature in detail.

I've already placed the more complete code and documentation on GitHub:

https://github.com/labuladong/play-with-pulsar

Let's start with the game that I loved to play when I was a child:

![](https://labuladong.github.io/pictures/pulsar-game/0.jpg)

This game is called Q version Bubble Fighter, and many readers should have played it as a child. In the game, players can control a robot to place bombs, blow up obstacles to obtain random props, and eliminate all other robots. If a player is eliminated by another robot, they fail the challenge.

The other robots in this game are computer-controlled, and to be honest, they are somewhat stupid. I beat the hard difficulty level in just an hour. So I thought, could this type of Bomberman game be made into a multiplayer online game, allowing a few good friends to play PK online?

Based on this idea and fully utilizing existing technology components, I plan to write a series of tutorials:

![](https://labuladong.github.io/pictures/pulsar-game/en_xmind.jpg)

The final product of this tutorial is a multiplayer online Bomberman game:

![](https://labuladong.github.io/pictures/pulsar-game/preview.jpg)

## Game Functionality Planning

In the classic Bomberman game, each player can move and place bombs. After a certain period of time, the bombs will explode, and players who are hit by them will die immediately, but players can infinitely revive.

![](https://labuladong.github.io/pictures/pulsar-game/revive.gif)

In addition to the basic gameplay, we also have the following requirements:

1. **The concept of "rooms" is required** - players can only battle together in the same room, and different rooms cannot affect each other.

![](https://labuladong.github.io/pictures/pulsar-game/roomname.jpg)

2. To enhance the operational difficulty and fun of the game, players are allowed to **push bombs**.

![](https://labuladong.github.io/pictures/pulsar-game/pushbomb.gif)

3. Obstacles in the map are randomly generated and are divided into two types: destroyable and indestructible. Considering that destroyable obstacles will be destroyed by players, we need to **regularly update the maps of each room**.

![](https://labuladong.github.io/pictures/pulsar-game/mapupdate.gif)

4. There needs to be a **room scoreboard** that displays the scores of each player in the room.

![](https://labuladong.github.io/pictures/pulsar-game/scoreboard.jpg)

5. In addition to the current game room score, we also need a **global scoreboard**, which can rank all players' scores in different rooms.

6. Assuming we will hold important events, we need to support game "recording" so that we can view game replays.

7. It would be great if we could **support AI player battles** so that we could also play solo.

## Challenges of Multiplayer Games

I haven't specifically developed multiplayer online games, but after a simple analysis, I've summarized the following key points:

**1. Multiplayer online games definitely need a backend service for all players to connect to. However, since this is a small game, I hope to develop it as simply as possible and write as little code as possible to avoid reinventing the wheel.**

**2. Most importantly, all players' actions must be synchronized, or in other words, ensure the "consistency" of each player's view.**

Ideally, the actions of one player can be instantly synchronized to all other players through quantum entanglement, so that each player's view will always be consistent.

However, in reality, the operation of each player on the local machine needs to be sent to the game's server via the network, and then the server synchronizes it again to other players through the network. There are two more network communications in this process, and any accident can easily destroy the consistency of each player's view.

For example, two players `playerA` and `playerB` stand in positions `(2, 3)` and `(6, 4)` respectively. At this point, each player's local and server states are consistent and accurate:

![](https://labuladong.github.io/pictures/pulsar-game/1.jpeg)

Now, if `playerA` moves, updates the local state, and notifies the server, the server will then communicate it to `playerB`. However, if there is network jitter during server-to-playerB communication and communication fails, it'll result in an incorrect local state for `playerB`:

![](https://labuladong.github.io/pictures/pulsar-game/2.jpeg)

`playerB` sees that `playerA` is still in position `(2, 3)`, but in reality, `playerA` has moved to `(3, 3)`.

If `playerB` attacks `(2, 3)` at this time, he will see that he attacked `playerA`, but in reality, `playerA` has not been attacked. This is obviously a severe bug.

You may ask, what if the communication failure happens? Can’t we just retry?

Actually, that won't be easy, because the local state of `playerB` will be incorrect during the retry period. Any action based on that incorrect state will only make the problem worse.

## How to Synchronize Players

The solution is simple - **We can use a message queue to solve the player synchronization problem on the backend.**

1. Abstract each player's actions into an event.

2. There's a global event sequence (message queue) on the server side.

3. Execute a series of identical events from the same initial state to obtain consistent results.

Once the above conditions are met, all local clients of players can consume global event queue in order from the server to ensure that the local state of each player's client is consistent.

Therefore, our backend service is essentially a message queue. Local events generated by the client must first be successfully sent to the message queue before being read and updated locally:

![](https://labuladong.github.io/pictures/pulsar-game/3.jpeg)

The following pseudo-code may provide a clearer understanding:

```java
// A thread is responsible for fetching and displaying events
new Thread(() -> {
    while (true) {
        // Continuously getting events from the message queue
        Event event = consumer.receive();
        // Update local state and display to players
        updateLocalScreen(event);
    }
});

// A thread is responsible for generating and sending local events
new Thread(() -> {
    while (true) {
        // Local events generated by the player must be sent to the message queue
        Event localEvent = listenLocalKeyboard();
        producer.send(event);
    }
});
```

This way, all player clients rely on the order of events in the backend message queue (globally consistent) to sequentially consume these events and update their local states, ensuring that the local states of all clients are globally strongly consistent.

> Note: When playing MOBA games, we may experience the fast-forward effect due to a brief network interruption and reconnection. Therefore, I suspect that real multiplayer online games may indeed use a message queue-like mechanism to ensure synchronization between players.

In the next article, I will explain in detail how to use Apache Pulsar as a message queue to implement the game functions listed above.