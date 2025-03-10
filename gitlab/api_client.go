package gitlab

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"gitlab-mr-combiner/config"
)

type ApiClient struct {
	client  *http.Client
	baseURL string
	token   string
}

func NewApiClient() *ApiClient {
	return &ApiClient{
		client:  &http.Client{},
		baseURL: fmt.Sprintf("%s/api/v4", config.GitlabURL),
		token:   config.GitlabToken,
	}
}

func (api *ApiClient) Send(method, endpoint string, body interface{}) ([]byte, error) {
	url := fmt.Sprintf("%s%s", api.baseURL, endpoint)

	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		reqBody = bytes.NewBuffer(jsonData)
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", api.token))

	resp, err := api.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= 400 {
		return nil, errors.New(string(data))
	}

	return data, nil
}
