package gamestats.model

import java.time.Instant
import io.circe._
import io.circe.generic.semiauto._

/** Evento tal como lo entrega el módulo Go, ver docs/event-spec.md. */
final case class Event(
    eventId: String,
    timestamp: Instant,
    gameId: String,
    gameVersion: String,
    matchId: String,
    playerId: Option[String],
    `type`: String,
    action: Option[String],
    data: Map[String, Json]
)

object Event {
  implicit val instantDecoder: Decoder[Instant] =
    Decoder.decodeString.emap(s => scala.util.Try(Instant.parse(s)).toEither.left.map(_ => s"Instant inválido: $s"))
  implicit val instantEncoder: Encoder[Instant] =
    Encoder.encodeString.contramap(_.toString)

  implicit val decoder: Decoder[Event] = deriveDecoder
  implicit val encoder: Encoder[Event] = deriveEncoder
}

/** Lote que Go envía cuando una partida termina: todos sus eventos en orden. */
final case class MatchBatch(matchId: String, events: List[Event])

object MatchBatch {
  implicit val decoder: Decoder[MatchBatch] = deriveDecoder
  implicit val encoder: Encoder[MatchBatch] = deriveEncoder
}

/** Tipos de evento conocidos para el juego de carreras. */
object EventTypes {
  val MatchStarted    = "MATCH_STARTED"
  val PlayerJoined    = "PLAYER_JOINED"
  val PlayerLeft      = "PLAYER_LEFT"
  val Disconnected    = "PLAYER_DISCONNECTED"
  val LapCompleted    = "LAP_COMPLETED"
  val PositionChanged = "POSITION_CHANGED"
  val PenaltyApplied  = "PENALTY_APPLIED"
  val PitStop         = "PIT_STOP"
  val RaceIncident    = "RACE_INCIDENT"
  val MatchFinished   = "MATCH_FINISHED"
}
