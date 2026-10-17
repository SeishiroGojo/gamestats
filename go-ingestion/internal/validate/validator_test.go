package validate

import (
	"testing"
	"time"

	"gamestats/ingestion/internal/model"
)

// Estos tests verifican que Validate() acepte eventos correctos y
// rechace eventos a los que les falta información obligatoria.
//
// TAREA: ya hay 2 casos de ejemplo escritos y funcionando. Sumá los
// casos marcados como "TODO" más abajo, siguiendo exactamente el mismo
// patrón (copiá una fila, cambiá los valores). No hace falta tocar nada
// más en este archivo ni en ningún otro.
//
// Para correr los tests: parado en la carpeta go-ingestion, ejecutar
//   go test ./...
// Todos deberían pasar (PASS) al terminar.

func TestValidate(t *testing.T) {
	baseEvent := func() model.Event {
		return model.Event{
			EventID:     "evt-1",
			Timestamp:   time.Now(),
			GameID:      "racing-circuit",
			GameVersion: "1.0",
			MatchID:     "match-1",
			Type:        model.TypeMatchStarted,
		}
	}

	cases := []struct {
		name      string
		modify    func(e model.Event) model.Event
		wantError bool
	}{
		{
			name:      "evento válido de MATCH_STARTED pasa sin error",
			modify:    func(e model.Event) model.Event { return e },
			wantError: false,
		},
		{
			name: "falta eventId debe fallar",
			modify: func(e model.Event) model.Event {
				e.EventID = ""
				return e
			},
			wantError: true,
		},

		{
			name: "falta matchId debe fallar",
			modify: func(e model.Event) model.Event {
				e.MatchID = ""
				return e
			},
			wantError: true,
		},
		{
			name: "falta type debe fallar",
			modify: func(e model.Event) model.Event {
				e.Type = ""
				return e
			},
			wantError: true,
		},
		{
			name: "LAP_COMPLETED sin playerId debe fallar",
			modify: func(e model.Event) model.Event {
				e.Type = model.TypeLapCompleted
				e.PlayerID = ""
				return e
			},
			wantError: true,
		},
		{
			name: "LAP_COMPLETED con playerId pasa sin error",
			modify: func(e model.Event) model.Event {
				e.Type = model.TypeLapCompleted
				e.PlayerID = "player-1"
				return e
			},
			wantError: false,
		},
		{
			name: "timestamp vacío debe fallar",
			modify: func(e model.Event) model.Event {
				e.Timestamp = time.Time{}
				return e
			},
			wantError: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			event := tc.modify(baseEvent())
			err := Validate(event)

			if tc.wantError && err == nil {
				t.Errorf("se esperaba un error de validación, pero no hubo ninguno")
			}
			if !tc.wantError && err != nil {
				t.Errorf("no se esperaba error, pero Validate devolvió: %v", err)
			}
		})
	}
}
