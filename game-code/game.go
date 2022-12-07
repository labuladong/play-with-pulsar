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

	bombLength = 8
	// there are indestructibleObstacleCount obstacles in map
	indestructibleObstacleCount = totalGridCount / 5
	destructibleObstacleCount   = totalGridCount / 4
	// bomb explode after explodeTime second
	explodeTime = 2
	// flame disappear after flameTime second
	flameTime = 2
	// obstacle update every updateObstacleTime second
	updateObstacleTime = 30
	// random bomb appear every randomBombTime second
	randomBombTime = 2
)

type ObstacleType int

const (
	destructibleObstacleType   ObstacleType = 1
	indestructibleObstacleType ObstacleType = 2
)

type Game struct {
	// scores of every player
	scores *lru.Cache

	// local player playerName
	localPlayerName string
	nameToPlayers   map[string]*playerInfo
	posToPlayers    map[Position]*playerInfo

	nameToBombs map[string]*Bomb
	posToBombs  map[Position]*Bomb

	// although all events are handled serially,
	// the flameMap may be updated by multiple go routines
	flameLock sync.RWMutex
	flameMap  map[Position]*Bomb

	// protect for map update and destroy obstacles
	obstacleLock sync.RWMutex
	// two types of obstacle
	obstacleMap map[Position]ObstacleType

	// audio player
	audioContext *audio.Context
	deadPlayer   *audio.Player

	// receive event to redraw our game
	eventCh chan Event
	// send local event to send to pulsar
	sendCh chan Event

	client *pulsarClient
}

func (g *Game) Close() {
	g.client.Close()
	close(g.sendCh)
	close(g.eventCh)
}

func (g *Game) Update() error {
	// listen to event
	select {
	case event := <-g.eventCh:
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
	var bomb = false
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		dir = dirLeft
	} else if inpututil.IsKeyJustPressed(ebiten.KeyArrowRight) || inpututil.IsKeyJustPressed(ebiten.KeyD) {
		dir = dirRight
	} else if inpututil.IsKeyJustPressed(ebiten.KeyArrowDown) || inpututil.IsKeyJustPressed(ebiten.KeyS) {
		dir = dirDown
	} else if inpututil.IsKeyJustPressed(ebiten.KeyArrowUp) || inpututil.IsKeyJustPressed(ebiten.KeyW) {
		dir = dirUp
	} else if inpututil.IsKeyJustPressed(ebiten.KeySpace) {
		bomb = true
	} else if inpututil.IsKeyJustPressed(ebiten.KeyR) {
		// revive
		event := &UserReviveEvent{
			playerInfo: info,
		}
		g.sendAsync(event)
	} else if inpututil.IsKeyJustPressed(ebiten.KeyEscape) {
		// quit game
	}

	g.flameLock.RLock()
	if val, ok := g.flameMap[localPlayer.pos]; ok && val != nil && localPlayer.alive {
		localPlayer.alive = false
		// dead due to boom
		event := &UserDeadEvent{
			playerInfo: info,
			// the player who set the bomb
			killer: val.playerName,
		}
		g.sendAsync(event)
	}
	g.flameLock.RUnlock()

	if dir != dirNone && localPlayer.alive {
		nextPlayerPos := getNextPosition(localPlayer.pos, dir)
		info.pos = nextPlayerPos
		event := &UserMoveEvent{
			playerInfo: info,
		}
		g.sendAsync(event)
		if bomb, ok := g.posToBombs[nextPlayerPos]; ok {
			// push the bomb
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

	// set bomb on empty block
	if _, ok := g.posToBombs[localPlayer.pos]; !ok && bomb {
		info.pos = localPlayer.pos
		event := &SetBombEvent{
			bombName: info.name + "-" + randStringRunes(5),
			pos:      info.pos,
		}
		g.sendAsync(event)
	}

	return nil
}

// setBomb create a bomb with trigger channel
func (g *Game) setBombWithTrigger(bombName string, position Position, trigger chan struct{}) string {
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

func (g *Game) removeBomb(bombName string) {
	if bomb, ok := g.nameToBombs[bombName]; ok {
		delete(g.nameToBombs, bombName)
		if _, ok = g.posToBombs[bomb.pos]; ok {
			delete(g.posToBombs, bomb.pos)
		}
	}
}

func (g *Game) sendAsync(event Event) {
	// don't block
	select {
	case g.sendCh <- event:
	default:
	}
}

func (g *Game) Draw(screen *ebiten.Image) {
	// todo replace Rect with images

	for pos, _ := range g.posToBombs {
		ebitenutil.DrawRect(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), gridSize, gridSize, bombColor)
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

	g.flameLock.RLock()
	for pos, val := range g.flameMap {
		// only val > 0 means flame
		if val != nil {
			ebitenutil.DrawRect(screen, float64(pos.X*gridSize), float64(pos.Y*gridSize), gridSize, gridSize, flameColor)
		}
	}
	g.flameLock.RUnlock()
}

func (g *Game) Layout(outsideWidth, outsideHeight int) (int, int) {
	return screenWidth, screenHeight
}

func (g *Game) explode(bomb *Bomb) {
	pos := bomb.pos
	if _, ok := g.posToBombs[pos]; !ok {
		return
	}
	// remove the bomb in the grid
	g.removeBomb(bomb.bombName)

	// calculate flames
	g.obstacleLock.RLock()
	var positions []Position
	for i := pos.X - 1; i >= pos.X-bombLength; i-- {
		p := Position{X: i, Y: pos.Y}
		if t, ok := g.obstacleMap[p]; ok && t == indestructibleObstacleType {
			break
		}
		positions = append(positions, p)
	}
	for i := pos.X; i <= pos.X+bombLength; i++ {
		p := Position{X: i, Y: pos.Y}
		if t, ok := g.obstacleMap[p]; ok && t == indestructibleObstacleType {
			break
		}
		positions = append(positions, p)
	}
	for j := pos.Y - 1; j >= pos.Y-bombLength; j-- {
		p := Position{X: pos.X, Y: j}
		if t, ok := g.obstacleMap[p]; ok && t == indestructibleObstacleType {
			break
		}
		positions = append(positions, p)
	}
	for j := pos.Y; j <= pos.Y+bombLength; j++ {
		p := Position{X: pos.X, Y: j}
		if t, ok := g.obstacleMap[p]; ok && t == indestructibleObstacleType {
			break
		}
		positions = append(positions, p)
	}
	g.obstacleLock.RUnlock()

	g.flameLock.Lock()
	g.obstacleLock.Lock()
	defer g.flameLock.Unlock()
	defer g.obstacleLock.Unlock()
	for _, position := range positions {
		if !validCoordinate(position) {
			continue
		}
		// set value to the bomb pointer
		g.flameMap[position] = bomb
		if t, ok := g.obstacleMap[position]; ok && t == destructibleObstacleType {
			delete(g.obstacleMap, position)
		}
		// if a player standing there, dead
		// todo
		//if player, ok := g.posToPlayers[position]; ok {
		//	player.alive = false
		//}
	}

}

func (g *Game) unExplode(pos Position) {
	var positions []Position
	for i := pos.X - bombLength; i < pos.X+bombLength+1; i++ {
		positions = append(positions, Position{X: i, Y: pos.Y})
	}
	for j := pos.Y - bombLength; j < pos.Y+bombLength+1; j++ {
		positions = append(positions, Position{X: pos.X, Y: j})
	}
	g.flameLock.Lock()
	defer g.flameLock.Unlock()
	bomb := g.flameMap[pos]
	for _, position := range positions {
		if !validCoordinate(position) {
			continue
		}
		//if val, ok := g.flameMap[position]; !ok || val <= 0 {
		//	// the unexplode event only has position info,
		//	// so history event may trigger unexplode event unexpectedly,
		//	// so we ensure all grids is flame, then trigger this event
		//	return
		//}
		if _, ok := g.flameMap[position]; bomb == nil || (ok) {
			g.flameMap[position] = nil
		}
	}
}

// produce a random bomb every second
func (g *Game) randomBombsEnable() {
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
func newGame(playerName, roomName string) *Game {
	info := &playerInfo{
		name:   playerName,
		avatar: "fff",
		pos: Position{
			X: 0,
			Y: 0,
		},
		alive: true,
	}
	client := newPulsarClient(roomName, playerName)
	cache, err := lru.New(5)
	g := &Game{
		scores:          cache,
		localPlayerName: playerName,
		nameToPlayers:   map[string]*playerInfo{},
		posToPlayers:    map[Position]*playerInfo{},
		nameToBombs:     map[string]*Bomb{},
		posToBombs:      map[Position]*Bomb{},
		flameMap:        map[Position]*Bomb{},
		eventCh:         nil,
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
	g.sendCh = make(chan Event, 20)
	// use this channel to receive from pulsar
	g.eventCh = g.client.start(g.sendCh)

	return g
}
