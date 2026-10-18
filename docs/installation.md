# Instalación y ejecución

## 1. Requisitos

| Para... | Se necesita |
|---|---|
| Ejecutar el sistema completo | **Docker Desktop** (incluye Docker Compose) |
| Correr los tests y la prueba de humo en Go | **Go 1.22 o superior** (<https://go.dev/dl>) |
| Correr los tests de Scala sin Docker | **JDK 21** y **sbt** (opcional; también se pueden correr con Docker, ver 4.2) |
| Trabajar el código | VS Code con las extensiones *Go*, *Scala (Metals)* y *Docker* |

## 2. Obtener el código

```powershell
git clone https://github.com/SeishiroGojo/gamestats.git
cd gamestats
```

## 3. Ejecutar el sistema

```powershell
docker compose up --build
```

La primera vez tarda varios minutos (descarga imágenes base y dependencias de
sbt). Está listo cuando aparecen estas líneas:

```
scala-analysis-1  | Base de datos lista en /app/data/gamestats.db
scala-analysis-1  | GameStats analysis escuchando en :8081
go-ingestion-1    | ... GameStats ingestion escuchando en :8080 (Scala en http://scala-analysis:8081)
```

| Servicio | URL |
|---|---|
| Ingestión (Go) | <http://localhost:8080> |
| Análisis (Scala) | <http://localhost:8081> |

Para detenerlo: `Ctrl+C`; para borrar también los datos guardados:
`docker compose down -v`.

> El aviso `the attribute 'version' is obsolete` de Docker Compose es
> informativo y no afecta el funcionamiento.

## 4. Verificar que todo funciona

En otra terminal, en la carpeta del proyecto:

### 4.1 Prueba de humo automática (recomendada)

```powershell
cd go-ingestion
go run ./cmd/smoketest -data ../data
```

Envía los escenarios de `data/`, espera los resultados y verifica los valores.
Termina con `RESULTADO: todo correcto` (código de salida 0).

### 4.2 Tests unitarios

```powershell
# Go (con detector de condiciones de carrera)
cd go-ingestion
go test -race ./...

# Scala (requiere sbt)
cd ../scala-analysis
sbt test

# Scala sin instalar sbt, usando la misma imagen del Dockerfile
cd ..
docker run --rm -v "${PWD}/scala-analysis:/app" -w /app sbtscala/scala-sbt:eclipse-temurin-jammy-21.0.2_13_1.10.1_2.13.14 sbt test
```

### 4.3 Generar datos aleatorios

```powershell
cd go-ingestion
go run ./cmd/simulator
```

Crea 3 partidas de carreras en paralelo (4 jugadores, 5 vueltas) y las envía a
`http://localhost:8080`. Se puede cambiar el destino con la variable
`INGESTION_URL`.

## 5. Ejecutar sin Docker (desarrollo)

Requiere Go, JDK 21 y sbt. En tres terminales:

```powershell
# 1) Scala (puerto 8081). DB_PATH define dónde se guarda la base.
cd scala-analysis
$env:DB_PATH = "data/gamestats.db"
sbt run

# 2) Go (puerto 8080)
cd go-ingestion
$env:SCALA_ANALYSIS_URL = "http://localhost:8081"
go run ./cmd/server
```

## 6. Variables de configuración

| Módulo | Variable | Por defecto | Uso |
|---|---|---|---|
| Go | `PORT` | `8080` | puerto del servidor |
| Go | `SCALA_ANALYSIS_URL` | `http://localhost:8081` | dónde está Scala |
| Scala | `PORT` | `8081` | puerto del servidor |
| Scala | `DB_PATH` | `data/gamestats.db` | archivo SQLite |
| Simulador / smoketest | `INGESTION_URL` / flags `-ingestion`, `-analysis`, `-data` | `localhost` | destino de las pruebas |

## 7. Solución de problemas

| Síntoma | Causa y solución |
|---|---|
| `cannot connect to the Docker daemon` | Docker Desktop no está abierto: iniciarlo y esperar a que termine de arrancar. |
| El build de Scala falla con `ClassNotFoundException: org.sqlite.JDBC` | Falta la dependencia `sqlite-jdbc` en `scala-analysis/build.sbt`. |
| En PowerShell `curl` da error de parámetros | `curl` es un alias de `Invoke-WebRequest`; usar `Invoke-RestMethod` o `curl.exe`. |
| Dos comandos pegados en una sola línea | Ejecutarlos por separado (Enter entre uno y otro) o separarlos con `;`. |
| El puerto 8080 u 8081 está ocupado | Cerrar el programa que lo usa o cambiar el mapeo de puertos en `docker-compose.yml`. |
| `GET /results` está vacío | Aún no se envió una partida completa (con `MATCH_FINISHED`) o Go no logró contactar a Scala: revisar `docker compose logs`. |
