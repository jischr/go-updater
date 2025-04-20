package models

import (
	"os/exec"
	"strings"

	"github.com/Masterminds/semver/v3"
)

type GitHubReleaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type GitHubRelease struct {
	TagName string               `json:"tag_name"`
	Assets  []GitHubReleaseAsset `json:"assets"`
}

func (r *GitHubRelease) GetAssetURL(os, arch string) string {
	for _, asset := range r.Assets {
		assetName := strings.ToLower(asset.Name)
		if strings.Contains(assetName, os) && strings.Contains(assetName, arch) {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func (r *GitHubRelease) CleanedVersion() (*semver.Version, error) {
	tag := strings.TrimPrefix(r.TagName, "v")
	newVersion, err := semver.NewVersion(tag)
	if err != nil {
		return nil, err
	}
	return newVersion, nil
}

type BinaryInstance struct {
	Version *semver.Version
	Port    int
	Cmd     *exec.Cmd
}
