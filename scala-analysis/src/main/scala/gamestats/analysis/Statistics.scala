package gamestats.analysis

import gamestats.model.{Event, EventTypes}
import io.circe.generic.semiauto._
import io.circe.{Decoder, Encoder}

/** Cálculo de estadísticas de carreras, implementado con colecciones
  * funcionales (filter / map / groupBy / fold) sobre List[Event], tal
  * como pide el enunciado para el módulo Scala.
  */
object Statistics {

  final case class PlayerStats(
      playerId: String,
      bestLapMs: Option[Long],
      avgLapMs: Option[Double],
      totalPenaltyMs: Long,
      penaltyCount: Int,
      finalPosition: Option[Int]
  )
  object PlayerStats {
    implicit val encoder: Encoder[PlayerStats] = deriveEncoder
  }

  final case class MatchStats(
      matchId: String,
      fastestLapMs: Option[Long],
      fastestLapPlayer: Option[String],
      avgLapMsAllPlayers: Option[Double],
      playerStats: List[PlayerStats]
  )
  object MatchStats {
    implicit val encoder: Encoder[MatchStats] = deriveEncoder
  }

  /** filter: solo los eventos de vuelta completada. */
  def lapEvents(events: List[Event]): List[Event] =
    events.filter(_.`type` == EventTypes.LapCompleted)

  def lapTimeMs(e: Event): Option[Long] =
    e.data.get("lapTimeMs").flatMap(_.asNumber).flatMap(_.toLong)

  private def positionField(e: Event, field: String): Option[Int] =
    e.data.get(field).flatMap(_.asNumber).flatMap(_.toInt)

  /** map: extrae la posición final de cada jugador desde MATCH_FINISHED. */
  def finalStandings(events: List[Event]): Map[String, Int] =
    events
      .find(_.`type` == EventTypes.MatchFinished)
      .flatMap(_.data.get("finalStandings").flatMap(_.asArray))
      .map { arr =>
        arr.flatMap { json =>
          for {
            obj <- json.asObject
            pid <- obj("playerId").flatMap(_.asString)
            pos <- obj("finalPosition").flatMap(_.asNumber).flatMap(_.toInt)
          } yield pid -> pos
        }.toMap
      }
      .getOrElse(Map.empty)

  /** groupBy + fold: estadísticas agregadas por jugador. */
  def playerStats(events: List[Event]): List[PlayerStats] = {
    val laps      = lapEvents(events).groupBy(_.playerId.getOrElse("unknown"))
    val penalties = events.filter(_.`type` == EventTypes.PenaltyApplied).groupBy(_.playerId.getOrElse("unknown"))
    val finals    = finalStandings(events)

    val playerIds = (laps.keySet ++ penalties.keySet).toList.sorted

    playerIds.map { playerId =>
      val times = laps.getOrElse(playerId, Nil).flatMap(lapTimeMs)
      val best  = if (times.nonEmpty) Some(times.min) else None
      // fold: promedio de tiempos de vuelta.
      val avg =
        if (times.nonEmpty)
          Some(times.foldLeft(0L)(_ + _).toDouble / times.size)
        else None

      val penEvents = penalties.getOrElse(playerId, Nil)
      val penTotal = penEvents
        .flatMap(_.data.get("timeMs").flatMap(_.asNumber).flatMap(_.toLong))
        .foldLeft(0L)(_ + _)

      PlayerStats(playerId, best, avg, penTotal, penEvents.size, finals.get(playerId))
    }
  }

  /** Estadística agregada por partida, combinando LAP_COMPLETED de todos
    * los jugadores para obtener la vuelta más rápida de la carrera.
    */
  def matchStats(matchId: String, events: List[Event]): MatchStats = {
    val pStats    = playerStats(events)
    val allTimes  = lapEvents(events).flatMap(lapTimeMs)
    val fastest   = if (allTimes.nonEmpty) Some(allTimes.min) else None
    val fastestBy = fastest.flatMap(f => lapEvents(events).collectFirst {
      case e if lapTimeMs(e).contains(f) => e.playerId.getOrElse("unknown")
    })
    val avgAll = if (allTimes.nonEmpty) Some(allTimes.foldLeft(0L)(_ + _).toDouble / allTimes.size) else None

    MatchStats(matchId, fastest, fastestBy, avgAll, pStats)
  }

  /** Evolución de posición por vuelta: combina LAP_COMPLETED y
    * POSITION_CHANGED de un mismo jugador (estadística que requiere
    * relacionar varios tipos de evento).
    */
  def positionEvolution(events: List[Event]): Map[String, List[(Int, Int)]] = {
    val relevant = events.filter(e =>
      (e.`type` == EventTypes.LapCompleted && positionField(e, "position").isDefined) ||
        (e.`type` == EventTypes.PositionChanged && positionField(e, "toPosition").isDefined)
    )
    relevant
      .groupBy(_.playerId.getOrElse("unknown"))
      .map { case (playerId, evs) =>
        val points = evs.flatMap { e =>
          val lapNum = positionField(e, "lapNumber").getOrElse(0)
          val pos    = if (e.`type` == EventTypes.LapCompleted) positionField(e, "position") else positionField(e, "toPosition")
          pos.map(p => (lapNum, p))
        }.sortBy(_._1)
        playerId -> points
      }
  }
}
