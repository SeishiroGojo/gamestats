package match

import (
	"sort"
	"sync"
	"time"

	"gamestats/ingestion/internal/model"
)

// liveStats lleva los contadores "en vivo" que GameStats expone en
// GET /stats/live. Se actualizan en el mismo instante en que llega cada
// evento (no esperan a que termine la partida), por eso permiten que un
// panel muestre cuántas instancias de cada juego se han iniciado, cuántas
// siguen en curso y cuántas terminaron mientras las partidas se juegan.
//
// Es un componente pequeño con su propio mutex: así actualizar contadores
// nunca compite con el mapa de partidas del Manager.
type liveStats struct {
	mu    sync.Mutex
	games map[string]*gameCounters
}

type gameCounters struct {
	started      int
	finished     int
	events       int
	eventsByType map[string]int
}

func newLiveStats() *liveStats {
	return &liveStats{games: make(map[string]*gameCounters)}
}

// record anota un evento aceptado (no duplicado, no tardío).
func (s *liveStats) record(e model.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()

	g, ok := s.games[e.GameID]
	if !ok {
		g = &gameCounters{eventsByType: make(map[string]int)}
		s.games[e.GameID] = g
	}
	g.events++
	g.eventsByType[e.Type]++
	switch e.Type {
	case model.TypeMatchStarted:
		g.started++
	case model.TypeMatchFinished:
		g.finished++
	}
}

// GameLive es el resumen en vivo de un juego.
type GameLive struct {
	GameID       string         `json:"gameId"`
	Started      int            `json:"started"`
	Finished     int            `json:"finished"`
	Active       int            `json:"active"`
	Events       int            `json:"events"`
	EventsByType map[string]int `json:"eventsByType"`
}

// LiveTotals suma los contadores de todos los juegos.
type LiveTotals struct {
	Started  int `json:"started"`
	Finished int `json:"finished"`
	Active   int `json:"active"`
	Events   int `json:"events"`
}

// LiveSnapshot es lo que devuelve GET /stats/live.
type LiveSnapshot struct {
	GeneratedAt time.Time  `json:"generatedAt"`
	Totals      LiveTotals `json:"totals"`
	Games       []GameLive `json:"games"`
}

// snapshot toma una copia consistente de los contadores, con los juegos
// ordenados por gameId para que la respuesta sea estable.
func (s *liveStats) snapshot(now time.Time) LiveSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()

	snap := LiveSnapshot{GeneratedAt: now.UTC(), Games: make([]GameLive, 0, len(s.games))}
	for id, g := range s.games {
		active := g.started - g.finished
		if active < 0 {
			active = 0
		}
		byType := make(map[string]int, len(g.eventsByType))
		for k, v := range g.eventsByType {
			byType[k] = v
		}
		snap.Games = append(snap.Games, GameLive{
			GameID: id, Started: g.started, Finished: g.finished,
			Active: active, Events: g.events, EventsByType: byType,
		})
		snap.Totals.Started += g.started
		snap.Totals.Finished += g.finished
		snap.Totals.Active += active
		snap.Totals.Events += g.events
	}
	sort.Slice(snap.Games, func(i, j int) bool { return snap.Games[i].GameID < snap.Games[j].GameID })
	return snap
}
