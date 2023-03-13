# Developing multiplayer games with Pulsar (7): Creating a global scoreboard using Pulsar Connector

> Note: This article is the third in the series "Developing multiplayer online games with Pulsar." For the complete code and documentation, please refer to my GitHub repository [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) and my article list.

In the previous chapter, we introduced the use of Pulsar Functions. Each game room has a corresponding score topic, and each player's score is sent to the score topic. The game client displays the in-game scoreboard by reading the tableView of this topic.

However, considering that a player may play games in multiple game rooms, we would like to have a total score for the player in different rooms to achieve a function like a "global ranking list."

To implement this global scoreboard, the simplest and most direct solution is to create a consumer to consume messages from all `{roomName}-score-topic`s, and calculate the total score for each player. However, it is more complicated than calculating global scores every time.

After all, Pulsar is not a system that specializes in data aggregation and statistics, so I will consider exporting user score information to other data systems for further analysis and statistics.

Since the messages sent to the score topic are actually key-value pairs, with the key being in the form of `playerName-roomName` and the value being the latest score of that player in that room.

If we can store the messages from `{roomName}-score-topic` in Redis, we can use Redis's `playerName-*` format to get the score information of a player in all rooms. Then, we just need to add them up.

**In this article, we will take Redis as an example to see how to use the Pulsar Connector feature to automatically export data in Pulsar to other systems.**

### Introduction to Pulsar Connector

First, here is the link to the official documentation:

https://pulsar.apache.org/docs/next/io-overview/

There are two types of connectors: sources and sinks. As the name suggests, a source imports data from another system into Pulsar, while a sink imports data from Pulsar into another system.

Actually, I think Pulsar Connector is essentially a Pulsar Function, which holds the client of other data systems, acting as a bridge between Pulsar and other systems.

As mentioned earlier when introducing Pulsar Functions, there are two deployment modes for Functions: a standalone deployment as a service cluster or uploaded deployment to the broker. Therefore, sources and sinks can also be independently deployed or uploaded to the broker. Later, I will show how to start a Redis sink connector locally with `localrun`, which is an independent deployment.

We want to import data from topics into Redis, so we need to find the Redis sink connector on the [Pulsar built-in connector](https://pulsar.apache.org/download/) download page and download it to the `connectors` directory where all Pulsar connectors are stored:

```bash
mkdir connectors
curl -O https://www.apache.org/dyn/mirrors/mirrors.cgi?action=download&filename=pulsar/pulsar-2.11.0/connectors/pulsar-io-redis-2.11.0.nar --output-dir ./connectors
```

### Deploy Redis Sink

First, start a Redis server locally using docker:

```bash
docker pull redis:5.0.5
docker run -d -p 6379:6379 --name my-redis redis:5.0.5 --requirepass "mypassword"
```

Since we want to import data from Pulsar into Redis, we need to provide the necessary information about this Redis service. Therefore, we create a `redis-sink-config.yml` file:

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

Then, we use `localrun` mode to start a Pulsar Connect service locally, running the Redis connector, and tell it which topic to read data from and store it in Redis:

```bash
bin/pulsar-admin sinks localrun \
    --archive connectors/pulsar-io-redis-2.11.0.nar \
    --tenant public \
    --namespace default \
    --name my-redis-sink \
    --sink-config-file redis-sink-config.yaml \
    --topics-pattern '*-score-topic'
```

This command starts the Redis sink connector locally, reads data from all topics with the suffix `-score-topic`, and stores it in Redis.

Next, we can use the following Redis command to query the total score of a player:

```bash
eval "local keys = redis.call('keys',KEYS[1]) ; local sum=0 ; for _,k in ipairs(keys) do sum = sum + tonumber(redis.call('get',k)) end ; return sum" 1 'playName-*'
```