package infra

import (
	"net/http"
	"time"
)

type HTTPClient struct {
	http    *http.Client
	BaseURL string
}

func NewHTTPClient(httpClient *http.Client, baseURL string) *HTTPClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 5 * time.Second}
	}
	return &HTTPClient{http: httpClient, BaseURL: baseURL}
}
