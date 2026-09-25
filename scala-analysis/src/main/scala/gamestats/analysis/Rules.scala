package gamestats.analysis

import gamestats.model.{Event, EventTypes}
import io.circe.generic.semiauto._
import io.circe.Encoder

/** Reglas que requieren considerar una secuencia o combinación de eventos,
  * no un evento aislado (requisito del enunciado, sección 5 / 9).
  */
object Rules {

  final case class ComebackResult(
      playerId: String,
      worstPosition: Int,
      finalPosition: Int,
      positionsGained: Int
  )
  object ComebackResult {
    implicit val encoder: Encoder[ComebackResult] = deriveEncoder
  }

  /** Un jugador cuenta como "remontada" si la diferencia entre la peor
    * posición que tuvo en algún punto de la carrera y su posición final
    * es mayor o igual a este umbral.
    */
  val ComebackThreshold = 3

  /** Reconstruye, para cada jugador, la secuencia de posiciones ocupadas
    * durante la carrera (a partir de LAP_COMPLETED y POSITION_CHANGED),
    * y compara la peor posición alcanzada contra la posición final.
    */
  def detectComebacks(events: List[Event]): List[ComebackResult] = {
    val positionEvents = events.filter(e =>
      e.`type` == EventTypes.PositionChanged || e.`type` == EventTypes.LapCompleted
    )
    val byPlayer = positionEvents.groupBy(_.playerId.getOrElse("unknown"))
    val finals   = Statistics.finalStandings(events)

    byPlayer.toList.flatMap { case (playerId, evs) =>
      val positions = evs.flatMap(positionOf)
      for {
        worst    <- if (positions.nonEmpty) Some(positions.max) else None
        finalPos <- finals.get(playerId)
        gained = worst - finalPos
        if gained >= ComebackThreshold
      } yield ComebackResult(playerId, worst, finalPos, gained)
    }.sortBy(-_.positionsGained)
  }

  private def positionOf(e: Event): Option[Int] = e.`type` match {
    case EventTypes.PositionChanged => e.data.get("toPosition").flatMap(_.asNumber).flatMap(_.toInt)
    case EventTypes.LapCompleted    => e.data.get("position").flatMap(_.asNumber).flatMap(_.toInt)
    case _                          => None
  }
}
