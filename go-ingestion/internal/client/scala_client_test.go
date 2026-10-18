package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gamestats/ingestion/internal/model"
)

func loteDePrueba() model.MatchBatch {
	return model.MatchBatch{
		MatchID: "match-x",
		Events: []model.Event{{
			EventID: "e1", Timestamp: time.Now(), GameID: "racing-circuit",
			MatchID: "match-x", Type: model.TypeMatchStarted,
		}},
	}
}

func TestSendMatchEnviaElLoteYDevuelveLaRespuestaDeScala(t *testing.T) {
	var recibido model.MatchBatch
	var ruta, metodo, contentType string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ruta, metodo, contentType = r.URL.Path, r.Method, r.Header.Get("Content-Type")
		json.NewDecoder(r.Body).Decode(&recibido)
		w.Write([]byte(`{"matchStats":{"matchId":"match-x"}}`))
	}))
	defer srv.Close()

	resp, err := NewScalaClient(srv.URL).SendMatch(loteDePrueba())
	if err != nil {
		t.Fatalf("no se esperaba error: %v", err)
	}
	if metodo != http.MethodPost || ruta != "/analyze" {
		t.Errorf("se esperaba POST /analyze, fue %s %s", metodo, ruta)
	}
	if contentType != "application/json" {
		t.Errorf("Content-Type inesperado: %q", contentType)
	}
	if recibido.MatchID != "match-x" || len(recibido.Events) != 1 {
		t.Errorf("Scala recibió un lote distinto al enviado: %+v", recibido)
	}
	if !strings.Contains(resp, `"matchId":"match-x"`) {
		t.Errorf("la respuesta de Scala no se devolvió completa: %s", resp)
	}
}

func TestSendMatchDevuelveErrorSiScalaRespondeConFallo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"JSON inválido"}`, http.StatusBadRequest)
	}))
	defer srv.Close()

	_, err := NewScalaClient(srv.URL).SendMatch(loteDePrueba())
	if err == nil {
		t.Fatal("se esperaba un error cuando Scala responde 400")
	}
	if !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "match-x") {
		t.Errorf("el error debería mencionar el código y la partida: %v", err)
	}
}

func TestSendMatchDevuelveErrorSiScalaNoEstaDisponible(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nadie escucha en esa dirección

	if _, err := NewScalaClient(url).SendMatch(loteDePrueba()); err == nil {
		t.Fatal("se esperaba un error de conexión")
	}
}
