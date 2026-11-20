package match

import (
	"log"
	"sort"
	"sync"
	"time"

	"gamestats/ingestion/internal/client"
	"gamestats/ingestion/internal/model"
)

// FinishedGuardTTL es cuánto tiempo se recuerda que una partida ya terminó.
// Durante ese lapso, los eventos que lleguen para ella (por ejemplo, un
// reintento de MATCH_FINISHED que el emisor envió dos veces porque no vio
// la respuesta) se ignoran en lugar de abrir una "partida nueva" con un
// solo evento. Debe ser mayor que la ventana de reintentos de los emisores.
const FinishedGuardTTL = 2 * time.Minute

// Resultado de entregar un evento al Manager.
type DispatchResult int

const (
	// Accepted: el evento se entregó al worker de su partida.
	Accepted DispatchResult = iota
	// Duplicate: ya se había recibido un evento con ese eventId en esta
	// partida (típico de un reintento); se ignora sin error.
	Duplicate
	// Closed: la partida ya terminó; el evento llegó tarde y se ignora.
	Closed
)

// matchWorker mantiene el estado de una partida mientras está activa.
// Cada partida corre en su propia goroutine, consumiendo eventos desde
// un channel dedicado. Esto permite procesar varias partidas (y varios
// juegos) en paralelo sin que una bloquee a otra.
type matchWorker struct {
	matchID string
	inbox   chan model.Event
	events  []model.Event

	// mu protege seen y closing. Con él, "comprobar duplicado, comprobar
	// que la partida sigue abierta y encolar" es una sola operación
	// atómica por partida (ver Dispatch).
	mu      sync.Mutex
	seen    map[string]struct{}
	closing bool
}

// Manager reparte los eventos entrantes hacia el worker de la partida
// correspondiente, creando el worker si es la primera vez que se ve esa
// partida, y limpiándolo cuando la partida termina (MATCH_FINISHED).
type Manager struct {
	mu       sync.RWMutex
	workers  map[string]*matchWorker
	finished map[string]time.Time // matchId -> cuándo terminó (ver FinishedGuardTTL)
	analyzer *client.ScalaClient
	stats    *liveStats
}

func NewManager(analyzer *client.ScalaClient) *Manager {
	return &Manager{
		workers:  make(map[string]*matchWorker),
		finished: make(map[string]time.Time),
		analyzer: analyzer,
		stats:    newLiveStats(),
	}
}

// Dispatch envía un evento ya validado hacia el worker de su partida.
// Es seguro llamarlo concurrentemente desde varios handlers HTTP.
//
// Es idempotente por eventId: reenviar un evento ya recibido devuelve
// Duplicate y no tiene ningún efecto. Esto permite que los emisores
// reintenten sin miedo cuando falla la comunicación (entrega "al menos una
// vez" por parte del emisor, procesamiento "exactamente una vez" aquí).
func (m *Manager) Dispatch(e model.Event) DispatchResult {
	w, closed := m.workerFor(e.MatchID)
	if closed {
		return Closed
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closing {
		return Closed
	}
	if _, dup := w.seen[e.EventID]; dup {
		return Duplicate
	}
	w.seen[e.EventID] = struct{}{}
	if e.Type == model.TypeMatchFinished {
		// Ningún otro evento de esta partida se acepta a partir de aquí.
		// Así el worker puede cerrar su channel sin que nadie escriba en él.
		w.closing = true
	}
	m.stats.record(e)
	w.inbox <- e
	return Accepted
}

// workerFor devuelve el worker de la partida, creándolo si hace falta.
// El segundo valor es true si la partida ya terminó hace poco.
func (m *Manager) workerFor(matchID string) (*matchWorker, bool) {
	m.mu.RLock()
	w, ok := m.workers[matchID]
	_, done := m.finished[matchID]
	m.mu.RUnlock()
	if ok {
		return w, false
	}
	if done {
		return nil, true
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	// Volver a chequear por si otra goroutine ya lo creó (o la partida
	// terminó) mientras esperábamos el lock de escritura.
	if w, ok = m.workers[matchID]; ok {
		return w, false
	}
	if _, done = m.finished[matchID]; done {
		return nil, true
	}

	w = &matchWorker{
		matchID: matchID,
		inbox:   make(chan model.Event, 128),
		seen:    make(map[string]struct{}),
	}
	m.workers[matchID] = w
	go m.run(w)
	return w, false
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

	now := time.Now()
	m.mu.Lock()
	// Se registra como terminada ANTES de quitarla del mapa, dentro del
	// mismo lock: no hay un instante en que un evento tardío pueda crear
	// una partida nueva con el mismo matchId.
	m.finished[w.matchID] = now
	for id, t := range m.finished {
		if now.Sub(t) > FinishedGuardTTL {
			delete(m.finished, id)
		}
	}
	delete(m.workers, w.matchID)
	m.mu.Unlock()
	close(w.inbox)
}

// ActiveMatches expone cuántas partidas están siendo procesadas
// concurrentemente en este momento (útil para health-check / demo).
func (m *Manager) ActiveMatches() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.workers)
}

// Live devuelve los contadores en vivo por juego (GET /stats/live).
func (m *Manager) Live() LiveSnapshot {
	return m.stats.snapshot(time.Now())
}
