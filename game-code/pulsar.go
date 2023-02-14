package main

import (
	"context"
	"encoding/json"
	"github.com/apache/pulsar-client-go/pulsar"
	log "github.com/sirupsen/logrus"
	"math"
	"reflect"
)

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

// EventMessage is the data in Pulsar
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

type pulsarClient struct {
	roomName, playerName string
	client               pulsar.Client
	producer             pulsar.Producer
	consumer             pulsar.Consumer
	tableView            pulsar.TableView
	consumeCh            chan pulsar.ConsumerMessage
	// exclude type
	exclusiveObstacleConsumer pulsar.Consumer
	// to read the latest obstacle graph
	obstacleReader pulsar.Reader
	// subscribe the obstacle topic,
	closeCh chan struct{}
}

// player action event
func (c *pulsarClient) getEventTopicName() string {
	return c.roomName + "-event-topic"
}

// map update event
func (c *pulsarClient) getMapTopicName() string {
	return c.roomName + "-map-topic"
}

// the name for every player to subscribe event topic
func (c *pulsarClient) getEventSubscriptionName() string {
	return c.playerName + "-event-sub"
}

// the name for every player to subscribe map topic
func (c *pulsarClient) getMapSubscriptionName() string {
	return c.playerName + "-map-sub"
}

// the name for only one player to subscribe map topic, this player will send map update event
func (c *pulsarClient) getUniqueMapSubscriptionName() string {
	return c.roomName + "-map-sub"
}

func (c *pulsarClient) Close() {
	c.producer.Close()
	c.consumer.Unsubscribe()
	c.consumer.Close()
	c.client.Close()
	c.tableView.Close()
	close(c.consumeCh)
	close(c.closeCh)
}

func newPulsarClient(roomName, playerName string) *pulsarClient {
	topicName := roomName + "-event-topic"
	subscriptionName := playerName
	client, err := pulsar.NewClient(readClientOptionFromYaml())
	if err != nil {
		log.Fatal("[newPulsarClient]", err)
	}

	// player event topicName
	producer, err := client.CreateProducer(pulsar.ProducerOptions{
		Topic:           topicName,
		DisableBatching: true,
		// use schema to confirm the structure of message
		Schema: pulsar.NewJSONSchema(eventJsonSchemaDef, nil),
	})
	if err != nil {
		log.Fatal(err)
	}
	consumeCh := make(chan pulsar.ConsumerMessage)
	consumer, err := client.Subscribe(pulsar.ConsumerOptions{
		Topic:            topicName,
		SubscriptionName: subscriptionName,
		Type:             pulsar.Exclusive,
		MessageChannel:   consumeCh,
		// use schema to confirm the structure of message
		Schema: pulsar.NewJSONSchema(eventJsonSchemaDef, nil),
	})
	if err != nil {
		log.Fatal("this player has logged in")
	}
	// only handle new event
	err = consumer.Seek(pulsar.LatestMessageID())
	if err != nil {
		log.Fatal(err)
	}

	tableView, err := client.CreateTableView(pulsar.TableViewOptions{
		Topic:           roomName + "-score-topic",
		Schema:          pulsar.NewStringSchema(nil),
		SchemaValueType: reflect.TypeOf(""),
	})
	if err != nil {
		log.Fatal(err)
	}

	return &pulsarClient{
		tableView:  tableView,
		playerName: playerName,
		roomName:   roomName,
		client:     client,
		producer:   producer,
		consumer:   consumer,
		consumeCh:  consumeCh,
		closeCh:    make(chan struct{}),
	}
}

func readClientOptionFromYaml() pulsar.ClientOptions {
	clientOptions := pulsar.ClientOptions{
		URL: pulsarConfig.BrokerUrl,
	}
	if pulsarConfig.OAuth.Enabled {
		oauth := pulsar.NewAuthenticationOAuth2(map[string]string{
			"type":       "client_credentials",
			"issuerUrl":  pulsarConfig.OAuth.IssuerURL,
			"audience":   pulsarConfig.OAuth.Audience,
			"privateKey": pulsarConfig.OAuth.PrivateKey,
		})
		clientOptions.Authentication = oauth
	}
	return clientOptions
}

// try grab exclusive consumer, if success, send new random graph
func (c *pulsarClient) canUpdateObstacles() bool {
	// every minute update random obstacle
	if c.exclusiveObstacleConsumer != nil {
		return true
	}
	// all player will get same subscription name
	obstacleSubscriptionName := c.getUniqueMapSubscriptionName()
	obstacleConsumerCh := make(chan pulsar.ConsumerMessage)
	obstacleConsumer, err := c.client.Subscribe(pulsar.ConsumerOptions{
		Topic: c.getMapTopicName(),
		// all player clients should have same subscription name
		// then Exclusive type can work
		SubscriptionName: obstacleSubscriptionName,
		// only one consumer can subscribe obstacle topic
		Type:                        pulsar.Exclusive,
		MessageChannel:              obstacleConsumerCh,
		SubscriptionInitialPosition: pulsar.SubscriptionPositionLatest,
	})
	if err != nil {
		// subscription already has other consumers
		return false
	} else {
		c.exclusiveObstacleConsumer = obstacleConsumer
		return true
	}

	// now, this player is the first consumer, update the map obstacle topic
	//c.updateObstacleTopic(newMap)

}

func (c *pulsarClient) readLatestEvent(topicName string) Event {
	reader, err := c.client.CreateReader(pulsar.ReaderOptions{
		Topic: topicName,
		// get the latest message
		StartMessageID:          pulsar.LatestMessageID(),
		StartMessageIDInclusive: true,
	})
	if err != nil {
		log.Error("[readLatestEvent]", err)
	}
	defer reader.Close()

	if reader.HasNext() {
		msg, err := reader.Next(context.Background())
		if err != nil {
			log.Error("[readLatestEvent]", err)
		}
		actionMsg := EventMessage{}

		err = json.Unmarshal(msg.Payload(), &actionMsg)
		return convertMsgToEvent(&actionMsg)
	}
	return nil
}

// start to receive message from pulsar, forwarding to receiveCh
func (c *pulsarClient) start(in chan Event) chan Event {
	// All players' action can be received from this channel
	outCh := make(chan Event)
	go func() {
		for {
			select {
			// receive message from pulsar, forwarding to outCh
			case cm := <-c.consumeCh:
				msg := cm.Message
				if msg == nil {
					log.Warning("receive a nil message")
					break
				}
				actionMsg := EventMessage{}
				err := msg.GetSchemaValue(&actionMsg)
				if err != nil {
					log.Error("[start]", err)
					break
				}
				l := math.Min(float64(len(msg.Payload())), 100)
				log.Info("receive message from pulsar:\n", string(msg.Payload())[:int(l)])
				cm.Ack(msg)
				outCh <- convertMsgToEvent(&actionMsg)

			// need to send message to pulsar
			case action := <-in:
				if action == nil {
					log.Warning("send a nil message, maybe channel has been closed")
					break
				}
				actionMsg := convertEventToMsg(action)
				_, err := c.producer.Send(context.Background(), &pulsar.ProducerMessage{
					Value: actionMsg,
				})
				if err != nil {
					log.Error("send msg failed:", err)
					break
				}
				//log.Info("send message to pulsar:\n", string(bytes))

			case <-c.closeCh:
				return
			}
		}
	}()

	return outCh
}

func convertEventToMsg(action Event) *EventMessage {
	var msg *EventMessage
	switch t := action.(type) {
	case *UserMoveEvent:
		msg = &EventMessage{
			Type:   UserMoveEventType,
			Name:   t.name,
			Avatar: t.avatar,
			X:      t.pos.X,
			Y:      t.pos.Y,
			Alive:  t.alive,
		}
	case *UserJoinEvent:
		msg = &EventMessage{
			Type:   UserJoinEventType,
			Name:   t.name,
			Avatar: t.avatar,
			X:      t.pos.X,
			Y:      t.pos.Y,
			Alive:  t.alive,
			List:   t.Obstacles,
		}
	case *UserDeadEvent:
		msg = &EventMessage{
			Type:   UserDeadEventType,
			Name:   t.name,
			Avatar: t.avatar,
			X:      t.pos.X,
			Y:      t.pos.Y,
			// record the killer player name
			Comment: t.killer,
			Alive:   false,
		}
	case *UserReviveEvent:
		msg = &EventMessage{
			Type:   UserReviveEventType,
			Name:   t.name,
			Avatar: t.avatar,
			X:      t.pos.X,
			Y:      t.pos.Y,
			Alive:  true,
		}
	case *SetBombEvent:
		msg = &EventMessage{
			Type: SetBombEventType,
			Name: t.bombName,
			X:    t.pos.X,
			Y:    t.pos.Y,
		}
	case *BombMoveEvent:
		msg = &EventMessage{
			Type: MoveBombEventType,
			Name: t.bombName,
			X:    t.pos.X,
			Y:    t.pos.Y,
		}
	case *ExplodeEvent:
		msg = &EventMessage{
			Type: ExplodeEventType,
			Name: t.bombName,
			X:    t.pos.X,
			Y:    t.pos.Y,
		}
	case *UndoExplodeEvent:
		msg = &EventMessage{
			Type: UndoExplodeEventType,
			X:    t.pos.X,
			Y:    t.pos.Y,
		}
	case *UpdateMapEvent:
		msg = &EventMessage{
			Type: UpdateObstacleEventType,
			List: t.Obstacles,
		}
	}
	return msg
}

func convertMsgToEvent(msg *EventMessage) Event {
	info := &playerInfo{
		name:   msg.Name,
		avatar: msg.Avatar,
		pos: Position{
			X: msg.X,
			Y: msg.Y,
		},
		alive: msg.Alive,
	}
	switch msg.Type {
	case UserJoinEventType:
		return &UserJoinEvent{
			playerInfo: info,
			Obstacles:  msg.List,
		}
	case SetBombEventType:
		return &SetBombEvent{
			bombName: msg.Name,
			pos:      info.pos,
		}
	case MoveBombEventType:
		return &BombMoveEvent{
			bombName: msg.Name,
			pos:      info.pos,
		}
	case UserMoveEventType:
		return &UserMoveEvent{
			playerInfo: info,
		}
	case UserDeadEventType:
		return &UserDeadEvent{
			playerInfo: info,
			killer:     msg.Comment,
		}
	case UserReviveEventType:
		return &UserReviveEvent{
			playerInfo: info,
		}
	case ExplodeEventType:
		return &ExplodeEvent{
			bombName: msg.Name,
			pos:      info.pos,
		}
	case UndoExplodeEventType:
		return &UndoExplodeEvent{
			pos: info.pos,
		}
	case UpdateObstacleEventType:
		return &UpdateMapEvent{
			Obstacles: msg.List,
		}
	}
	return nil
}
