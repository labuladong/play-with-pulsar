package main

import (
	"image/color"
	"math/rand"
)

var (
	playerColor                 = color.RGBA{R: 0xFF, A: 0xff, G: 0x34}
	deadPlayerColor             = color.RGBA{R: 0xeb, A: 0xc4, G: 0x40}
	bombColor                   = color.RGBA{R: 218, G: 165, B: 32, A: 0xff}
	flameColor                  = color.RGBA{R: 255, G: 215, B: 0, A: 0xaf}
	destructibleObstacleColor   = color.Gray{Y: 90}
	indestructibleObstacleColor = color.White
)

type playerInfo struct {
	// localPlayer name
	name   string
	avatar string
	pos    Position
	alive  bool
}

type Direction int

const (
	dirNone Direction = iota
	dirLeft
	dirRight
	dirDown
	dirUp
)

func getNextPosition(position Position, direction Direction) Position {
	f := map[Direction]func(int, int) (int, int){
		dirLeft: func(x int, y int) (int, int) {
			return x - 1, y
		},
		dirRight: func(x int, y int) (int, int) {
			return x + 1, y
		},
		dirUp: func(x int, y int) (int, int) {
			return x, y - 1
		},
		dirDown: func(x int, y int) (int, int) {
			return x, y + 1
		},
		dirNone: func(x int, y int) (int, int) {
			return x, y
		},
	}
	x, y := f[direction](position.X, position.Y)
	res := Position{X: x, Y: y}
	if validCoordinate(res) {
		return res
	}
	return position
}

func validCoordinate(pos Position) bool {
	return pos.X >= 0 && pos.Y >= 0 && pos.X < xGridCountInScreen && pos.Y < yGridCountInScreen
}

type Position struct {
	X int
	Y int
}

type Bomb struct {
	// the player name
	playerName, bombName string
	pos                  Position
	// when exploded, this chanel will receive a message, control bomb moving
	explodeCh chan struct{}
}

func randStringRunes(n int) string {
	var letterRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ")
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func encodeXY(x, y int) int {
	return y*xGridCountInScreen + x
}

func decodeXY(code int) (int, int) {
	return code % xGridCountInScreen, code / xGridCountInScreen
}

// sample k number in [0, n)
func sample(n, k int) []int {
	pickedNums := make([]int, k)
	for i := 0; i < k; i++ {
		pickedNums[i] = i
	}
	for i := k; i < n; i++ {
		r := rand.Intn(i + 1)
		if r < k {
			pickedNums[r] = i
		}
	}
	return pickedNums
}

func sliceContains(slice []int, p int) bool {
	for _, e := range slice {
		if e == p {
			return true
		}
	}
	return false
}
