package main

import (
	"bytes"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/audio"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	raudio "github.com/hajimehoshi/ebiten/v2/examples/resources/audio"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	lru "github.com/hashicorp/golang-lru"
	log "github.com/sirupsen/logrus"
	"image/color"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	scoreBarHeight = 30

	screenWidth  = 600
	screenHeight = 500 + scoreBarHeight

	gridSize           = 20
	xGridCountInScreen = screenWidth / gridSize
	// display score board at bottom
	yGridCountInScreen = (screenHeight - scoreBarHeight) / gridSize
	totalGridCount     = xGridCountInScreen * yGridCountInScreen

	bombLength = 6
	// there are indestructibleObstacleCount obstacles in map
	indestructibleObstacleCount = totalGridCount / 5
	destructibleObstacleCount   = totalGridCount / 4
	// bomb explode after explodeTime second
	explodeTime = 2
	// flame disappear after flameTime second
	flameTime = 2
	// obstacle update every updateObstacleTime second
	updateObstacleTime = 60
	// random bomb appear every randomBombTime second
	randomBombTime = 2
)

type ObstacleType int

const (
	destructibleObstacleType   ObstacleType = 1
	indestructibleObstacleType ObstacleType = 2
)

type BombGame struct {
	// scores of every player
	scores *lru.Cache

	// local player playerName
	localPlayerName string
	nameToPlayers   map[string]*playerInfo
	posToPlayers    map[Position]*playerInfo

	nameToBombs map[string]*Bomb
	posToBombs  map[Position]*Bomb

	// the bombs that are exploding (flame on grids)
	explodingBombs map[Position]*Bomb

	// this map is calculated by explodingBombs when explode or unexplode
	flameMap map[Position]*Bomb

	// protect for map update and destroy obstacles
	// because Update() will read it and explode event will write it
	// these two condition are in different thread
	obstacleLock sync.RWMutex
	// two types of obstacle
	obstacleMap map[Position]ObstacleType

	// audio player
	audioContext *audio.Context
	deadPlayer   *audio.Player

	// receive event to redraw our game
	receiveCh chan Event
	// send local event to send to pulsar
	sendCh chan Event

	client *pulsarClient
}

func (g *BombGame) Close() {
	g.client.Close()
	close(g.sendCh)
	close(g.receiveCh)
}

func (g *BombGame) Update() error {
	// listen to event
	select {
	case event := <-g.receiveCh:
		event.handle(g)
	default:
	}

	localPlayer := g.nameToPlayers[g.localPlayerName]

	info := &playerInfo{
		name:   localPlayer.name,
		pos:    localPlayer.pos,
		avatar: localPlayer.avatar,
		alive:  localPlayer.alive,
	}

	var dir = dirNone
	var setBomb = false
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		dir = dirLeft
	} else if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		dir = dirRight
	} else if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		dir = dirDown
	} else if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		dir = dirUp
	} else if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		setBomb = true
	} else if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		// revive
		event := &UserReviveEvent{
			playerInfo: info,
		}
		g.sendAsync(event)
	} else if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		g.Close()
		return os.ErrClosed
	}

	// local player dead due to boom
	if val, ok := g.flameMap[localPlayer.pos]; ok && val != nil && localPlayer.alive {
		localPlayer.alive = false
		event := &UserDeadEvent{
			playerInfo: info,
			// the player who set the bomb
			killer: val.playerName,
		}
		g.sendAsync(event)
	}

	if dir != dirNone && localPlayer.alive {
		nextPlayerPos := getNextPosition(localPlayer.pos, dir)
		info.pos = nextPlayerPos
		event := &UserMoveEvent{
			playerInfo: info,
		}
		// handle user move
		g.sendAsync(event)

		if bomb, ok := g.posToBombs[nextPlayerPos]; ok {
			// handle push the bomb
			go func(bomb *Bomb, direction Direction) {
				nextPos := getNextPosition(bomb.pos, direction)
				ticker := time.NewTicker(time.Second / 2)
				defer ticker.Stop()
				for i := 0; i < 8; i++ {
					select {
					case <-bomb.explodeCh:
						// bomb exploded, stop
						return
					case <-ticker.C:
						g.obstacleLock.RLock()
						if _, ok = g.obstacleMap[nextPos]; !validCoordinate(nextPos) || ok {
							// move to border or obstacle, stop
							g.obstacleLock.RUnlock()
							return
						}
						g.obstacleLock.RUnlock()

						event := &BombMoveEvent{
							bombName: bomb.bombName,
							pos:      nextPos,
						}
						g.sendAsync(event)
						nextPos = getNextPosition(nextPos, direction)
					}
				}
			}(bomb, dir)
		}
	}

	// handle set bomb on empty block
	if _, ok := g.posToBombs[localPlayer.pos]; !ok && setBomb {
		info.pos = localPlayer.pos
		event := &SetBombEvent{
			bombName: info.name + "-" + randStringRunes(5),
			pos:      info.pos,
		}
		g.sendAsync(event)
	}

	return nil
}

func (g *BombGame) join() {
	info := g.nameToPlayers[g.localPlayerName]
	newMapList := g.genRandomObstacleList()
	g.sendAsync(&UserJoinEvent{
		playerInfo: info,
		Obstacles:  newMapList,
	})
}

// 生成随机地图（防止覆盖已知的玩家）
func (g *BombGame) genRandomObstacleList() []int {
	indestructibleObstacles := sample(totalGridCount, indestructibleObstacleCount)

	var destructibleObstacles []int
	for _, v := range sample(totalGridCount, indestructibleObstacleCount+destructibleObstacleCount) {
		// ignore efficiency, just keep simple, brutal force deduplicate
		if !sliceContains(indestructibleObstacles, v) {
			// for destructibleObstacleType, we use negative number to present
			destructibleObstacles = append(destructibleObstacles, -v)
		}
	}
	obstacles := append(indestructibleObstacles, destructibleObstacles...)
	var dirs = [][]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}, {0, 0}}

	for _, info := range g.nameToPlayers {
		for _, d := range dirs {
			code := encodeXY(info.pos.X+d[0], info.pos.Y+d[1])
			if sliceContains(obstacles, code) {
				sliceRemove(obstacles, code)
			} else if sliceContains(obstacles, -code) {
				sliceRemove(obstacles, -code)
			}
		}
	}
	return obstacles
}

// setBomb create a bomb with trigger channel
func (g *BombGame) setBombWithTrigger(bombName string, position Position, trigger chan struct{}) string {
	bomb := &Bomb{
		bombName:   bombName,
		playerName: strings.Split(bombName, "-")[0],
		pos:        position,
		explodeCh:  trigger,
	}
	g.nameToBombs[bomb.bombName] = bomb
	g.posToBombs[bomb.pos] = bomb
	return bomb.bombName
}

func (g *BombGame) removeBomb(bombName string) {
	if bomb, ok := g.nameToBombs[bombName]; ok {
		delete(g.nameToBombs, bombName)
		if _, ok = g.posToBombs[bomb.pos]; ok {
			delete(g.posToBombs, bomb.pos)
		}
	}
}

func (g *BombGame) sendAsync(event Event) {
	// don't block
	select {
	case g.sendCh <- event:
	default:
		log.Warning("[sendAsync] there is event being abandoned")
	}
}

func (g *BombGame) Draw(screen *ebiten.Image) {
	// todo replace Rect with images

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

	if !g.nameToPlayers[g.localPlayerName].alive {
		ebitenutil.DebugPrint(screen, fmt.Sprintf("You are dead, press R to revive."))
	}

	scoreStr := strings.Builder{}
	scoreStr.WriteString("scores: ")
	for _, k := range g.scores.Keys() {
		score, ok := g.scores.Get(k)
		if !ok {
			continue
		}
		scoreStr.WriteString(k.(string))
		scoreStr.WriteString(" = ")
		scoreStr.WriteString(score.(string) + "; ")
	}
	// print the score of all players
	ebitenutil.DebugPrintAt(screen, scoreStr.String(), 0, screenHeight-scoreBarHeight+10)

	for pos, val := range g.flameMap {
		// draw the flame
		if val != nil {
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize+gridSize/2), float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize/2), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize+gridSize/2), float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize+gridSize), flameColor)
			ebitenutil.DrawLine(screen, float64(pos.X*gridSize+gridSize/2), float64(pos.Y*gridSize), float64(pos.X*gridSize+gridSize), float64(pos.Y*gridSize+gridSize/2), flameColor)
			//ebitenutil.DrawRect(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), gridSize, gridSize, flameColor)
		}
	}
}

func (g *BombGame) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

// produce a random bomb every second
func (g *BombGame) randomBombsEnable() {
	go func() {
		// every one seconds, generate a new bomb
		ticker := time.NewTicker(time.Second * randomBombTime)
		for {
			select {
			case <-ticker.C:
				randomPos := Position{
					X: rand.Intn(xGridCountInScreen),
					Y: rand.Intn(yGridCountInScreen),
				}
				if _, ok := g.obstacleMap[randomPos]; ok {
					continue
				}
				if _, ok := g.posToBombs[randomPos]; ok {
					continue
				}
				g.sendAsync(&SetBombEvent{
					bombName: "random-" + randStringRunes(5),
					pos:      randomPos,
				})
			}
		}
	}()
}

// playerName will be the subscription name
// roomName will be the topic name
func newGame(playerName, roomName string) *BombGame {
	info := &playerInfo{
		name:   playerName,
		avatar: "fff",
		pos: Position{
			X: rand.Intn(xGridCountInScreen),
			Y: rand.Intn(yGridCountInScreen),
		},
		alive: true,
	}
	client := newPulsarClient(roomName, playerName)
	cache, err := lru.New(5)
	g := &BombGame{
		scores:          cache,
		localPlayerName: playerName,
		nameToPlayers:   map[string]*playerInfo{},
		posToPlayers:    map[Position]*playerInfo{},
		nameToBombs:     map[string]*Bomb{},
		posToBombs:      map[Position]*Bomb{},
		explodingBombs:  map[Position]*Bomb{},
		flameMap:        map[Position]*Bomb{},
		receiveCh:       nil,
		sendCh:          nil,
		client:          client,
	}

	// pulsar tableview update scores of every player
	client.tableView.ForEachAndListen(func(playerName string, i interface{}) error {
		score := *i.(*string)
		g.scores.Add(playerName, score)
		return nil
	})

	// init audio player
	jabD, err := wav.DecodeWithoutResampling(bytes.NewReader(raudio.Jab_wav))
	g.audioContext = audio.NewContext(48000)
	g.deadPlayer, err = g.audioContext.NewPlayer(jabD)
	if err != nil {
		log.Fatal(err)
	}

	// init local player
	g.nameToPlayers[info.name] = info
	g.posToPlayers[info.pos] = info

	// use this channel to send to pulsar
	g.sendCh = make(chan Event, 50)
	// use this channel to receive from pulsar
	g.receiveCh = g.client.start(g.sendCh)
	g.join()

	// handle obstacle update
	go func() {
		for {
			select {
			case <-time.Tick(time.Second * updateObstacleTime):
				// every minute update random obstacle
				if g.client.canUpdateObstacles() {
					g.sendAsync(&UpdateMapEvent{
						Obstacles: g.genRandomObstacleList(),
					})
				}
			}
		}
	}()

	return g
}
