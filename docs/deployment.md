# Despliegue

## 1. Entorno de despliegue

**Docker Compose sobre una máquina con Docker** (un servidor/VM del equipo o,
en su defecto, la computadora del equipo con Docker Desktop), ejecutando las
**imágenes publicadas por el CI en GitHub Container Registry (GHCR)**:

- `ghcr.io/<usuario>/gamestats-ingestion` (Go)
- `ghcr.io/<usuario>/gamestats-analysis` (Scala)

Justificación: es reproducible (el mismo artefacto que pasó el CI es el que se
ejecuta), no requiere compilar en el servidor, no depende de un proveedor de
nube y se instala con un solo archivo (`docker-compose.deploy.yml`).

| Archivo | Uso |
|---|---|
| `docker-compose.yml` | **Desarrollo**: construye las imágenes desde el código fuente |
| `docker-compose.deploy.yml` | **Despliegue**: descarga las imágenes ya publicadas, reinicia solo si se cae (`restart: unless-stopped`) y publica el puerto 8081 únicamente en `127.0.0.1` |

## 2. Publicar las imágenes

1. Hacer *push* a `main` (o crear una etiqueta `vX.Y.Z`).
2. Esperar a que el workflow **GameStats CI** termine en verde
   (pestaña **Actions** del repositorio). Los jobs `e2e` y `publish` son los que
   validan y publican.
3. La primera vez, hacer públicos los paquetes (ver `ci-cd.md`, “Configuración
   necesaria en GitHub”). Las imágenes aparecen en la pestaña **Packages** del
   perfil o del repositorio.

## 3. Desplegar en una máquina con Docker

Solo se necesita Docker y el archivo `docker-compose.deploy.yml`.

```bash
mkdir gamestats && cd gamestats
# copiar aquí docker-compose.deploy.yml (git clone, scp o descarga manual)

# Dueño de las imágenes, EN MINÚSCULAS (usuario de GitHub):
echo "GHCR_OWNER=seishirogojo" > .env

docker compose -f docker-compose.deploy.yml pull
docker compose -f docker-compose.deploy.yml up -d
docker compose -f docker-compose.deploy.yml ps
```

En PowerShell, para crear el `.env`:
`Set-Content -Path .env -Value "GHCR_OWNER=seishirogojo"`.

### Verificación del despliegue

```bash
curl http://localhost:8080/health          # {"status":"ok","activeMatches":0}
curl http://localhost:8081/health          # {"status":"ok"} (solo desde la propia máquina)
```

Y la prueba de punta a punta, ejecutada en la propia máquina del despliegue
(requiere el código del repositorio y Go instalado):

```powershell
cd go-ingestion
go run ./cmd/smoketest -data ../data
```

Si se quiere consultar resultados desde otra máquina, definir en `.env`
`ANALYSIS_BIND=0.0.0.0` (y proteger el puerto con un firewall, pues la API no
tiene autenticación).

### Variables

| Variable | Por defecto | Descripción |
|---|---|---|
| `GHCR_OWNER` | `seishirogojo` | dueño de las imágenes en GHCR (minúsculas) |
| `GAMESTATS_TAG` | `latest` | versión a ejecutar (`latest`, `1.0.0`, `sha-abc1234`) |
| `ANALYSIS_BIND` | `127.0.0.1` | interfaz en la que se publica el puerto 8081 |

## 4. Despliegue automático por SSH (opcional)

El job `deploy` del CI actualiza un servidor propio después de cada publicación
en `main`. Está **desactivado** hasta que se configure:

1. En el servidor: instalar Docker y crear un usuario con permiso para usarlo.
2. En GitHub → **Settings → Secrets and variables → Actions**:
   - *Variable* `DEPLOY_ENABLED` = `true`
   - *Secrets* `DEPLOY_HOST` (IP o nombre), `DEPLOY_USER`, `DEPLOY_PATH`
     (carpeta en el servidor, p. ej. `/opt/gamestats`) y `DEPLOY_SSH_KEY`
     (clave privada de un par creado solo para esto).
3. En **Settings → Environments** se puede crear el entorno `production` para
   exigir aprobación manual antes de cada despliegue.

El job copia `docker-compose.deploy.yml`, hace `pull` y `up -d`, y espera hasta
que `http://localhost:8080/health` responda en el servidor.

## 5. Actualizar, volver atrás y operar

```bash
# Actualizar a lo último publicado
docker compose -f docker-compose.deploy.yml pull && docker compose -f docker-compose.deploy.yml up -d

# Volver a una versión anterior (cualquier etiqueta publicada)
GAMESTATS_TAG=sha-abc1234 docker compose -f docker-compose.deploy.yml up -d

# Ver logs
docker compose -f docker-compose.deploy.yml logs -f

# Detener (conserva los datos) / detener y borrar los datos
docker compose -f docker-compose.deploy.yml down
docker compose -f docker-compose.deploy.yml down -v
```

### Respaldo de la base de datos

Los resultados están en el volumen `gamestats_scala-data`:

```bash
docker run --rm -v gamestats_scala-data:/data -v "$PWD":/backup alpine \
  tar czf /backup/gamestats-data.tgz -C /data .
```

## 6. Evidencia para la entrega

- Pestaña **Actions** con el workflow en verde (incluye `e2e` y `publish`).
- Pestaña **Packages** con las dos imágenes publicadas.
- Salida de `docker compose -f docker-compose.deploy.yml ps` con ambos
  servicios en estado *healthy*.
- Salida de `go run ./cmd/smoketest` terminando en `RESULTADO: todo correcto`.
