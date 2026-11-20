# Integración con GameConsole (GameStats v2)

GameStats v2 mantiene intacto el análisis del juego de carreras (`racing-circuit`) y agrega lo que necesita una consola de varios juegos.

## Cambios

| Servicio | Cambio |
|---|---|
| Ingestión (Go) | Idempotencia por `eventId` (reenviar un evento no lo duplica); los eventos tardíos de una partida ya cerrada se ignoran (ventana de 2 min); contadores en vivo por juego |
| Análisis (Scala) | Resumen genérico por partida para cualquier juego distinto de carreras y estadísticas agregadas por juego y generales |

## Endpoints nuevos

| Método y ruta | Servicio (puerto) | Respuesta |
|---|---|---|
| `GET /stats/live` | Ingestión (8080) | `{generatedAt, totals{started,finished,active,events}, games[{gameId,started,finished,active,events,eventsByType}]}` |
| `GET /stats/games` | Análisis (8081) | Lista por juego: `matches, won, lost, draws, abandoned, winRate, avgDurationMs, avgMoves, avgAiDecisions, avgScore, bestScore` |
| `GET /stats/overall` | Análisis (8081) | `games, matches, won, lost, draws, abandoned, winRate, avgDurationMs, totalMoves, totalAiDecisions` |

`POST /events` agrega a su respuesta los campos `duplicates` (reenvíos ignorados) y `late` (eventos de partidas ya cerradas).

## Cómo se calcula el resumen de una partida (`MatchSummaries.summarize`)

* `outcome`: de `MATCH_FINISHED.data.outcome` (`WON`, `LOST`, `DRAW`, `ABANDONED`); si falta, de `PLAYER_WON` / `PLAYER_LOST`.
* `moves`: cantidad de `PLAYER_ACTION` y `MOVE`; `aiDecisions`: cantidad de `AI_DECISION`.
* `score`: `MATCH_FINISHED.data.score` (si el juego lo tiene); `durationMs`: último − primer `timestamp`.
* `winRate` = victorias / partidas completadas (los abandonos no cuentan).

Los resúmenes se guardan en la tabla SQLite `match_summaries` y las agregaciones son funciones puras (`Aggregates`), con pruebas en `AggregatesSuite`.
