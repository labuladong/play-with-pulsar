package main

import (
	"flag"
	"fmt"
	"github.com/hajimehoshi/ebiten/v2"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"os"
)

var pulsarConfig *PulsarConfig

func parseConfigFile(path string) *PulsarConfig {
	// Read YAML file
	yamlFile, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}

	// Parse YAML file
	var config PulsarConfig
	err = yaml.Unmarshal(yamlFile, &config)
	if err != nil {
		panic(err)
	}
	if config.BrokerUrl == "" {
		config.BrokerUrl = "pulsar://localhost:6650"
	}

	fmt.Println("Broker url:", config.BrokerUrl)
	fmt.Println("OAuth.Enabled:", config.OAuth.Enabled)
	fmt.Println("OAuth.IssuerURL:", config.OAuth.IssuerURL)
	fmt.Println("OAuth.Audience:", config.OAuth.Audience)
	fmt.Println("OAuth.PrivateKey:", config.OAuth.PrivateKey)
	return &config
}

type OAuthConfig struct {
	Enabled    bool   `yaml:"enabled"`
	IssuerURL  string `yaml:"issuerUrl"`
	Audience   string `yaml:"audience"`
	PrivateKey string `yaml:"privateKey"`
}

type PulsarConfig struct {
	BrokerUrl string      `yaml:"brokerUrl"`
	OAuth     OAuthConfig `yaml:"OAuth"`
}

func main() {
	var help = flag.Bool("help", false, "Show help")
	var roomName string
	var playerName string
	var mode string
	var at string

	pulsarConfig = parseConfigFile("config.yml")

	// Bind the flag
	flag.StringVar(&roomName, "room", "", "the room name")
	flag.StringVar(&playerName, "player", "", "the player name")
	flag.StringVar(&mode, "mode", "play", "play/watch")
	flag.StringVar(&at, "at", "earliest", "specify the point you'd like to watch")
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
		replay := NewGameReplay(roomName, at)
		defer replay.Close()
		if err := ebiten.RunGame(replay); err != nil {
			log.Fatal("[main]", err)
		}
	} else {
		log.Fatal("mode must be play or watch")
		os.Exit(1)
	}
}
