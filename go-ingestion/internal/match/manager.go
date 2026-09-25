package match

import (
	"log"
	"sort"
	"sync"

	"gamestats/ingestion/internal/client"
	"gamestats/ingestion/internal/model"
)

// matchWorker mantiene el estado de una partida mientras está activa.
// Cada partida corre en su propia goroutine, consumiendo eventos desde
// un channel dedicado. Esto permite procesar varias partidas (y varios
// juegos) en paralelo sin que una bloquee a otra.
type matchWorker struct {
	matchID string
	inbox   chan model.Event
	events  []model.Event
}

// Manager reparte los eventos entrantes hacia el worker de la partida
// correspondiente, creando el worker si es la primera vez que se ve esa
// partida, y limpiándolo cuando la partida termina (MATCH_FINISHED).
type Manager struct {
	mu       sync.RWMutex
	workers  map[string]*matchWorker
	analyzer *client.ScalaClient
}

func NewManager(analyzer *client.ScalaClient) *Manager {
	return &Manager{
		workers:  make(map[string]*matchWorker),
		analyzer: analyzer,
	}
}

// Dispatch envía un evento ya validado hacia el worker de su partida.
// Es seguro llamarlo concurrentemente desde varios handlers HTTP.
func (m *Manager) Dispatch(e model.Event) {
	w := m.workerFor(e.MatchID)
	w.inbox <- e
}

func (m *Manager) workerFor(matchID string) *matchWorker {
	m.mu.RLock()
	w, ok := m.workers[matchID]
	m.mu.RUnlock()
	if ok {
		return w
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Volver a chequear por si otra goroutine ya lo creó mientras
	// esperábamos el lock de escritura.
	if w, ok = m.workers[matchID]; ok {
		return w
	}

	w = &matchWorker{
		matchID: matchID,
		inbox:   make(chan model.Event, 128),
	}
	m.workers[matchID] = w
	go m.run(w)
	return w
}

// run es el ciclo de vida de una partida: acumula eventos en orden de
// llegada y, al recibir MATCH_FINISHED, entrega el lote completo a Scala
// y libera los recursos de la partida.
func (m *Manager) run(w *matchWorker) {
	for e := range w.inbox {
		w.events = append(w.events, e)

		if e.Type == model.TypeMatchFinished {
			m.finalize(w)
			return
		}
	}
}

func (m *Manager) finalize(w *matchWorker) {
	sort.SliceStable(w.events, func(i, j int) bool {
		return w.events[i].Timestamp.Before(w.events[j].Timestamp)
	})

	batch := model.MatchBatch{MatchID: w.matchID, Events: w.events}
	result, err := m.analyzer.SendMatch(batch)
	if err != nil {
		log.Printf("error enviando partida %s a Scala: %v", w.matchID, err)
	} else {
		log.Printf("resultado del análisis para %s:\n%s", w.matchID, result)
	}

	m.mu.Lock()
	delete(m.workers, w.matchID)
	m.mu.Unlock()
	close(w.inbox)
	// Nota de diseño: si un evento tardío llega para este matchID justo
	// después de MATCH_FINISHED, workerFor() ya no lo encontrará en el mapa
	// y creará una partida nueva. En un sistema real esto se trataría como
	// "evento fuera de orden" y se registraría aparte; se deja como
	// extensión posible.
}

// ActiveMatches expone cuántas partidas están siendo procesadas
// concurrentemente en este momento (útil para health-check / demo).
func (m *Manager) ActiveMatches() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}
