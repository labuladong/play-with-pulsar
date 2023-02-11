package main

import (
	"flag"
	"github.com/hajimehoshi/ebiten/v2"
	log "github.com/sirupsen/logrus"
	"os"
)

const pulsarUrl = "pulsar://localhost:6650"

func main() {
	var help = flag.Bool("help", false, "Show help")
	var roomName string
	var playerName string
	var mode string
	// Bind the flag
	flag.StringVar(&roomName, "room", "", "the room name")
	flag.StringVar(&playerName, "player", "", "the player name")
	flag.StringVar(&mode, "mode", "play", "play/watch")
	// Parse the flag
	flag.Parse()

	// Usage Demo
	if *help {
		flag.Usage()
		os.Exit(0)
	}
	if mode == "" {
		log.Fatal("must specify the -mode")
		os.Exit(1)
	}
	if playerName == "" && mode == "play" {
		log.Fatal("playerName must not be empty")
		os.Exit(1)
	}
	if roomName == "" {
		log.Fatal("roomName must not be empty")
		os.Exit(1)
	}

	ebiten.SetWindowSize(screenWidth, screenHeight)
	if mode == "play" {
		game := newGame(playerName, roomName)
		defer game.Close()
		if err := ebiten.RunGame(game); err != nil {
			log.Fatal("[main]", err)
		}
	} else if mode == "watch" {
		replay := NewGameReplay(roomName)
		defer replay.Close()
		if err := ebiten.RunGame(replay); err != nil {
			log.Fatal("[main]", err)
		}
	} else {
		log.Fatal("mode must be play or watch")
		os.Exit(1)
	}
}
