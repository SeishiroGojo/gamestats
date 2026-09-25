package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"gamestats/ingestion/internal/model"
)

// ScalaClient encapsula la comunicación Go -> Scala.
// Mecanismo elegido: HTTP/REST con JSON (ver docs/event-spec.md, sección 7).
type ScalaClient struct {
	baseURL string
	http    *http.Client
}

func NewScalaClient(baseURL string) *ScalaClient {
	return &ScalaClient{
		baseURL: baseURL,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// SendMatch envía el lote completo de eventos de una partida terminada
// al endpoint de análisis de Scala (POST /analyze), y devuelve el JSON de
// resultado que Scala calculó, para que quien reciba la llamada pueda
// mostrarlo (loguearlo, guardarlo, exponerlo, etc.).
func (c *ScalaClient) SendMatch(batch model.MatchBatch) (string, error) {
	body, err := json.Marshal(batch)
	if err != nil {
		return "", fmt.Errorf("serializando el lote: %w", err)
	}

	resp, err := c.http.Post(c.baseURL+"/analyze", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("llamando a Scala: %w", err)
	}
	defer resp.Body.Close()

	respBody, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return "", fmt.Errorf("leyendo respuesta de Scala: %w", readErr)
	}

	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("Scala respondió %d para la partida %s: %s", resp.StatusCode, batch.MatchID, string(respBody))
	}
	return string(respBody), nil
}
