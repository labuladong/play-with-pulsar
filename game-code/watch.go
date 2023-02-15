package main

import (
	"context"
	"encoding/json"
	"github.com/apache/pulsar-client-go/pulsar"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	log "github.com/sirupsen/logrus"
	"image/color"
	"os"
	"time"
)

type GameReplay struct {
	*BombGame
	cancel context.CancelFunc
}

func NewGameReplay(roomName, at string) *GameReplay {
	ctx, cancel := context.WithCancel(context.Background())
	game := &BombGame{
		nameToPlayers:  map[string]*playerInfo{},
		posToPlayers:   map[Position]*playerInfo{},
		nameToBombs:    map[string]*Bomb{},
		posToBombs:     map[Position]*Bomb{},
		explodingBombs: map[Position]*Bomb{},
		flameMap:       map[Position]*Bomb{},
		receiveCh:      readAllMessage(ctx, roomName, at),
	}
	return &GameReplay{
		BombGame: game,
		cancel:   cancel,
	}
}

func (g *GameReplay) Close() {
	close(g.BombGame.receiveCh)
	g.cancel()
}

func readAllMessage(ctx context.Context, roomName, at string) chan Event {
	topicName := roomName + "-event-topic"
	var start pulsar.MessageID
	if at == "latest" {
		start = pulsar.LatestMessageID()
	} else {
		start = pulsar.EarliestMessageID()
	}
	client, err := pulsar.NewClient(readClientOptionFromYaml())
	reader, err := client.CreateReader(pulsar.ReaderOptions{
		Topic:          topicName,
		StartMessageID: start,
		Schema:         pulsar.NewJSONSchema(eventJsonSchemaDef, nil),
	})
	if err != nil {
		log.Error("[Playback]", err)
	}

	ch := make(chan Event)
	go func() {
		// play back the game, don't too fast
		tick := time.Tick(200 * time.Millisecond)
		for true {
			msg, _ := reader.Next(ctx)
			var actionMsg EventMessage
			if msg != nil {
				err = json.Unmarshal(msg.Payload(), &actionMsg)
				if err != nil {
					log.Error("[Playback][json.Unmarshal]", err)
					continue
				}
			}
			select {
			case <-ctx.Done():
				reader.Close()
				return
			case <-tick:
				ch <- convertMsgToEvent(&actionMsg)
			}
		}
	}()
	return ch
}

func (g *GameReplay) Update() error {
	// listen to event
	select {
	case event := <-g.receiveCh:
		event.handle(g.BombGame)
	default:
	}
	if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.Close()
		return os.ErrClosed
	}
	return nil
}

func (g *GameReplay) Draw(screen *ebiten.Image) {

	for pos, _ := range g.posToBombs {
		ebitenutil.DrawCircle(screen, float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize+gridSize/2), gridSize/2, bombColor)
	}

	for pos, t := range g.obstacleMap {
		if t == destructibleObstacleType {
			ebitenutil.DrawRect(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), gridSize, gridSize, destructibleObstacleColor)
		} else {
			ebitenutil.DrawRect(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), gridSize, gridSize, indestructibleObstacleColor)
		}
	}

	for _, player := range g.nameToPlayers {
		var userColor color.RGBA
		if player.alive {
			userColor = playerColor
		} else {
			userColor = deadPlayerColor
		}
		ebitenutil.DrawRect(screen, float64(player.pos.X*gridSize), float64(player.pos.Y*gridSize), gridSize, gridSize, userColor)
	}

	for pos, val := range g.flameMap {
		// draw the flame
		if val != nil {
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize+gridSize/2), float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize/2), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize+gridSize/2), float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize/2), flameColor)
		}
	}

	ebitenutil.DebugPrintAt(screen, "You are in watch mode", 0, screenHeight-scoreBarHeight+10)
}
