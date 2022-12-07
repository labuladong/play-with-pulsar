package main

import (
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	"log"
)

const pulsarUrl = "pulsar://localhost:6650"

func main() {
	ebiten.SetWindowSize(screenWidth, screenHeight)
	ebiten.SetWindowTitle("Bomb man")
	fmt.Println("input room name:")
	//reader := bufio.NewReader(os.Stdin)
	//roomName, _ := reader.ReadString('\n')
	//roomName = strings.Trim(roomName, "\n")

	//fmt.Println("input player name:")
	//playerName, _ := reader.ReadString('\n')
	//playerName = strings.Trim(playerName, "\n")
	//playerName = strings.ReplaceAll(playerName, "-", "_")

	game := newGame("testName2", "roomName")
	defer game.Close()

	//game.randomBombsEnable()

	if err := ebiten.RunGame(game); err != nil {
		log.Fatal("[main]", err)
	}
}
