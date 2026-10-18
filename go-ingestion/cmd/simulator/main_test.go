package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"gamestats/ingestion/internal/model"
	"gamestats/ingestion/internal/validate"
)

// Comprueba que el simulador produce una partida coherente: eventos que
// pasan la validación de Go, un solo MATCH_FINISHED al final y un jugador
// que remonta (para poder demostrar la regla de remontada en la demo).
func TestRunMatchGeneraUnaPartidaValidaConRemontada(t *testing.T) {
	var mu sync.Mutex
	var eventos []model.Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var e model.Event
		if err := json.NewDecoder(r.Body).Decode(&e); err == nil {
			mu.Lock()
			eventos = append(eventos, e)
			mu.Unlock()
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	runMatch(srv.URL, "match-test", 4, 5)

	mu.Lock()
	defer mu.Unlock()

	if len(eventos) == 0 {
		t.Fatal("el simulador no envió ningún evento")
	}
	porTipo := map[string]int{}
	for _, e := range eventos {
		if err := validate.Validate(e); err != nil {
			t.Errorf("el simulador generó un evento inválido (%s): %v", e.Type, err)
		}
		if e.MatchID != "match-test" {
			t.Errorf("evento de otra partida: %s", e.MatchID)
		}
		porTipo[e.Type]++
	}

	if porTipo[model.TypeMatchStarted] != 1 || porTipo[model.TypeMatchFinished] != 1 {
		t.Errorf("debe haber 1 MATCH_STARTED y 1 MATCH_FINISHED: %v", porTipo)
	}
	if porTipo[model.TypePlayerJoined] != 4 {
		t.Errorf("se esperaban 4 PLAYER_JOINED, hay %d", porTipo[model.TypePlayerJoined])
	}
	if porTipo[model.TypeLapCompleted] != 4*5 {
		t.Errorf("se esperaban 20 LAP_COMPLETED, hay %d", porTipo[model.TypeLapCompleted])
	}
	// El jugador que arranca último gana una posición por vuelta.
	if porTipo[model.TypePositionChanged] < 3 {
		t.Errorf("se esperaban al menos 3 POSITION_CHANGED (remontada), hay %d", porTipo[model.TypePositionChanged])
	}
}
