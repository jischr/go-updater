package clients

import (
	"encoding/json"
	"fmt"
	"net/http"

	"updater/internal/config"
	"updater/pkg/models"

	"github.com/Masterminds/semver/v3"
)

type GitHubClient struct {
	httpClient   *http.Client
	githubApiUrl string
	repoOwner    string
	repoName     string
}

type GitHubClientInterface interface {
	FetchLatestRelease() (*models.GitHubRelease, *semver.Version, error)
}

func NewGithubClientWithClient(config *config.Config, httpClient *http.Client) GitHubClientInterface {
	return &GitHubClient{
		httpClient:   httpClient,
		githubApiUrl: config.GithubAPIURL,
		repoOwner:    config.RepoOwner,
		repoName:     config.RepoName,
	}
}

func NewGitHubClient(config *config.Config) GitHubClientInterface {
	return NewGithubClientWithClient(config, &http.Client{})
}

func (c *GitHubClient) FetchLatestRelease() (*models.GitHubRelease, *semver.Version, error) {
	url := fmt.Sprintf(c.githubApiUrl, c.repoOwner, c.repoName)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	var release models.GitHubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	if err != nil {
		return nil, nil, err
	}

	newVersion, err := release.CleanedVersion()
	if err != nil {
		return nil, nil, err
	}

	return &release, newVersion, err
}
