# Estadísticas y reglas implementadas

Todas se calculan en Scala (`analysis/Statistics.scala` y `analysis/Rules.scala`)
sobre la lista de eventos de **una partida** y están cubiertas por
`AnalysisSuite` (munit).

## 1. Estadísticas por jugador (`playerStats`)

| Campo | Definición | Eventos que usa |
|---|---|---|
| `bestLapMs` | mínimo de `lapTimeMs` del jugador | `LAP_COMPLETED` |
| `avgLapMs` | suma de `lapTimeMs` ÷ cantidad de vueltas (`foldLeft`) | `LAP_COMPLETED` |
| `penaltyCount`, `totalPenaltyMs` | cantidad y suma de `timeMs` | `PENALTY_APPLIED` |
| `pitStopCount`, `totalPitMs` | cantidad y suma de `durationMs` | `PIT_STOP` |
| `incidentCount` | cantidad de incidentes | `RACE_INCIDENT` |
| `finalPosition` | posición en `finalStandings`; `null` si el jugador no figura (p. ej. se desconectó) | `MATCH_FINISHED` |

Los campos que no se pueden calcular (por ejemplo `bestLapMs` si el jugador no
completó vueltas) se devuelven como `null`; nunca se inventan valores.

## 2. Estadísticas por partida (`matchStats`)

| Campo | Definición |
|---|---|
| `fastestLapMs` / `fastestLapPlayer` | la vuelta más rápida entre **todos** los jugadores y quién la hizo |
| `avgLapMsAllPlayers` | promedio de todas las vueltas de la partida |
| `playerStats` | la lista anterior, una entrada por jugador |

## 3. Estadística que combina tipos de evento

`positionEvolution` une `LAP_COMPLETED` (posición al cerrar cada vuelta) con
`POSITION_CHANGED` (cambios dentro de la vuelta) por jugador y devuelve la
curva `(vuelta, posición)`. Está implementada y probada; todavía no se incluye
en el JSON de `/analyze` (ver `architecture.md`, sección 7).

Además, `playerStats` ya **relaciona tipos distintos** (vueltas, penalizaciones,
pits, incidentes y clasificación final) en una sola entrada por jugador.

## 4. Regla basada en una secuencia: remontada

Requiere recorrer la **secuencia completa** de posiciones de un jugador, no un
evento aislado:

1. Se toman, por jugador, todas las posiciones que ocupó durante la carrera:
   `position` de cada `LAP_COMPLETED` y `toPosition` de cada `POSITION_CHANGED`.
2. `worstPosition` = la mayor (peor) de esas posiciones.
3. `finalPosition` = la de `MATCH_FINISHED`.
4. Hay remontada si `worstPosition − finalPosition ≥ 3` (`ComebackThreshold`).
5. Se devuelven ordenadas por posiciones ganadas, de mayor a menor.

Un jugador sin posición final (desconectado) nunca genera remontada.

## 5. Ejemplo completo (`data/scenario-comeback.json`)

Cuatro jugadores, tres vueltas. `p4` sale cuarto, hace una parada en pits en la
vuelta 1, remonta en la vuelta 2 y termina primero; `p2` recibe 5 s de
penalización y `p3` tiene un incidente.

```json
{
  "matchStats": {
    "matchId": "match-comeback",
    "fastestLapMs": 89800,
    "fastestLapPlayer": "p4",
    "avgLapMsAllPlayers": 92075.0,
    "playerStats": [
      { "playerId": "p1", "bestLapMs": 91500, "avgLapMs": 91766.67, "totalPenaltyMs": 0,    "penaltyCount": 0, "finalPosition": 2, "pitStopCount": 0, "totalPitMs": 0,    "incidentCount": 0 },
      { "playerId": "p2", "bestLapMs": 92000, "avgLapMs": 92233.33, "totalPenaltyMs": 5000, "penaltyCount": 1, "finalPosition": 3, "pitStopCount": 0, "totalPitMs": 0,    "incidentCount": 0 },
      { "playerId": "p3", "bestLapMs": 92900, "avgLapMs": 93000.0,  "totalPenaltyMs": 0,    "penaltyCount": 0, "finalPosition": 4, "pitStopCount": 0, "totalPitMs": 0,    "incidentCount": 1 },
      { "playerId": "p4", "bestLapMs": 89800, "avgLapMs": 91300.0,  "totalPenaltyMs": 0,    "penaltyCount": 0, "finalPosition": 1, "pitStopCount": 1, "totalPitMs": 2400, "incidentCount": 0 }
    ]
  },
  "comebacks": [
    { "playerId": "p4", "worstPosition": 4, "finalPosition": 1, "positionsGained": 3 }
  ]
}
```

(Los promedios se muestran redondeados; la API devuelve todos los decimales.)

## 6. Escenarios de prueba y qué demuestra cada uno

| Archivo | Partida | Qué demuestra |
|---|---|---|
| `data/sample-events.json` | `match-demo` | caso mínimo: 2 jugadores, sin remontada |
| `data/scenario-comeback.json` | `match-comeback` | remontada de 3 posiciones, penalización, pit stop e incidente |
| `data/scenario-no-comeback.json` | `match-clean` | adelantamientos entre jugadores que **no** alcanzan el umbral |
| `data/scenario-disconnect.json` | `match-disconnect` | jugador desconectado: aparece en estadísticas con `finalPosition: null` |
| `data/scenario-invalid-events.json` | — | 6 eventos inválidos que Go debe rechazar (falta cada campo obligatorio) |

`go run ./cmd/smoketest` (en `go-ingestion/`) envía todos estos escenarios y
comprueba los valores anteriores automáticamente. El simulador
(`cmd/simulator`) genera además datos aleatorios con tres partidas en paralelo.
