# Developing Multiplayer Games with Pulsar Part 4: Introduction to Pulsar Installation and Use

> Note: This article is part 4 of "Developing Multiplayer Online Games with Pulsar". For source code and complete documentation, please visit my GitHub repository play-with-pulsar, as well as my list of articles.

For the most detailed deployment instructions, please refer to the official website:

https://pulsar.apache.org/

Here, I will introduce the architectural principles of Pulsar, which will make it easy to understand the various deployment methods of Pulsar.

### Key Components of a Pulsar Cluster

As shown in the diagram below, a Pulsar cluster consists of one or more broker nodes, one or more bookie nodes, and one or more zookeeper nodes:

![](https://labuladong.github.io/pictures/pulsar-game/pulsar-cluster.jpeg)

The broker node is responsible for computation (such as client connections, data caching/blocking, etc.), the bookie node is responsible for storage (persistent and consistent storage of data), and the zookeeper stores the metadata of these nodes. It's as simple as that.

If you want to quickly verify our Bomberman game, you can follow the steps of [Run a standalone Pulsar cluster locally](https://pulsar.apache.org/docs/next/getting-started-standalone/) on the official website to start a standalone mode of Pulsar cluster locally:

```shell
./bin/pulsar standalone
```
As you may have guessed, standalone mode actually packages up bookie, broker, zk and starts all these services at once.

However, the latest Pulsar installation package includes the optimization of PIP-117 for standalone mode, which can avoid starting zookeeper and thus optimize the performance of standalone mode.

But because we are learning Pulsar, I still recommend starting zookeeper, so that we can use related tools of zookeeper to view metadata.

We can avoid the optimization of PIP-117 by setting the `PULSAR_STANDALONE_USE_ZOOKEEPER` environment variable and start zookeeper to store the metadata of Pulsar cluster:

```shell
PULSAR_STANDALONE_USE_ZOOKEEPER=1 ./bin/pulsar standalone
```

### Pulsar Cluster Configuration Files

As mentioned above, a Pulsar cluster is composed of broker nodes, bookie nodes, and zookeeper nodes, so each type of node has its own configuration file.

The broker node's configuration file is `conf/broker.conf`, the bookie node's configuration file is `conf/bookkeeper.conf`, and the zookeeper node's configuration file is `conf/zookeeper.conf`.

The standalone mode of Pulsar has its own configuration file `conf/standalone.conf`. Because standalone mode starts multiple nodes at once, it can be understood as a combination of the above three configuration files.

What do all these configuration fields mean? We won't go into that for now. We'll come back to them later when we encounter specific problems in game development.