package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"gamestats/ingestion/internal/model"
)

// Estos tests recorren los archivos de ejemplo de la carpeta data/ y
// comprueban que Validate() se comporta como documenta docs/demo-guide.md:
// los escenarios válidos se aceptan completos y el de eventos inválidos se
// rechaza completo. Así, si alguien edita un JSON de prueba y lo rompe, el
// CI lo detecta.

const dataDir = "../../../data"

func cargarEventos(t *testing.T, nombre string) []model.Event {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dataDir, nombre))
	if err != nil {
		t.Fatalf("no se pudo leer %s: %v", nombre, err)
	}
	var eventos []model.Event
	if err := json.Unmarshal(raw, &eventos); err != nil {
		t.Fatalf("%s no es un arreglo JSON de eventos válido: %v", nombre, err)
	}
	return eventos
}

func TestEscenariosValidosPasanLaValidacion(t *testing.T) {
	archivos := []string{
		"sample-events.json",
		"scenario-comeback.json",
		"scenario-no-comeback.json",
		"scenario-disconnect.json",
	}
	for _, nombre := range archivos {
		t.Run(nombre, func(t *testing.T) {
			eventos := cargarEventos(t, nombre)
			if len(eventos) == 0 {
				t.Fatal("el escenario no tiene eventos")
			}

			ids := map[string]bool{}
			terminadas := 0
			for _, e := range eventos {
				if err := Validate(e); err != nil {
					t.Errorf("evento %s debería ser válido: %v", e.EventID, err)
				}
				if ids[e.EventID] {
					t.Errorf("eventId repetido: %s", e.EventID)
				}
				ids[e.EventID] = true

				if e.MatchID != eventos[0].MatchID {
					t.Errorf("el evento %s pertenece a otra partida (%s)", e.EventID, e.MatchID)
				}
				if e.Type == model.TypeMatchFinished {
					terminadas++
				}
			}
			if terminadas != 1 {
				t.Errorf("se esperaba exactamente 1 MATCH_FINISHED, hay %d", terminadas)
			}
		})
	}
}

func TestEscenarioDeEventosInvalidosSeRechazaCompleto(t *testing.T) {
	eventos := cargarEventos(t, "scenario-invalid-events.json")
	if len(eventos) != 6 {
		t.Fatalf("se esperaban 6 eventos inválidos de prueba, hay %d", len(eventos))
	}
	for _, e := range eventos {
		if err := Validate(e); err == nil {
			t.Errorf("el evento %s debería haber sido rechazado", e.EventID)
		}
	}
}
