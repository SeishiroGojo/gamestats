# GameStats — Especificación de eventos (Carreras)

## 1. Juego elegido

**Carreras** (estilo circuito con vueltas, posiciones, penalizaciones y pit stops).
El sistema queda diseñado para ser genérico (el envoltorio del evento no depende
del juego), pero los tipos de evento y las estadísticas concretas de este
documento están pensados para carreras.

## 2. Envoltorio común del evento

Todo evento que entra al sistema por el módulo Go tiene esta forma:

```json
{
  "eventId": "evt-0001",
  "timestamp": "2026-09-25T14:32:10.500Z",
  "gameId": "racing-circuit",
  "gameVersion": "1.0",
  "matchId": "match-42",
  "playerId": "player-1",
  "type": "LAP_COMPLETED",
  "action": null,
  "data": { }
}
```

| Campo | Obligatorio | Descripción |
|---|---|---|
| `eventId` | Sí | Identificador único del evento (UUID o similar). |
| `timestamp` | Sí | Instante ISO-8601 en que ocurrió. |
| `gameId` | Sí | Identificador del juego (`racing-circuit`). |
| `gameVersion` | Sí | Versión del juego que generó el evento. |
| `matchId` | Sí | Partida/carrera a la que pertenece. |
| `playerId` | Depende del tipo | Jugador asociado (no aplica a eventos globales de partida). |
| `type` | Sí | Tipo general del evento (ver tabla abajo). |
| `action` | No | Acción específica, cuando aplica. |
| `data` | Depende del tipo | Información específica del evento. |

## 3. Tipos de evento

| `type` | `playerId` requerido | `data` esperado | Descripción |
|---|---|---|---|
| `MATCH_STARTED` | No | `{ track, totalLaps, players: [playerId...] }` | Arranca la carrera. |
| `PLAYER_JOINED` | Sí | `{ playerName, carId }` | Un jugador entra a la partida. |
| `PLAYER_LEFT` / `PLAYER_DISCONNECTED` | Sí | `{ reason }` | Jugador sale o se desconecta. |
| `LAP_COMPLETED` | Sí | `{ lapNumber, lapTimeMs, position, avgSpeedKmh }` | Vuelta completada. |
| `POSITION_CHANGED` | Sí | `{ fromPosition, toPosition, lapNumber }` | Cambio de posición en carrera. |
| `PENALTY_APPLIED` | Sí | `{ reason, timeMs }` | Penalización de tiempo. |
| `PIT_STOP` | Sí | `{ durationMs, lapNumber }` | Parada en pits. |
| `RACE_INCIDENT` | Sí | `{ incidentType, severity }` | Choque/salida de pista, etc. |
| `MATCH_FINISHED` | No | `{ finalStandings: [{playerId, finalPosition, totalTimeMs}] }` | Fin de la carrera. |

## 4. Relación eventos → partidas → jugadores

- Una **partida** (`matchId`) agrupa todos los eventos entre un `MATCH_STARTED`
  y su `MATCH_FINISHED` correspondiente.
- Un **jugador** (`playerId`) puede aparecer en varias partidas; dentro de una
  partida, se identifica por la combinación `(matchId, playerId)`.
- El módulo Go es responsable de reconstruir esta agrupación en memoria
  mientras la partida está activa.

## 5. Estadísticas definidas

**Por jugador** (agregada):
- Mejor vuelta (`bestLapTimeMs`).
- Tiempo promedio de vuelta (`avgLapTimeMs`).
- Total de penalizaciones y tiempo penalizado.
- Posición final.

**Por partida** (agregada):
- Vuelta más rápida de la carrera (entre todos los jugadores).
- Promedio de tiempo de vuelta de todos los jugadores.

**Combinando tipos de evento** (`LAP_COMPLETED` + `POSITION_CHANGED`):
- Evolución de posición por vuelta de cada jugador (curva de posición vs. vuelta).

## 6. Regla basada en secuencia de eventos: Remontada ("Comeback")

No se detecta con un solo evento; requiere reconstruir la secuencia de
`POSITION_CHANGED` (y `LAP_COMPLETED` como respaldo de posición) de un jugador
durante toda la partida:

1. Se calcula la **peor posición** alcanzada por el jugador en cualquier punto
   de la carrera (`worstPosition`).
2. Se compara contra la **posición final** (`MATCH_FINISHED.finalStandings`).
3. Si `worstPosition - finalPosition >= UMBRAL` (por defecto `UMBRAL = 3`),
   se marca `ComebackDetected(playerId, worstPosition, finalPosition, gain)`.

Esto obliga a Scala a recorrer y relacionar una secuencia de eventos, no
solamente contar ocurrencias.

## 7. Formato de intercambio Go → Scala

Cuando Go recibe `MATCH_FINISHED`, empaqueta **todos los eventos de esa
partida** (en orden de `timestamp`) y hace `POST /analyze` al módulo Scala
con:

```json
{ "matchId": "match-42", "events": [ ...eventos... ] }
```

Scala responde con un JSON de resultados (ver `Main.scala`) que incluye
estadísticas por jugador, por partida y las remontadas detectadas.
