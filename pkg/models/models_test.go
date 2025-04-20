package models_test

import (
	"testing"

	"updater/pkg/models"

	"github.com/stretchr/testify/assert"
)

func TestGitHubRelease_GetAssetURL(t *testing.T) {
	release := &models.GitHubRelease{
		TagName: "v1.2.3",
		Assets: []models.GitHubReleaseAsset{
			{
				Name:               "simple-server-Darwin-amd64",
				BrowserDownloadURL: "https://example.com/simple-server-Darwin-amd64",
			},
			{
				Name:               "simple-server-darwin-arm64",
				BrowserDownloadURL: "https://example.com/simple-server-darwin-arm64",
			},
		},
	}

	assert.Equal(t, "https://example.com/simple-server-Darwin-amd64", release.GetAssetURL("darwin", "amd64"))
	assert.Equal(t, "https://example.com/simple-server-darwin-arm64", release.GetAssetURL("darwin", "arm64"))
	assert.Equal(t, "", release.GetAssetURL("linux", "amd64"))
}
