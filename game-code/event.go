package main

import (
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

const (
	UserMoveEventType       = "UserMoveEvent"
	UserJoinEventType       = "UserJoinEvent"
	UserDeadEventType       = "UserDeadEvent"
	UserReviveEventType     = "UserReviveEvent"
	SetBombEventType        = "SetBombEvent"
	MoveBombEventType       = "BombMoveEvent"
	ExplodeEventType        = "ExplodeEvent"
	UndoExplodeEventType    = "UndoExplodeEvent"
	UpdateObstacleEventType = "UpdateMapEvent"
)

// Event make change on Graph
type Event interface {
	handle(game *BombGame)
}

// UserMoveEvent makes playerInfo move
type UserMoveEvent struct {
	*playerInfo
}

func (e *UserMoveEvent) handle(g *BombGame) {
	log.Info("handle UserMoveEvent")
	if !validCoordinate(e.pos) {
		// move out of boarder
		return
	}

	if _, ok := g.obstacleMap[e.pos]; ok {
		// move to obstacle
		return
	}
	if player, ok := g.nameToPlayers[e.name]; ok && !player.alive {
		// already dead
		return
	}
	g.nameToPlayers[e.name] = e.playerInfo
	g.posToPlayers[e.pos] = e.playerInfo
}

type UserDeadEvent struct {
	*playerInfo
	killer string
}

func (e *UserDeadEvent) handle(game *BombGame) {
	if _, ok := game.nameToPlayers[e.name]; ok {
		game.nameToPlayers[e.name].alive = false
	}
}

type UserReviveEvent struct {
	*playerInfo
}

func (e *UserReviveEvent) handle(game *BombGame) {
	game.nameToPlayers[e.name] = e.playerInfo
	game.nameToPlayers[e.name].alive = true
}

// UserJoinEvent new user join room, must update the map to ensure
// all player have the consistent start view
type UserJoinEvent struct {
	*playerInfo
	Obstacles []int
}

func (e *UserJoinEvent) handle(game *BombGame) {
	// 1. display the new user on screen
	game.nameToPlayers[e.name] = e.playerInfo
	game.posToPlayers[e.pos] = e.playerInfo
	// 2. update the obstacle map
	game.obstacleMap = genObstacleMapFromList(e.Obstacles, nil)
}

type SetBombEvent struct {
	bombName string
	pos      Position
}

func (e *SetBombEvent) handle(game *BombGame) {
	log.Info("handle SetBombEvent")
	if _, ok := game.obstacleMap[e.pos]; ok {
		// set on obstacle
		return
	}
	bombName := game.setBombWithTrigger(e.bombName, e.pos, make(chan struct{}))
	if strings.HasPrefix(bombName, "random-") ||
		strings.HasPrefix(bombName, game.localPlayerName+"-") {
		// send explode message
		go func() {
			// bomb will explode after 2 seconds
			bombTimer := time.NewTimer(explodeTime * time.Second)
			<-bombTimer.C
			game.sendAsync(&ExplodeEvent{
				bombName: bombName,
			})
		}()
	}
}

type ExplodeEvent struct {
	bombName string
	pos      Position
}

func (e *ExplodeEvent) handle(game *BombGame) {
	log.Info("handle ExplodeEvent")
	bomb, ok := game.nameToBombs[e.bombName]
	if !ok {
		// bombs are set to the same place will cause this situation
		return
	}
	// async notify, if this bomb is moving, it will stop moving
	select {
	case bomb.explodeCh <- struct{}{}:
	default:
	}

	bombPos := bomb.pos
	if _, ok = game.posToBombs[bombPos]; !ok {
		return
	}
	// remove the bomb in the grid
	game.removeBomb(bomb.bombName)
	// just mark the exploding bomb position, Draw() will generate the flame
	game.explodingBombs[bombPos] = bomb

	// explode may destroy obstacles, update obstacleMap
	game.obstacleLock.Lock()
	defer game.obstacleLock.Unlock()
	getExplodeFlame(bombPos, func(p Position) bool {
		if t, ok := game.obstacleMap[p]; ok {
			if t == indestructibleObstacleType {
				return false
			} else if t == destructibleObstacleType {
				delete(game.obstacleMap, p)
			}
		}
		return true
	})

	// update flame map
	newFlameMap := map[Position]*Bomb{}
	for bombPos, bomb := range game.explodingBombs {
		getExplodeFlame(bombPos, func(p Position) bool {
			if t, ok := game.obstacleMap[p]; ok && t == indestructibleObstacleType {
				return false
			}
			newFlameMap[p] = bomb
			return true
		})
	}
	game.flameMap = newFlameMap

	if strings.HasPrefix(bomb.bombName, "random-") ||
		strings.HasPrefix(bomb.bombName, game.localPlayerName+"-") {
		go func() {
			// explosion flame will disappear after 2 seconds
			flameTimer := time.NewTimer(flameTime * time.Second)
			<-flameTimer.C
			game.sendAsync(&UndoExplodeEvent{
				pos: bomb.pos,
			})
		}()
	}
}

type UndoExplodeEvent struct {
	pos Position
}

func (e *UndoExplodeEvent) handle(game *BombGame) {
	delete(game.explodingBombs, e.pos)
	newFlameMap := map[Position]*Bomb{}
	for bombPos, bomb := range game.explodingBombs {
		getExplodeFlame(bombPos, func(p Position) bool {
			if t, ok := game.obstacleMap[p]; ok && t == indestructibleObstacleType {
				return false
			}
			newFlameMap[p] = bomb
			return true
		})
	}
	game.flameMap = newFlameMap
}

type BombMoveEvent struct {
	// bomb playerName, generate by player info
	bombName string
	pos      Position
}

func (e *BombMoveEvent) handle(game *BombGame) {
	log.Info("handle BombMoveEvent")
	bomb, ok := game.nameToBombs[e.bombName]
	if !ok {
		return
	}
	_, ok = game.posToBombs[bomb.pos]
	if !ok {
		return
	}
	// move this bomb
	delete(game.posToBombs, bomb.pos)
	bomb.pos = e.pos
	game.posToBombs[e.pos] = bomb
}

type UpdateMapEvent struct {
	Obstacles []int
}

func (e *UpdateMapEvent) handle(game *BombGame) {
	game.obstacleMap = genObstacleMapFromList(e.Obstacles, nil)
}

func genObstacleMapFromList(list []int, f func(p Position) bool) map[Position]ObstacleType {
	obstacleMap := map[Position]ObstacleType{}
	for _, code := range list {
		destructible := false
		if code < 0 {
			// this is a destructible obstacle
			destructible = true
			code = -code
		}
		x, y := decodeXY(code)
		pos := Position{
			X: x,
			Y: y,
		}
		if f != nil && !f(pos) {
			continue
		}
		if destructible {
			obstacleMap[pos] = destructibleObstacleType
		} else {
			obstacleMap[pos] = indestructibleObstacleType
		}
	}
	return obstacleMap
}

func genListFromObstacleMap(obstacleMap map[Position]ObstacleType) []int {
	var list []int
	for pos, t := range obstacleMap {
		code := encodeXY(pos.X, pos.Y)
		if t == destructibleObstacleCount {
			code = -code
		}
		list = append(list, code)
	}
	return list
}
