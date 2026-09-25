package gamestats

import com.sun.net.httpserver.{HttpExchange, HttpHandler, HttpServer}
import gamestats.analysis.{Rules, Statistics}
import gamestats.model.MatchBatch
import io.circe.generic.semiauto._
import io.circe.parser.decode
import io.circe.syntax._
import io.circe.Encoder

import java.io.OutputStream
import java.net.InetSocketAddress
import java.nio.charset.StandardCharsets
import scala.io.Source
import scala.util.{Failure, Success, Try}

/** Módulo de análisis de GameStats.
  *
  * Recibe (POST /analyze) el lote de eventos de una partida terminada,
  * enviado por el módulo Go, y responde con las estadísticas y reglas
  * calculadas de forma funcional (ver analysis.Statistics / analysis.Rules).
  */
object Main {

  final case class AnalysisResult(
      matchStats: Statistics.MatchStats,
      comebacks: List[Rules.ComebackResult]
  )
  object AnalysisResult {
    implicit val encoder: Encoder[AnalysisResult] = deriveEncoder
  }

  def main(args: Array[String]): Unit = {
    val port = sys.env.get("PORT").flatMap(p => Try(p.toInt).toOption).getOrElse(8081)
    val server = HttpServer.create(new InetSocketAddress(port), 0)

    server.createContext("/analyze", analyzeHandler)
    server.createContext("/health", healthHandler)
    server.setExecutor(null) // usa un executor por defecto de un solo hilo; suficiente para la demo
    server.start()
    println(s"GameStats analysis escuchando en :$port")
  }

  private val analyzeHandler: HttpHandler = (exchange: HttpExchange) => {
    if (exchange.getRequestMethod != "POST") {
      respond(exchange, 405, """{"error":"método no permitido"}""")
    } else {
      val body = Source.fromInputStream(exchange.getRequestBody, StandardCharsets.UTF_8.name()).mkString

      decode[MatchBatch](body) match {
        case Left(error) =>
          respond(exchange, 400, s"""{"error":"JSON inválido: ${error.getMessage.replace("\"", "'")}"}""")
        case Right(batch) =>
          val stats     = Statistics.matchStats(batch.matchId, batch.events)
          val comebacks = Rules.detectComebacks(batch.events)
          val result    = AnalysisResult(stats, comebacks)
          respond(exchange, 200, result.asJson.noSpaces)
      }
    }
  }

  private val healthHandler: HttpHandler = (exchange: HttpExchange) => {
    respond(exchange, 200, """{"status":"ok"}""")
  }

  private def respond(exchange: HttpExchange, status: Int, body: String): Unit = {
    val bytes = body.getBytes(StandardCharsets.UTF_8)
    exchange.getResponseHeaders.add("Content-Type", "application/json")
    exchange.sendResponseHeaders(status, bytes.length.toLong)
    val os: OutputStream = exchange.getResponseBody
    Try(os.write(bytes)) match {
      case Success(_) => ()
      case Failure(e) => System.err.println(s"error escribiendo respuesta: ${e.getMessage}")
    }
    os.close()
  }
}
