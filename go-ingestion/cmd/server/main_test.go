package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gamestats/ingestion/internal/client"
	"gamestats/ingestion/internal/match"
	"gamestats/ingestion/internal/model"
)

// Pruebas de integración del servidor de ingestión: se usa el handler real
// (validación + Manager + cliente HTTP) con un "Scala" falso por detrás.

type respuestaEventos struct {
	Accepted int      `json:"accepted"`
	Rejected []string `json:"rejected"`
}

func nuevoEntorno(t *testing.T) (http.Handler, chan model.MatchBatch) {
	t.Helper()
	recibidos := make(chan model.MatchBatch, 10)
	scala := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b model.MatchBatch
		json.NewDecoder(r.Body).Decode(&b)
		recibidos <- b
		w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(scala.Close)

	manager := match.NewManager(client.NewScalaClient(scala.URL))
	return eventsHandler(manager), recibidos
}

func publicar(t *testing.T, h http.Handler, cuerpo string) (*httptest.ResponseRecorder, respuestaEventos) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/events", strings.NewReader(cuerpo))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var r respuestaEventos
	json.Unmarshal(rec.Body.Bytes(), &r)
	return rec, r
}

func leerData(t *testing.T, nombre string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("../../../data", nombre))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestEventsRechazaMetodosQueNoSonPost(t *testing.T) {
	h, _ := nuevoEntorno(t)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/events", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("se esperaba 405, fue %d", rec.Code)
	}
}

func TestEventsRechazaJSONMalformado(t *testing.T) {
	h, _ := nuevoEntorno(t)
	rec, _ := publicar(t, h, `{esto no es json`)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("se esperaba 400, fue %d", rec.Code)
	}
}

func TestEventsAceptaUnEventoSueltoOUnArreglo(t *testing.T) {
	h, _ := nuevoEntorno(t)
	evento := `{"eventId":"a1","timestamp":"2026-10-01T15:00:00Z","gameId":"racing-circuit","matchId":"m-suelto","type":"MATCH_STARTED"}`

	rec, r := publicar(t, h, evento)
	if rec.Code != http.StatusAccepted || r.Accepted != 1 {
		t.Errorf("evento suelto: código %d, aceptados %d", rec.Code, r.Accepted)
	}
	rec, r = publicar(t, h, "["+strings.Replace(evento, "a1", "a2", 1)+"]")
	if rec.Code != http.StatusAccepted || r.Accepted != 1 {
		t.Errorf("arreglo: código %d, aceptados %d", rec.Code, r.Accepted)
	}
}

func TestEventsInvalidosDevuelven207ConElDetalle(t *testing.T) {
	h, _ := nuevoEntorno(t)
	rec, r := publicar(t, h, leerData(t, "scenario-invalid-events.json"))

	if rec.Code != http.StatusMultiStatus {
		t.Errorf("se esperaba 207, fue %d", rec.Code)
	}
	if r.Accepted != 0 || len(r.Rejected) != 6 {
		t.Errorf("se esperaban 0 aceptados y 6 rechazados, hubo %d y %d", r.Accepted, len(r.Rejected))
	}
}

func TestEscenarioCompletoLlegaAScalaUnaSolaVezYOrdenado(t *testing.T) {
	h, recibidos := nuevoEntorno(t)
	rec, r := publicar(t, h, leerData(t, "scenario-comeback.json"))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("se esperaba 202, fue %d", rec.Code)
	}

	select {
	case lote := <-recibidos:
		if lote.MatchID != "match-comeback" {
			t.Errorf("partida inesperada: %s", lote.MatchID)
		}
		if len(lote.Events) != r.Accepted {
			t.Errorf("Scala recibió %d eventos y Go aceptó %d", len(lote.Events), r.Accepted)
		}
		for i := 1; i < len(lote.Events); i++ {
			if lote.Events[i].Timestamp.Before(lote.Events[i-1].Timestamp) {
				t.Fatalf("el lote no está ordenado por timestamp en la posición %d", i)
			}
		}
		if ultimo := lote.Events[len(lote.Events)-1]; ultimo.Type != model.TypeMatchFinished {
			t.Errorf("el último evento debería ser MATCH_FINISHED, es %s", ultimo.Type)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Scala no recibió el lote de la partida")
	}

	select {
	case <-recibidos:
		t.Error("Scala recibió la partida más de una vez")
	case <-time.After(200 * time.Millisecond):
	}
}

func TestHealthIndicaPartidasActivas(t *testing.T) {
	manager := match.NewManager(client.NewScalaClient("http://127.0.0.1:1"))
	rec := httptest.NewRecorder()
	healthHandler(manager).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	var cuerpo map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &cuerpo); err != nil {
		t.Fatal(err)
	}
	if cuerpo["status"] != "ok" || cuerpo["activeMatches"] != float64(0) {
		t.Errorf("respuesta de /health inesperada: %v", cuerpo)
	}
}
