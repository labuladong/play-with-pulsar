package main

import (
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

const (
	UserMoveEventType     = "UserMoveEvent"
	UserJoinEventType     = "UserJoinEvent"
	UserDeadEventType     = "UserDeadEvent"
	UserReviveEventType   = "UserReviveEvent"
	SetBombEventType      = "SetBombEvent"
	MoveBombEventType     = "BombMoveEvent"
	ExplodeEventType      = "ExplodeEvent"
	UndoExplodeEventType  = "UndoExplodeEvent"
	InitObstacleEventType = "UpdateMapEvent"
)

// Event make change on Graph
type Event interface {
	handle(game *Game)
}

// UserMoveEvent makes playerInfo move
type UserMoveEvent struct {
	*playerInfo
}

func (a *UserMoveEvent) handle(g *Game) {
	log.Info("handle UserMoveEvent")
	if !validCoordinate(a.pos) {
		// move out of boarder
		return
	}
	if _, ok := g.obstacleMap[a.pos]; ok {
		// move to obstacle
		return
	}
	if player, ok := g.nameToPlayers[a.name]; ok && !player.alive {
		// already dead
		return
	}
	g.nameToPlayers[a.name] = a.playerInfo
	g.posToPlayers[a.pos] = a.playerInfo
}

type UserDeadEvent struct {
	*playerInfo
	killer string
}

func (e *UserDeadEvent) handle(game *Game) {
	if _, ok := game.nameToPlayers[e.name]; ok {
		game.nameToPlayers[e.name].alive = false
	}
}

type UserReviveEvent struct {
	*playerInfo
}

func (e *UserReviveEvent) handle(game *Game) {
	game.nameToPlayers[e.name] = e.playerInfo
	game.nameToPlayers[e.name].alive = true
}

type UserJoinEvent struct {
	*playerInfo
}

func (e *UserJoinEvent) handle(game *Game) {
	//TODO implement me
	panic("implement me")
}

type SetBombEvent struct {
	bombName string
	pos      Position
}

func (e *SetBombEvent) handle(game *Game) {
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

func (e *ExplodeEvent) handle(game *Game) {
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
	game.explode(bomb)

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

func (e *UndoExplodeEvent) handle(game *Game) {
	game.unExplode(e.pos)
}

type BombMoveEvent struct {
	// bomb playerName, generate by player info
	bombName string
	pos      Position
}

func (e *BombMoveEvent) handle(game *Game) {
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

func (e *UpdateMapEvent) handle(game *Game) {
	obstacleMap := map[Position]ObstacleType{}
	for _, code := range e.Obstacles {
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
		if game.posToBombs[pos] != nil || game.posToPlayers[pos] != nil {
			continue
		}
		if destructible {
			obstacleMap[pos] = destructibleObstacleType
		} else {
			obstacleMap[pos] = indestructibleObstacleType
		}
	}
	game.obstacleMap = obstacleMap
}
