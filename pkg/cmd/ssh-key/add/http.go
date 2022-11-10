package add

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	"github.com/ungtb10d/cli/v2/api"
	"github.com/ungtb10d/cli/v2/internal/ghinstance"
)

func SSHKeyUpload(httpClient *http.Client, hostname string, keyFile io.Reader, title string) error {
	url := ghinstance.RESTPrefix(hostname) + "user/keys"

	keyBytes, err := io.ReadAll(keyFile)
	if err != nil {
		return err
	}

	payload := map[string]string{
		"title": title,
		"key":   string(keyBytes),
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(payloadBytes))
	if err != nil {
		return err
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 299 {
		return api.HandleHTTPError(resp)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		return err
	}

	return nil
}
