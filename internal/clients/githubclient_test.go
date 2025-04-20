package clients_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"updater/internal/clients"
	"updater/internal/config"
	"updater/pkg/models"

	"github.com/stretchr/testify/require"
)

func TestGitHubClient_FetchLatestRelease(t *testing.T) {
	config := config.DefaultConfig()

	githubRelease := &models.GitHubRelease{
		TagName: "v1.0.0",
		Assets: []models.GitHubReleaseAsset{
			{
				Name:               "simple-server-Darwin-amd64",
				BrowserDownloadURL: "https://example.com/simple-server-Darwin-amd64",
			},
		},
	}
	json, err := json.Marshal(githubRelease)
	require.NoError(t, err)

	mock := &MockRoundTripper{
		RoundTripFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(string(json))),
				Header:     make(http.Header),
			}, nil
		},
	}
	client := &http.Client{Transport: mock}
	githubClient := clients.NewGithubClientWithClient(config, client)

	release, newVersion, err := githubClient.FetchLatestRelease()
	require.NoError(t, err)
	require.Equal(t, githubRelease, release)
	require.Equal(t, "1.0.0", newVersion.String())
}

type MockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

func (m *MockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}
