# Developing Multiplayer Games with Pulsar (Part 3): Building a 2D Game with Ebiten in Golang

> Note: This article is the third one in the "Developing Multiplayer Online Games with Pulsar" series. The source code and all the documents can be found at my GitHub repository [play-with-pulsar](https://github.com/labuladong/play-with-pulsar), as well as my article list.

For building the Bomberman game, I have chosen Ebitengine, a 2D game framework in Golang, which you can find at the following website:

https://ebitengine.org/

I picked this framework for two main reasons:

1. It's straightforward and easy to learn, making it perfect for rapid development of 2D games.

2. It supports compilation to WebAssembly, so if needed, it can run directly on a web page.

The usage of this library is very simple. You just need to implement a few core methods of the `Game` interface:

```golang
type Game interface {
	// Fill in the logic of data updates in the Update function
	Update() error

	// Fill in the logic of image rendering in the Draw function
	Draw(screen *Image)

	// Return the size of the game interface
	Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int)
}
```

We know that the principle of dynamic image display on a monitor is actually to quickly refresh frames of images, which appears as dynamic images to the naked eye.

In essence, this game framework does the following:

Before every frame of image refresh, this game framework first calls the `Update` method to update the game data, then calls the `Draw` method to render each frame of image according to the game data. This enables the creation of simple 2D games.

Next, let's implement a Snake game to get a better understanding of how this framework functions.

### Creating a Snake game

The Snake game is actually an example provided by the framework's official documentation. There is only one `main.go` file, which can be found at the following link:

https://github.com/hajimehoshi/ebiten/blob/main/examples/snake/main.go

This game is actually very simple, consisting of only 200 lines of code. Here's a brief overview of the key logic in the code, as the layout of our Bomberman game is based on the layout of this Snake game.

The data of the Snake game is stored in `Game`:

```go
// Store game data
type Game struct {
	// The direction of the movement of the snake
	moveDirection int
	// Snake body
	snakeBody     []Position
	// Position of the food
	apple         Position

	// Control the speed of the snake based on the level of difficulty
	timer         int
	moveTime      int
	level         int

	// Score count
	score         int
	bestScore     int
}
```

Now let's take a look at the `Update` method, which is responsible for monitoring the player's moves and updating the game data in the `Game` structure:

```go
func (g *Game) Update() error {
	// Monitor the arrow keys and WASD to update the direction of the snake's movement
	if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) || inpututil.IsKeyJustPressed(ebiten.KeyA) {
		if g.moveDirection != dirRight {
			g.moveDirection = dirLeft
		}
	} else if (...)

	// Move the snake one step forward
	if g.needsToMoveSnake() {
		if g.collidesWithWall() || g.collidesWithSelf() {
			// Snake hits the wall or bites itself, game over, reset relevant game data
			g.reset()
		}

		if g.collidesWithApple() {
			// Snake eats the food
			// 1. Generate a new food at a random position
			g.apple.X = rand.Intn(xGridCountInScreen - 1)
			g.apple.Y = rand.Intn(yGridCountInScreen - 1)
			// 2. Extend the snake's body
			g.snakeBody = append(g.snakeBody, Position{X, Y})
			// 3. Update the score
			g.score++
			// 4. Increase the speed of the snake's movement to increase the difficulty level of the game
			g.level++
		}

		// Move the snake body one grid forward
		switch g.moveDirection {
		case dirLeft:
			g.snakeBody[0].X--
		case dirRight:
			g.snakeBody[0].X++
		case dirDown:
			g.snakeBody[0].Y++
		case dirUp:
			g.snakeBody[0].Y--
		}
	}
}
```

After the `Update` method has updated the data, the `Draw` method will render the game interface based on the updated data:

```go
func (g *Game) Draw(screen *ebiten.Image) {
	// Draw the snake body
	for _, v := range g.snakeBody {
		ebitenutil.DrawRect(v.X, v.Y, color.RGBA{0x80, 0xa0, 0xc0, 0xff})
	}
	// Draw the food
	ebitenutil.DrawRect(g.apple.X, g.apple.Y, color.RGBA{0xFF, 0x00, 0x00, 0xff})
}
```

Simple, isn't it? After completing this code, we would have a classic Snake game:

![]()

In comparison to our Bomberman game:

![](https://labuladong.github.io/pictures/pulsar-game/pushbomb.gif)

You can see that the basic game layout is actually similar to that of the Snake game, using differently colored blocks to represent obstacles, players, and bombs. This is mainly because it's easy to implement and doesn't require non-programming work such as artwork or textures. Therefore, the `Draw` method of the Bomberman game should be similar to that of the Snake game, rendering different colored blocks.

The most crucial feature of our Bomberman game is the inclusion of multiplayer elements, so the key updates happen in the `Update` method. Let's explore this further.

### Implementation of the Bomberman Game

First, we create a `Game` struct to store the data of the Bomberman game:

```go
type Game struct {
	// The name of the current player
	localPlayerName string
	// Record the names and positions of all players in the game
	posToPlayers    map[Position]*playerInfo
	// Record the positions of all bombs
	posToBombs  map[Position]*Bomb
	// Record the positions of the bomb explosion flames
	flameMap  map[Position]*Bomb
	// Record the positions of the obstacles
	obstacleMap map[Position]ObstacleType

	// All events sent from Pulsar are passed to this channel
	receiveCh chan Event
	// All events put in this channel will be sent to Pulsar
	sendCh chan Event

	// Manage the connection with Pulsar
	client *pulsarClient
	// Store the player score information of the game room
	scores *lru.Cache
}
```

The `Draw` method is straightforward. It simply renders all game objects:

```go
func (g *Game) Draw(screen *ebiten.Image) {
	// Draw the bombs
	for pos, _ := range g.posToBombs {
		ebitenutil.DrawRect(pos.X, pos.Y, bombColor)
	}

	// Draw the obstacles
	for pos, t := range g.obstacleMap {
		ebitenutil.DrawRect(pos.X, pos.Y, obstacleColor)
	}

	// Draw the players
	for _, player := range g.nameToPlayers {
		ebitenutil.DrawRect(player.X, player.Y, userColor)
	}

	// Draw the flames
	for pos, val := range g.flameMap {
		ebitenutil.DrawRect(pos.X, pos.Y, flameColor)
	}
}
```

Since the snake game is only a standalone game, there may not be many events that update the game data, such as local players pressing direction keys, or the snake hitting a wall or biting itself.

On the other hand, the bomber game may have a lot of game data update events. In addition to keyboard events generated by local players, events generated by online players, such as new players joining the room, players dying, players resurrecting, and players moving, must also be considered.

To simplify the handling of various complex situations, we can create an `Event` interface, as described in the previous article "How to Implement Game Requirements with Pulsar". All player actions are abstracted as an `Event`:

```
type Event interface {
	handle(game *Game)
}
```

The `handle` method of the `Event` interface is passed the `Game` structure, and the concrete class that implements the `Event` interface determines how to update the game data.

For example, player movement is abstracted as a `UserMoveEvent` class, which implements the `Event` interface:

```
// UserMoveEvent makes playerInfo move
type UserMoveEvent struct {
    playerName string
    pos        Position
}

// Handle the event of player movement and update the relevant data
func (e *UserMoveEvent) handle(g *Game) {
    // Prevent movement beyond the border
    if !validCoordinate(e.pos) {
        return
    }
    // Prevent moving onto obstacles
    if _, ok := g.obstacleMap[e.pos]; ok {
        return
    }
    // Dead players are not allowed to move
    if player, ok := g.nameToPlayers[e.name]; ok && !player.alive {
        return
    }
    // Update the player's position information
    g.posToPlayers[e.pos] = &playerInfo{
        name: e.playerName
        pos:  e.pos
    }
}
```

Similarly, other events will update the game data in their `handle` method. The implementation code for all event classes is in `event.go`.

With the `Event` interface, the code in `Update` can be greatly simplified:

```
func (g *Game) Update() error {
    // Events from Pulsar are sent to the receiveCh channel, 
    // and all events are processed non-blocking, 
    // updating local game data
    select {
    case event := <-g.receiveCh:
        event.handle(g)
    default:
    }
    
    // Listen for events generated by local players, 
    // send all events to Pulsar via the sendCh channel
    dir, setBomb := listenLocalKeyboard()

    if dir != dirNone {
        // Generate an event for player movement
        nextPlayerPos := getNextPosition(localPlayer.pos, dir)
        localEvent := &UserMoveEvent{
            name: localPlayer.playerName
            pos:  nextPlayerPos,
        }
        g.sendCh <- localEvent
    }
    if setBomb {
        // Generate an event for setting bombs
        localEvent := &SetBombEvent{
            pos: localPlayer.pos,
        }
        g.sendCh <- localEvent
    }
    // ...
}
```

We put the locally generated events into `sendCh` and read Pulsar events from `receiveCh` and render them. Since `Update` is called every frame refresh, just like a dead loop, the logic above implements the pseudo-code logic of synchronizing events generated by multiple players as discussed in the article "Multiplayer game challenge analysis".

Pulsar clients will handle the sending and receiving of events on the other side of `sendCh` and `receiveCh`. How they actually work will be discussed in subsequent chapters.