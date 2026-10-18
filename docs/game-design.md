# Diseño del juego, los eventos y las estadísticas

## 1. Juego elegido: carreras

Se eligió un juego de **carreras por circuito** (vueltas, posiciones,
penalizaciones, paradas en pits e incidentes). Razones:

- Produce **flujos de eventos ricos y de naturaleza temporal** (decenas de
  eventos por jugador), lo que permite análisis reales y no solo contar.
- Tiene una **secuencia con significado** (la evolución de la posición durante
  la carrera) que permite definir una regla que depende de varios eventos.
- Las métricas son intuitivas (mejor vuelta, consistencia, remontadas), fáciles
  de verificar a mano con datos de prueba y de explicar en una demostración.
- Admite jugadores que **se desconectan** y partidas que se solapan, casos
  útiles para probar el procesamiento concurrente y los casos especiales.

El catálogo completo de eventos, con sus campos, está en
[`event-spec.md`](event-spec.md).

## 2. Por qué estos eventos

| Evento | Qué información aporta | Para qué se usa |
|---|---|---|
| `MATCH_STARTED` / `MATCH_FINISHED` | Inicio y fin de la partida, clasificación final | Delimitan la partida; Go sabe cuándo enviarla; la clasificación alimenta la posición final |
| `PLAYER_JOINED` / `PLAYER_LEFT` / `PLAYER_DISCONNECTED` | Quién participa y quién abandona | Casos especiales (un jugador sin posición final) |
| `LAP_COMPLETED` | vuelta, tiempo, posición, velocidad | Mejor vuelta, promedio, posición por vuelta |
| `POSITION_CHANGED` | de qué posición a cuál, en qué vuelta | Reconstruir la secuencia de posiciones (remontada) |
| `PENALTY_APPLIED` | motivo y tiempo | Total y cantidad de penalizaciones |
| `PIT_STOP` | duración y vuelta | Cantidad y tiempo en pits |
| `RACE_INCIDENT` | tipo y severidad | Incidentes por jugador |

Un evento aislado casi nunca dice algo útil (una vuelta de 91 s no es buena ni
mala por sí sola); el valor aparece al **agrupar y relacionar** eventos por
jugador y por partida, que es lo que hace el módulo Scala.

## 3. Relación entre eventos, partidas y jugadores

- `matchId` identifica la partida; todos sus eventos viajan juntos a Scala.
- `playerId` identifica al jugador dentro de la partida; los eventos globales
  (`MATCH_STARTED`, `MATCH_FINISHED`) no lo llevan.
- `gameId` y `gameVersion` identifican qué juego produjo el evento.

## 4. Cómo incorporar otro juego (extensibilidad)

El diseño separa lo que es común de lo que es propio de cada juego:

| Capa | ¿Depende del juego? |
|---|---|
| Sobre del evento (`eventId`, `timestamp`, `gameId`, `matchId`, `playerId`, `type`, `data`) | No |
| Validación de campos comunes, agrupación por partida, concurrencia, envío a Scala, persistencia y consulta | No |
| Tipos de evento y contenido de `data` | Sí |
| Estadísticas y reglas (Scala) | Sí |

Para sumar, por ejemplo, **ajedrez**:

1. Definir sus eventos en la especificación (p. ej. `MOVE` con `{from, to,
   notation, timeMs}` y `CAPTURE`), usando `gameId: "chess"`.
2. Agregar los tipos que exigen `playerId` en el mapa `playerRequired`
   (`internal/validate/validator.go`) — un cambio de una línea por tipo.
3. Crear en Scala un objeto de análisis del juego (por ejemplo
   `ChessStatistics`, con tiempo medio por jugada o capturas por jugador) con
   las mismas operaciones funcionales, y elegirlo según `gameId` en
   `Main.scala`.
4. Agregar escenarios en `data/` y pruebas, como se hizo para carreras.

No hace falta cambiar la comunicación entre módulos ni el almacenamiento: los
resultados ya se guardan como JSON por `matchId`, sea cual sea el juego.

Otros ejemplos que encajan con el mismo sobre: cartas (`CARD_PLAYED` →
efectividad por carta), combate (`PLAYER_ELIMINATED` → K/D) o estrategia
(`OBJECTIVE_COMPLETED` → eficiencia de recursos).
