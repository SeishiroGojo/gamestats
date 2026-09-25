package validate

import (
	"fmt"

	"gamestats/ingestion/internal/model"
)

// playerRequired indica qué tipos de evento deben traer playerId.
var playerRequired = map[string]bool{
	model.TypePlayerJoined:    true,
	model.TypePlayerLeft:      true,
	model.TypeDisconnected:    true,
	model.TypeLapCompleted:    true,
	model.TypePositionChanged: true,
	model.TypePenaltyApplied:  true,
	model.TypePitStop:         true,
	model.TypeRaceIncident:    true,
}

// Validate comprueba la información mínima necesaria de un evento.
// No valida reglas de negocio (eso es responsabilidad de Scala); solo
// garantiza que el evento tiene lo mínimo para poder ser procesado y
// agrupado por partida.
func Validate(e model.Event) error {
	if e.EventID == "" {
		return fmt.Errorf("eventId es obligatorio")
	}
	if e.Timestamp.IsZero() {
		return fmt.Errorf("timestamp es obligatorio")
	}
	if e.GameID == "" {
		return fmt.Errorf("gameId es obligatorio")
	}
	if e.MatchID == "" {
		return fmt.Errorf("matchId es obligatorio")
	}
	if e.Type == "" {
		return fmt.Errorf("type es obligatorio")
	}
	if playerRequired[e.Type] && e.PlayerID == "" {
		return fmt.Errorf("playerId es obligatorio para el tipo %s", e.Type)
	}
	return nil
}
