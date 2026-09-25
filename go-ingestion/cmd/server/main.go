// Servidor de ingestión de GameStats.
//
// Responsabilidades (ver docs/event-spec.md):
//   - Recibir eventos por HTTP (POST /events).
//   - Validar su estructura mínima.
//   - Agruparlos por partida, procesando varias partidas en paralelo
//     mediante goroutines y channels (ver internal/match).
//   - Al terminar una partida, enviar el lote completo a Scala.
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"gamestats/ingestion/internal/client"
	"gamestats/ingestion/internal/match"
	"gamestats/ingestion/internal/model"
	"gamestats/ingestion/internal/validate"
)

func main() {
	port := getEnv("PORT", "8080")
	scalaURL := getEnv("SCALA_ANALYSIS_URL", "http://localhost:8081")

	analyzer := client.NewScalaClient(scalaURL)
	manager := match.NewManager(analyzer)

	mux := http.NewServeMux()
	mux.HandleFunc("/events", eventsHandler(manager))
	mux.HandleFunc("/health", healthHandler(manager))

	log.Printf("GameStats ingestion escuchando en :%s (Scala en %s)", port, scalaURL)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}

// eventsHandler acepta un único evento o un arreglo de eventos por request,
// para que el simulador pueda enviar en lote sin cambiar el contrato.
func eventsHandler(manager *match.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "método no permitido", http.StatusMethodNotAllowed)
			return
		}

		events, err := decodeEvents(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var rejected []string
		for _, e := range events {
			if err := validate.Validate(e); err != nil {
				log.Printf("evento rechazado (%s): %v", e.EventID, err)
				rejected = append(rejected, e.EventID+": "+err.Error())
				continue
			}
			manager.Dispatch(e)
		}

		w.Header().Set("Content-Type", "application/json")
		if len(rejected) > 0 {
			w.WriteHeader(http.StatusMultiStatus)
		} else {
			w.WriteHeader(http.StatusAccepted)
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"accepted": len(events) - len(rejected),
			"rejected": rejected,
		})
	}
}

func decodeEvents(r *http.Request) ([]model.Event, error) {
	dec := json.NewDecoder(r.Body)

	// Detecta si el body es un arreglo o un único objeto.
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, err
	}

	var events []model.Event
	if err := json.Unmarshal(raw, &events); err == nil {
		return events, nil
	}

	var single model.Event
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, err
	}
	return []model.Event{single}, nil
}

func healthHandler(manager *match.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":         "ok",
			"activeMatches":  manager.ActiveMatches(),
		})
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
