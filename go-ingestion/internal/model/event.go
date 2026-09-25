package model

import "time"

// Event representa un hecho ocurrido dentro de una partida.
// La estructura del "envoltorio" es común a cualquier juego; el campo Data
// contiene la información específica del tipo de evento (ver docs/event-spec.md).
type Event struct {
	EventID     string                 `json:"eventId"`
	Timestamp   time.Time              `json:"timestamp"`
	GameID      string                 `json:"gameId"`
	GameVersion string                 `json:"gameVersion"`
	MatchID     string                 `json:"matchId"`
	PlayerID    string                 `json:"playerId,omitempty"`
	Type        string                 `json:"type"`
	Action      string                 `json:"action,omitempty"`
	Data        map[string]interface{} `json:"data,omitempty"`
}

// Tipos de evento conocidos para el juego de carreras.
// GameStats no está limitado a estos, pero el módulo Go los usa para
// decidir cuándo abrir y cuándo cerrar el ciclo de vida de una partida.
const (
	TypeMatchStarted    = "MATCH_STARTED"
	TypePlayerJoined    = "PLAYER_JOINED"
	TypePlayerLeft      = "PLAYER_LEFT"
	TypeDisconnected    = "PLAYER_DISCONNECTED"
	TypeLapCompleted    = "LAP_COMPLETED"
	TypePositionChanged = "POSITION_CHANGED"
	TypePenaltyApplied  = "PENALTY_APPLIED"
	TypePitStop         = "PIT_STOP"
	TypeRaceIncident    = "RACE_INCIDENT"
	TypeMatchFinished   = "MATCH_FINISHED"
)

// MatchBatch es lo que Go envía a Scala cuando una partida termina:
// todos los eventos de esa partida, en orden de timestamp.
type MatchBatch struct {
	MatchID string  `json:"matchId"`
	Events  []Event `json:"events"`
}
