// Prueba de humo (smoke test) de punta a punta de GameStats.
//
// Con los dos servicios levantados (docker compose up), envía los escenarios
// de la carpeta data/ al módulo Go y comprueba, consultando al módulo Scala,
// que los resultados son los esperados (ver docs/demo-guide.md):
//
//	go run ./cmd/smoketest                      # servicios en localhost
//	go run ./cmd/smoketest -data ../data        # carpeta de escenarios
//
// Sale con código 0 si todo coincide y con 1 si algo falla, por lo que sirve
// tanto para la demostración como para el pipeline de CI.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type verificador struct {
	fallos int
}

func (v *verificador) ok(msg string) { fmt.Printf("  [OK]    %s\n", msg) }
func (v *verificador) fallo(format string, args ...interface{}) {
	v.fallos++
	fmt.Printf("  [FALLO] %s\n", fmt.Sprintf(format, args...))
}

// igual compara dos valores (como texto) e informa el resultado.
func (v *verificador) igual(descripcion string, obtenido, esperado interface{}) {
	if fmt.Sprint(obtenido) == fmt.Sprint(esperado) {
		v.ok(fmt.Sprintf("%s = %v", descripcion, obtenido))
		return
	}
	v.fallo("%s: se esperaba %v y se obtuvo %v", descripcion, esperado, obtenido)
}

func main() {
	ingestion := flag.String("ingestion", "http://localhost:8080", "URL del módulo Go")
	analysis := flag.String("analysis", "http://localhost:8081", "URL del módulo Scala")
	dataDir := flag.String("data", "../data", "carpeta con los escenarios JSON")
	espera := flag.Duration("timeout", 60*time.Second, "tiempo máximo de espera por cada paso")
	flag.Parse()

	v := &verificador{}
	cliente := &http.Client{Timeout: 10 * time.Second}

	fmt.Println("== 1. Servicios disponibles")
	for nombre, url := range map[string]string{"go-ingestion": *ingestion, "scala-analysis": *analysis} {
		if err := esperarOK(cliente, url+"/health", *espera); err != nil {
			v.fallo("%s no responde en %s: %v", nombre, url, err)
			os.Exit(1)
		}
		v.ok(nombre + " responde en /health")
	}

	fmt.Println("== 2. Eventos inválidos (Go debe rechazarlos)")
	codigo, cuerpo := publicar(cliente, *ingestion, filepath.Join(*dataDir, "scenario-invalid-events.json"), v)
	var rechazo struct {
		Accepted int      `json:"accepted"`
		Rejected []string `json:"rejected"`
	}
	json.Unmarshal(cuerpo, &rechazo)
	v.igual("código HTTP", codigo, http.StatusMultiStatus)
	v.igual("eventos aceptados", rechazo.Accepted, 0)
	v.igual("eventos rechazados", len(rechazo.Rejected), 6)

	fmt.Println("== 3. Escenarios válidos (Go agrupa, Scala analiza y guarda)")
	escenarios := []struct{ archivo, matchID string }{
		{"sample-events.json", "match-demo"},
		{"scenario-comeback.json", "match-comeback"},
		{"scenario-no-comeback.json", "match-clean"},
		{"scenario-disconnect.json", "match-disconnect"},
	}
	for _, e := range escenarios {
		codigo, _ := publicar(cliente, *ingestion, filepath.Join(*dataDir, e.archivo), v)
		v.igual(e.archivo+" -> código HTTP", codigo, http.StatusAccepted)
	}

	resultados := map[string]map[string]interface{}{}
	for _, e := range escenarios {
		r, err := esperarResultado(cliente, *analysis, e.matchID, *espera)
		if err != nil {
			v.fallo("no apareció el resultado de %s en Scala: %v", e.matchID, err)
			continue
		}
		resultados[e.matchID] = r
		v.ok("resultado guardado y consultable: GET /results/" + e.matchID)
	}

	fmt.Println("== 4. Contenido de los resultados")
	if r, ok := resultados["match-demo"]; ok {
		ms := objeto(r, "matchStats")
		v.igual("match-demo: vuelta más rápida (ms)", ms["fastestLapMs"], 91000)
		v.igual("match-demo: piloto de la vuelta más rápida", ms["fastestLapPlayer"], "p1")
		v.igual("match-demo: remontadas", len(lista(r, "comebacks")), 0)
	}
	if r, ok := resultados["match-comeback"]; ok {
		ms := objeto(r, "matchStats")
		v.igual("match-comeback: vuelta más rápida (ms)", ms["fastestLapMs"], 89800)
		v.igual("match-comeback: piloto de la vuelta más rápida", ms["fastestLapPlayer"], "p4")
		cb := lista(r, "comebacks")
		v.igual("match-comeback: cantidad de remontadas", len(cb), 1)
		if len(cb) == 1 {
			c := cb[0].(map[string]interface{})
			v.igual("match-comeback: jugador que remonta", c["playerId"], "p4")
			v.igual("match-comeback: peor posición", c["worstPosition"], 4)
			v.igual("match-comeback: posición final", c["finalPosition"], 1)
			v.igual("match-comeback: posiciones ganadas", c["positionsGained"], 3)
		}
		p2 := jugador(ms, "p2")
		v.igual("match-comeback: penalización de p2 (ms)", p2["totalPenaltyMs"], 5000)
		v.igual("match-comeback: pit stops de p4", jugador(ms, "p4")["pitStopCount"], 1)
		v.igual("match-comeback: incidentes de p3", jugador(ms, "p3")["incidentCount"], 1)
	}
	if r, ok := resultados["match-clean"]; ok {
		v.igual("match-clean: remontadas (ninguna alcanza el umbral)", len(lista(r, "comebacks")), 0)
	}
	if r, ok := resultados["match-disconnect"]; ok {
		ms := objeto(r, "matchStats")
		v.igual("match-disconnect: posición final de p1", jugador(ms, "p1")["finalPosition"], 1)
		v.igual("match-disconnect: p3 aparece en las estadísticas", jugador(ms, "p3")["playerId"], "p3")
		v.igual("match-disconnect: posición final de p3 (se desconectó)", jugador(ms, "p3")["finalPosition"], nil)
	}

	fmt.Println("== 5. Consulta de resultados")
	var listado []map[string]interface{}
	if err := getJSON(cliente, *analysis+"/results", &listado); err != nil {
		v.fallo("GET /results: %v", err)
	} else if len(listado) < len(escenarios) {
		v.fallo("GET /results devolvió %d partidas y se esperaban al menos %d", len(listado), len(escenarios))
	} else {
		v.ok(fmt.Sprintf("GET /results lista %d partidas", len(listado)))
	}
	if codigo := estado(cliente, *analysis+"/results/no-existe"); codigo != http.StatusNotFound {
		v.fallo("GET /results/no-existe debería responder 404 y respondió %d", codigo)
	} else {
		v.ok("GET /results/no-existe responde 404")
	}

	fmt.Println()
	if v.fallos > 0 {
		fmt.Printf("RESULTADO: %d verificación(es) fallaron\n", v.fallos)
		os.Exit(1)
	}
	fmt.Println("RESULTADO: todo correcto")
}

func publicar(c *http.Client, base, archivo string, v *verificador) (int, []byte) {
	raw, err := os.ReadFile(archivo)
	if err != nil {
		v.fallo("no se pudo leer %s: %v", archivo, err)
		return 0, nil
	}
	resp, err := c.Post(base+"/events", "application/json", bytes.NewReader(raw))
	if err != nil {
		v.fallo("POST /events falló: %v", err)
		return 0, nil
	}
	defer resp.Body.Close()
	cuerpo, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, cuerpo
}

func esperarOK(c *http.Client, url string, max time.Duration) error {
	limite := time.Now().Add(max)
	var ultimo error
	for time.Now().Before(limite) {
		resp, err := c.Get(url)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
			ultimo = fmt.Errorf("código %d", resp.StatusCode)
		} else {
			ultimo = err
		}
		time.Sleep(time.Second)
	}
	return ultimo
}

func esperarResultado(c *http.Client, analysis, matchID string, max time.Duration) (map[string]interface{}, error) {
	limite := time.Now().Add(max)
	var ultimo error
	for time.Now().Before(limite) {
		var r map[string]interface{}
		if err := getJSON(c, analysis+"/results/"+matchID, &r); err == nil {
			return r, nil
		} else {
			ultimo = err
		}
		time.Sleep(500 * time.Millisecond)
	}
	return nil, ultimo
}

func getJSON(c *http.Client, url string, destino interface{}) error {
	resp, err := c.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("código %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(destino)
}

func estado(c *http.Client, url string) int {
	resp, err := c.Get(url)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}

func objeto(m map[string]interface{}, clave string) map[string]interface{} {
	o, _ := m[clave].(map[string]interface{})
	return o
}

func lista(m map[string]interface{}, clave string) []interface{} {
	l, _ := m[clave].([]interface{})
	return l
}

// jugador busca las estadísticas de un jugador dentro de matchStats.
func jugador(matchStats map[string]interface{}, id string) map[string]interface{} {
	for _, p := range lista(matchStats, "playerStats") {
		if pm, ok := p.(map[string]interface{}); ok && pm["playerId"] == id {
			return pm
		}
	}
	return map[string]interface{}{}
}
