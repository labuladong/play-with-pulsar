# pulsar-bomb-game

A tiny game using Apache Pulsar.

## How to install

1️⃣ Clone this repo, enter code folder.

```bash
git clone https://github.com/labuladong/play-with-pulsar.git
cd play-with-pulsar/game-code
```

2️⃣ Install the dependency:

```bash
go mod download
```

3️⃣ Change your private key path in `main.go`.

4️⃣ Compile to generate executable file `game`:

```bash
go build -o game *.go
```

## How to play

Run the executable file `game` with room name, player name and play mode:

```bash
./game -player jack -room roomname -mode play
```