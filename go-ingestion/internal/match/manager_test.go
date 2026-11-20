package match

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"gamestats/ingestion/internal/client"
	"gamestats/ingestion/internal/model"
)

// Estos tests usan un servidor HTTP falso (httptest) en lugar de Scala,
// para comprobar el comportamiento del Manager sin levantar nada más:
// que entregue el lote completo al terminar la partida, en orden de
// timestamp, y que varias partidas en paralelo no se mezclen entre sí.

// nuevoScalaFalso levanta un "Scala" de mentira que guarda cada lote
// recibido en el channel devuelto.
func nuevoScalaFalso(t *testing.T) (*client.ScalaClient, chan model.MatchBatch) {
	t.Helper()
	recibidos := make(chan model.MatchBatch, 10)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch model.MatchBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		recibidos <- batch
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	}))
	t.Cleanup(srv.Close)

	return client.NewScalaClient(srv.URL), recibidos
}

func evento(matchID, id, tipo string, ts time.Time) model.Event {
	return model.Event{
		EventID:     id,
		Timestamp:   ts,
		GameID:      "racing-circuit",
		GameVersion: "1.0",
		MatchID:     matchID,
		Type:        tipo,
	}
}

// esperarLote espera un lote del Scala falso, con un tiempo límite para
// que el test falle en vez de quedarse colgado si algo no llega.
func esperarLote(t *testing.T, recibidos chan model.MatchBatch) model.MatchBatch {
	t.Helper()
	select {
	case b := <-recibidos:
		return b
	case <-time.After(3 * time.Second):
		t.Fatal("no llegó ningún lote a Scala dentro del tiempo esperado")
		return model.MatchBatch{}
	}
}

func TestPartidaTerminadaEnviaLoteOrdenadoPorTimestamp(t *testing.T) {
	analyzer, recibidos := nuevoScalaFalso(t)
	m := NewManager(analyzer)

	base := time.Date(2026, 9, 25, 14, 0, 0, 0, time.UTC)

	// Los eventos se despachan desordenados a propósito; el Manager debe
	// ordenarlos por timestamp antes de enviarlos.
	m.Dispatch(evento("match-1", "evt-2", model.TypeLapCompleted, base.Add(2*time.Second)))
	m.Dispatch(evento("match-1", "evt-1", model.TypeMatchStarted, base))
	m.Dispatch(evento("match-1", "evt-3", model.TypeMatchFinished, base.Add(3*time.Second)))

	batch := esperarLote(t, recibidos)

	if batch.MatchID != "match-1" {
		t.Errorf("matchId esperado match-1, llegó %s", batch.MatchID)
	}
	if len(batch.Events) != 3 {
		t.Fatalf("se esperaban 3 eventos, llegaron %d", len(batch.Events))
	}
	esperado := []string{"evt-1", "evt-2", "evt-3"}
	for i, e := range batch.Events {
		if e.EventID != esperado[i] {
			t.Errorf("posición %d: se esperaba %s, llegó %s", i, esperado[i], e.EventID)
		}
	}
}

func TestPartidaTerminadaSeLiberaDelManager(t *testing.T) {
	analyzer, recibidos := nuevoScalaFalso(t)
	m := NewManager(analyzer)

	base := time.Now()
	m.Dispatch(evento("match-1", "evt-1", model.TypeMatchStarted, base))
	if m.ActiveMatches() != 1 {
		t.Errorf("se esperaba 1 partida activa, hay %d", m.ActiveMatches())
	}

	m.Dispatch(evento("match-1", "evt-2", model.TypeMatchFinished, base.Add(time.Second)))
	esperarLote(t, recibidos)

	// La partida se elimina del mapa justo después de recibir la
	// respuesta de Scala, así que se espera un momento a que ocurra.
	limite := time.Now().Add(2 * time.Second)
	for m.ActiveMatches() != 0 && time.Now().Before(limite) {
		time.Sleep(10 * time.Millisecond)
	}
	if m.ActiveMatches() != 0 {
		t.Errorf("se esperaban 0 partidas activas, hay %d", m.ActiveMatches())
	}
}

func TestPartidasEnParaleloNoSeMezclan(t *testing.T) {
	analyzer, recibidos := nuevoScalaFalso(t)
	m := NewManager(analyzer)

	partidas := []string{"match-a", "match-b", "match-c"}
	eventosPorPartida := 20 // sin contar MATCH_STARTED ni MATCH_FINISHED

	base := time.Now()
	var wg sync.WaitGroup
	for _, id := range partidas {
		wg.Add(1)
		go func(matchID string) {
			defer wg.Done()
			m.Dispatch(evento(matchID, matchID+"-start", model.TypeMatchStarted, base))
			for i := 0; i < eventosPorPartida; i++ {
				ts := base.Add(time.Duration(i+1) * time.Millisecond)
				m.Dispatch(evento(matchID, fmt.Sprintf("%s-lap-%d", matchID, i), model.TypeLapCompleted, ts))
			}
			m.Dispatch(evento(matchID, matchID+"-end", model.TypeMatchFinished, base.Add(time.Minute)))
		}(id)
	}
	wg.Wait()

	porPartida := map[string]model.MatchBatch{}
	for range partidas {
		b := esperarLote(t, recibidos)
		porPartida[b.MatchID] = b
	}

	for _, id := range partidas {
		b, ok := porPartida[id]
		if !ok {
			t.Errorf("no llegó el lote de %s", id)
			continue
		}
		if len(b.Events) != eventosPorPartida+2 {
			t.Errorf("%s: se esperaban %d eventos, llegaron %d", id, eventosPorPartida+2, len(b.Events))
		}
		for _, e := range b.Events {
			if e.MatchID != id {
				t.Errorf("%s recibió un evento de otra partida (%s)", id, e.MatchID)
			}
		}
	}
}

// --- Idempotencia, partidas cerradas y contadores en vivo ---

func eventoDeJuego(juego, matchID, id, tipo string, ts time.Time) model.Event {
	e := evento(matchID, id, tipo, ts)
	e.GameID = juego
	return e
}

func TestEventoDuplicadoSeIgnora(t *testing.T) {
	analyzer, recibidos := nuevoScalaFalso(t)
	m := NewManager(analyzer)
	base := time.Now()

	if r := m.Dispatch(evento("m1", "e1", model.TypeMatchStarted, base)); r != Accepted {
		t.Fatalf("el primer envío debía aceptarse, fue %v", r)
	}
	if r := m.Dispatch(evento("m1", "e1", model.TypeMatchStarted, base)); r != Duplicate {
		t.Fatalf("el reenvío debía ser Duplicate, fue %v", r)
	}
	m.Dispatch(evento("m1", "e2", model.TypeMatchFinished, base.Add(time.Second)))

	b := esperarLote(t, recibidos)
	if len(b.Events) != 2 {
		t.Errorf("se esperaban 2 eventos (sin duplicados), llegaron %d", len(b.Events))
	}
}

func TestEventoTardioDeUnaPartidaTerminadaSeIgnora(t *testing.T) {
	analyzer, recibidos := nuevoScalaFalso(t)
	m := NewManager(analyzer)
	base := time.Now()

	m.Dispatch(evento("m1", "e1", model.TypeMatchStarted, base))
	m.Dispatch(evento("m1", "e2", model.TypeMatchFinished, base.Add(time.Second)))
	esperarLote(t, recibidos)

	// Reintento tardío del MATCH_FINISHED (el emisor no vio la respuesta).
	limite := time.Now().Add(2 * time.Second)
	var r DispatchResult
	for time.Now().Before(limite) {
		r = m.Dispatch(evento("m1", "e2", model.TypeMatchFinished, base.Add(time.Second)))
		if r == Closed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if r != Closed {
		t.Fatalf("el evento tardío debía ser Closed, fue %v", r)
	}
	select {
	case b := <-recibidos:
		t.Fatalf("no debía enviarse un segundo lote a Scala, llegó %+v", b)
	case <-time.After(200 * time.Millisecond):
	}
	if m.ActiveMatches() != 0 {
		t.Errorf("no debía quedar ninguna partida activa, hay %d", m.ActiveMatches())
	}
}

func TestContadoresEnVivoPorJuego(t *testing.T) {
	analyzer, recibidos := nuevoScalaFalso(t)
	m := NewManager(analyzer)
	base := time.Now()

	// Dos partidas de conecta4 (una termina) y una de 2048 (en curso).
	m.Dispatch(eventoDeJuego("connect4", "c1", "c1-s", model.TypeMatchStarted, base))
	m.Dispatch(eventoDeJuego("connect4", "c2", "c2-s", model.TypeMatchStarted, base))
	m.Dispatch(eventoDeJuego("connect4", "c1", "c1-m", "MOVE", base.Add(time.Second)))
	m.Dispatch(eventoDeJuego("game-2048", "g1", "g1-s", model.TypeMatchStarted, base))
	m.Dispatch(eventoDeJuego("connect4", "c1", "c1-f", model.TypeMatchFinished, base.Add(2*time.Second)))
	esperarLote(t, recibidos)

	snap := m.Live()
	if snap.Totals.Started != 3 || snap.Totals.Finished != 1 || snap.Totals.Active != 2 {
		t.Errorf("totales inesperados: %+v", snap.Totals)
	}
	if len(snap.Games) != 2 || snap.Games[0].GameID != "connect4" || snap.Games[1].GameID != "game-2048" {
		t.Fatalf("juegos inesperados (deben venir ordenados): %+v", snap.Games)
	}
	c4 := snap.Games[0]
	if c4.Started != 2 || c4.Finished != 1 || c4.Active != 1 || c4.Events != 4 || c4.EventsByType["MOVE"] != 1 {
		t.Errorf("contadores de connect4 inesperados: %+v", c4)
	}
}

func TestContadoresNoCuentanDuplicados(t *testing.T) {
	analyzer, _ := nuevoScalaFalso(t)
	m := NewManager(analyzer)
	base := time.Now()
	for i := 0; i < 5; i++ {
		m.Dispatch(eventoDeJuego("connect4", "c1", "c1-s", model.TypeMatchStarted, base))
	}
	if s := m.Live().Totals.Started; s != 1 {
		t.Errorf("5 reenvíos del mismo evento deben contar 1 inicio, cuenta %d", s)
	}
}
