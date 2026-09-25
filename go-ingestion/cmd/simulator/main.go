// Simulador de la fuente externa de eventos (el "videojuego" real no se
// implementa). Genera varias partidas de carreras en paralelo, con varios
// jugadores, vueltas, penalizaciones, pit stops y una remontada garantizada
// en al menos una partida, y las envía por HTTP al servidor de ingestión.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"gamestats/ingestion/internal/model"
)

func main() {
	target := getEnv("INGESTION_URL", "http://localhost:8080")
	numMatches := 3
	playersPerMatch := 4
	laps := 5

	var wg sync.WaitGroup
	for m := 0; m < numMatches; m++ {
		wg.Add(1)
		go func(matchNum int) {
			defer wg.Done()
			matchID := fmt.Sprintf("match-%d", matchNum+1)
			runMatch(target, matchID, playersPerMatch, laps)
		}(m)
	}
	wg.Wait()
	log.Println("simulación terminada")
}

func runMatch(target, matchID string, numPlayers, laps int) {
	players := make([]string, numPlayers)
	for i := range players {
		players[i] = fmt.Sprintf("player-%s-%d", matchID, i+1)
	}

	send(target, model.Event{
		EventID: id(), Timestamp: now(), GameID: "racing-circuit", GameVersion: "1.0",
		MatchID: matchID, Type: model.TypeMatchStarted,
		Data: map[string]interface{}{"track": "Monza", "totalLaps": laps, "players": players},
	})

	for _, p := range players {
		send(target, model.Event{
			EventID: id(), Timestamp: now(), GameID: "racing-circuit", GameVersion: "1.0",
			MatchID: matchID, PlayerID: p, Type: model.TypePlayerJoined,
			Data: map[string]interface{}{"playerName": p, "carId": "car-" + p},
		})
	}

	// Posiciones iniciales aleatorias; el primer jugador arranca último
	// para forzar una remontada demostrable.
	positions := map[string]int{}
	for i, p := range players {
		positions[p] = i + 1
	}
	positions[players[0]] = numPlayers

	for lap := 1; lap <= laps; lap++ {
		for _, p := range players {
			lapTime := 90000 + rand.Intn(5000)
			send(target, model.Event{
				EventID: id(), Timestamp: now(), GameID: "racing-circuit", GameVersion: "1.0",
				MatchID: matchID, PlayerID: p, Type: model.TypeLapCompleted,
				Data: map[string]interface{}{"lapNumber": lap, "lapTimeMs": lapTime, "position": positions[p], "avgSpeedKmh": 180 + rand.Intn(40)},
			})
		}

		// El jugador que arrancó último va ganando posiciones cada vuelta.
		if from, to := positions[players[0]], positions[players[0]]-1; to >= 1 {
			positions[players[0]] = to
			send(target, model.Event{
				EventID: id(), Timestamp: now(), GameID: "racing-circuit", GameVersion: "1.0",
				MatchID: matchID, PlayerID: players[0], Type: model.TypePositionChanged,
				Data: map[string]interface{}{"fromPosition": from, "toPosition": to, "lapNumber": lap},
			})
		}

		if rand.Intn(3) == 0 {
			p := players[rand.Intn(len(players))]
			send(target, model.Event{
				EventID: id(), Timestamp: now(), GameID: "racing-circuit", GameVersion: "1.0",
				MatchID: matchID, PlayerID: p, Type: model.TypePitStop,
				Data: map[string]interface{}{"durationMs": 2500 + rand.Intn(1500), "lapNumber": lap},
			})
		}
	}

	var standings []map[string]interface{}
	for _, p := range players {
		standings = append(standings, map[string]interface{}{
			"playerId": p, "finalPosition": positions[p], "totalTimeMs": laps * 92000,
		})
	}
	send(target, model.Event{
		EventID: id(), Timestamp: now(), GameID: "racing-circuit", GameVersion: "1.0",
		MatchID: matchID, Type: model.TypeMatchFinished,
		Data: map[string]interface{}{"finalStandings": standings},
	})
}

func send(target string, e model.Event) {
	body, _ := json.Marshal(e)
	resp, err := http.Post(target+"/events", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("error enviando evento %s: %v", e.EventID, err)
		return
	}
	resp.Body.Close()
	time.Sleep(20 * time.Millisecond) // simula el paso del tiempo real
}

func id() string {
	return fmt.Sprintf("evt-%d-%d", time.Now().UnixNano(), rand.Intn(10000))
}

func now() time.Time { return time.Now() }

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
