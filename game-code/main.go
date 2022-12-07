package main

import (
	"bufio"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"log"
	"os"
	"strings"
)

const pulsarUrl = "pulsar://localhost:6650"

func main() {
	fmt.Print("\033[H\033[2J")
	fmt.Println("Welcome to BombMan game!\n")
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Bomb man")
	fmt.Println("Input your room name:")
	reader := bufio.NewReader(os.Stdin)
	roomName, _ := reader.ReadString('\n')
	roomName = strings.Trim(roomName, "\n")

	fmt.Println()
	fmt.Println("Input your player name:")
	playerName, _ := reader.ReadString('\n')
	playerName = strings.Trim(playerName, "\n")
	playerName = strings.ReplaceAll(playerName, "-", "_")

	game := newGame(playerName, roomName)
	defer game.Close()

	//game.randomBombsEnable()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal("[main]", err)
	}
}
