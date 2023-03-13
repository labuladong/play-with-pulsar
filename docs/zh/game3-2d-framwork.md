---
title: '用 Pulsar 开发多人小游戏（三）：Golang 2D 游戏框架 Ebiten 实战'
---

> note：本文是《用 Pulsar 开发多人在线小游戏》的第三篇，配套源码和全部文档参见我的 GitHub 仓库 [play-with-pulsar](https://github.com/labuladong/play-with-pulsar) 以及我的文章列表。

我选择了 Go 语言的一款 2D 游戏框架来制作这个炸弹人游戏，叫做 Ebitengine，官网如下：

https://ebitengine.org/

之所以选择这款 Go 语言的框架，主要是两个原因：

1、非常简单易学，适合快速上手写 2D 小游戏。

2、支持编译成 WebAssembly，如果需要的话可以直接编译到网页上运行。

这个库的使用原理特别简单，只要你实现这个 `Game` 接口的几个核心方法就可以：

```golang
type Game interface {
    // 在 Update 函数里填写数据更新的逻辑
	Update() error

    // 在 Draw 函数里填写图像渲染的逻辑
	Draw(screen *Image)

    // 返回游戏界面的大小
	Layout(outsideWidth, outsideHeight int) (screenWidth, screenHeight int)
}
```

我们知道显示器能够显示动态影像的原理其实就是快速的刷新一帧一帧的图像，肉眼看起来就好像是动态影像了。

这个游戏框架做的事情其实很简单：

在每一帧图像刷新之前，这个游戏框架会先调用 `Update` 方法更新游戏数据，再调用 `Draw` 方法根据游戏数据渲染出每一帧图像，这样就能够制作出简单的 2D 小游戏了。

下面我们实现一个贪吃蛇游戏来具体看看这个框架的用法。

### 制作贪吃蛇游戏

贪吃蛇游戏是框架官网给出的一个例子，只有一个 `main.go` 文件，链接如下：

https://github.com/hajimehoshi/ebiten/blob/main/examples/snake/main.go

这个游戏其实很简单，总共也就 200 多行代码，我这里简单过一下代码中的核心逻辑，因为我们的炸弹人游戏是基于贪吃蛇游戏的布局之上开发的。

贪吃蛇游戏的数据都存在 `Game` 中：

```go
// 存储游戏数据
type Game struct {
    // 贪吃蛇移动的方向
	moveDirection int
    // 蛇身
	snakeBody     []Position
    // 食物的位置
	apple         Position

    // 控制蛇的移动速度随着难度增加而增加
	timer         int
	moveTime      int
	level         int

    // 分数统计
    score         int
	bestScore     int
}
```

接下来看 `Update` 方法，这个方法主要的任务是监听玩家的动作并更新 `Game` 结构体中的游戏数据：

```go
func (g *Game) Update() error {
    // 监听 WASD 和方向键，更新蛇的行进方向
    if inpututil.IsKeyJustPressed(ebiten.KeyArrowLeft) 
    || inpututil.IsKeyJustPressed(ebiten.KeyA) {
        if g.moveDirection != dirRight {
            g.moveDirection = dirLeft
        }
    } else if (...)

    if g.needsToMoveSnake() {
        if g.collidesWithWall() || g.collidesWithSelf() {
            // 蛇撞墙或者咬到自己，游戏结束，重置相关游戏数据
			g.reset()
		}

        if g.collidesWithApple() {
            // 蛇吃到食物
            // 1. 在随机位置生成新的食物
            g.apple.X = rand.Intn(xGridCountInScreen - 1)
            g.apple.Y = rand.Intn(yGridCountInScreen - 1)
            // 2. 蛇身变长
            g.snakeBody = append(g.snakeBody, Position{X, Y})
            // 3. 更新分数
            g.score++
            // 4. 加快贪吃蛇移动速度从而增加游戏难度
            g.level++
        }

        // 蛇身前进一格
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

`Update` 方法更新数据之后，`Draw` 方法会根据更新后的数据渲染游戏界面：

```go
func (g *Game) Draw(screen *ebiten.Image) {
    // 画出蛇身
	for _, v := range g.snakeBody {
		ebitenutil.DrawRect(v.X, v.Y, color.RGBA{0x80, 0xa0, 0xc0, 0xff})
	}
    // 画出食物
	ebitenutil.DrawRect(g.apple.X, g.apple.Y, color.RGBA{0xFF, 0x00, 0x00, 0xff})
}
```

是不是非常简单？完成这些代码之后，就实现了一个经典的贪吃蛇游戏：

![]()

类比一下我们的炸弹人游戏：

![](https://labuladong.github.io/pictures/pulsar-game/pushbomb.gif)

可以发现，基本的游戏布局其实和贪吃蛇游戏差不多，用不同颜色的方块代表障碍物、玩家、炸弹，这主要也是因为实现起来简单，不需要美术贴图之类的非编程工作。所以炸弹人游戏的 `Draw` 方法和贪吃蛇游戏应该差不多，就是渲染一些不同颜色方块。

我们这个炸弹人游戏最关键的是加入了联机的要素，所以最核心的改动是 `Update` 方法，下面介绍一下实现思路。

### 炸弹人游戏的实现思路

首先，我们也创建一个 `Game` 结构体存储炸弹人游戏的数据：

```go
type Game struct {
    // 当前玩家的名字
	localPlayerName string
    // 记录所有联机玩家的名字及位置
	posToPlayers    map[Position]*playerInfo
    // 记录所有炸弹的位置
	posToBombs  map[Position]*Bomb
    // 记录炸弹爆炸火焰的位置
	flameMap  map[Position]*Bomb
    // 记录障碍物的位置
	obstacleMap map[Position]ObstacleType

	// 从 Pulsar 发来的事件都会传递到这个 channel
	receiveCh chan Event
    // 塞进这个 channel 的事件都会发给 Pulsar
	sendCh chan Event

    // 管理和 Pulsar 的连接
	client *pulsarClient
    // 存储房间内玩家的分数信息
	scores *lru.Cache
}
```

`Draw` 方法很简单，去渲染所有游戏对象就行了：

```go
func (g *Game) Draw(screen *ebiten.Image) {
    // 画出炸弹
	for pos, _ := range g.posToBombs {
        ebitenutil.DrawRect(pos.X, pos.Y, bombColor)
	}

    // 画出障碍物
	for pos, t := range g.obstacleMap {
        ebitenutil.DrawRect(pos.X, pos.Y, obstacleColor)
	}

    // 画出玩家
	for _, player := range g.nameToPlayers {
        ebitenutil.DrawRect(player.X, player.Y, userColor)
	}
    
    // 画出火焰
	for pos, val := range g.flameMap {
        ebitenutil.DrawRect(pos.X, pos.Y, flameColor)
	}
}
```

因为贪吃蛇游戏只是单机游戏，所以可能更新游戏数据的事件不多，无非就是本地玩家按动方向键、贪吃蛇撞到墙或者咬到自己这几个事件。

而炸弹人游戏可能更新游戏数据的事件非常多，除了本地玩家的键盘事件之外，还要考虑到联机玩家产生的事件，比如新玩家加入房间、某个玩家死亡、某个玩家复活，某个玩家移动等等。

为了简化各种复杂情况的处理，我们可以按照前文 [如何用 Pulsar 实现游戏需求](https://labuladong.github.io/article/fname.html?fname=game2-use-mq) 所描述的那样，我们创建了一个 `Event` 接口，玩家的所有动作都被抽象成一个 `Event`：

```go
type Event interface {
	handle(game *Game)
}
```

`Game` 结构会传入 `handle` 方法，由实现 `Event` 接口的具体类去决定如何更新游戏数据。

比如玩家移动被抽象成了 `UserMoveEvent` 类，它实现了 `Event` 接口：

```go
// UserMoveEvent makes playerInfo move
type UserMoveEvent struct {
	playerName string
    pos        Position
}

// 处理玩家移动的事件，更新相应的数据
func (e *UserMoveEvent) handle(g *Game) {
    // 防止移动出界
	if !validCoordinate(e.pos) {
		return
	}
    // 防止移动到障碍物上
	if _, ok := g.obstacleMap[e.pos]; ok {
		return
	}
    // 已经死亡的玩家不允许再移动
	if player, ok := g.nameToPlayers[e.name]; ok && !player.alive {
		return
	}
    // 更新玩家的位置信息
	g.posToPlayers[e.pos] = &playerInfo {
        name   : e.playerName
        pos    : e.pos
    }
}
```

类似的，其他的事件也会在 `handle` 方法中处理游戏数据的更新。所有事件类的实现代码都放在 [`event.go`](https://github.com/labuladong/play-with-pulsar/blob/master/game-code/event.go) 中。

有了 `Event` 接口的抽象，就可以大幅简化 `Update` 中的代码：

```go
func (g *Game) Update() error {
	// Pulsar 那边的事件都会发到 receiveCh 中，
    // 非阻塞地处理这些事件，更新本地游戏数据
	select {
	case event := <-g.receiveCh:
		event.handle(g)
	default:
	}

    // 监听本地玩家产生的事件，
    // 全部通过 sendCh 发送给 Pulsar
    dir, setBomb := listenLocalKeyboard()

    if dir != dirNone {
        // 产生玩家移动的事件
		nextPlayerPos := getNextPosition(localPlayer.pos, dir)
		localEvent := &UserMoveEvent{
            name: localPlayer.playerName
			pos:  nextPlayerPos,
		}
		g.sendCh <- localEvent
	}
    if setBomb {
        // 产生放炸弹的事件
        localEvent := &SetBombEvent{
            pos: localPlayer.pos,
        }
        g.sendCh <- localEvent
    }
    // ...
}
```

我们把本地产生的事件塞进 `sendCh`，并从 `receiveCh` 读取并渲染 Pulsar 中的事件；而且 `Update` 在每一帧刷新时都会被调用，就好像一个死循环，所以上面这段逻辑就实现了 [多人游戏难点分析](https://labuladong.github.io/article/fname.html?fname=game1-introduce) 中提到的同步多个玩家事件的伪码逻辑：

```java
// 一个线程负责拉取并显示事件
new Thread(() -> {
    while (true) {
        // 不断从消息队列拉取事件
        Event event = consumer.receive();
        // 然后更新本地状态，显示给玩家
        updateLocalScreen(event);
    }
});

// 一个线程负责生成并发送本地事件
new Thread(() -> {
    while (true) {
        // 本地玩家产生的事件，要发送到消息队列
        Event localEvent = listenLocalKeyboard();
        producer.send(event);
    }
});
```

`sendCh` 和 `receiveCh` 另一端有 Pulsar 的 client 去处理事件的收发，它们具体是如何做的呢？我会在后面的章节介绍。