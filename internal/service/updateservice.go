package service

import (
	"fmt"
	"log"
	"runtime"
	"sync"

	"updater/internal/clients"
	"updater/internal/config"
	"updater/pkg/models"

	"github.com/Masterminds/semver/v3"
)

type UpdateService struct {
	Config         *config.Config
	GitHubClient   clients.GitHubClientInterface
	BinaryManager  *BinaryManager
	ProcessManager *ProcessManager
	VersionManager *VersionManager
	mux            *sync.RWMutex
}

func NewUpdateService(config *config.Config, githubClient clients.GitHubClientInterface, mux *sync.RWMutex) *UpdateService {
	binaryManager := NewBinaryManager(config)
	processManager := NewProcessManager()
	versionManager := NewVersionManager(config, mux)

	service := &UpdateService{
		Config:         config,
		GitHubClient:   githubClient,
		BinaryManager:  binaryManager,
		ProcessManager: processManager,
		VersionManager: versionManager,
		mux:            mux,
	}

	// Set up rollback callback
	versionManager.SetRollbackCallback(func(oldVersion, newVersion *semver.Version) {
		log.Printf("Rolling back from version %s to %s", oldVersion, newVersion)
	})

	return service
}

func (s *UpdateService) downloadRelease(release *models.GitHubRelease, newVersion *semver.Version) (string, error) {
	assetURL := release.GetAssetURL(runtime.GOOS, runtime.GOARCH)
	if assetURL == "" {
		return "", fmt.Errorf("no matching asset found")
	}

	return s.BinaryManager.DownloadAndExtract(assetURL, newVersion)
}

func (s *UpdateService) CheckForUpdates() error {
	release, newVersion, err := s.GitHubClient.FetchLatestRelease()
	if err != nil {
		return err
	}

	if !s.VersionManager.ShouldUpdate(newVersion) {
		return nil
	}

	binPath, err := s.downloadRelease(release, newVersion)
	if err != nil {
		return err
	}

	port := s.BinaryManager.FindFreePort()
	cmd, err := s.BinaryManager.StartBinary(binPath, port)
	if err != nil {
		return err
	}

	if !s.BinaryManager.VerifyBinary(port, newVersion.String()) {
		log.Printf("Verification failed for version %s on port %d", newVersion, port)
		cmd.Process.Kill()
		return fmt.Errorf("verification failed")
	}

	newInstance := &models.BinaryInstance{
		Version: newVersion,
		Port:    port,
		Cmd:     cmd,
	}

	oldInstance := s.VersionManager.GetActive()
	s.VersionManager.SetActive(newInstance)

	if oldInstance != nil {
		go s.ProcessManager.GracefulShutdown(oldInstance)
	}

	s.ProcessManager.MonitorProcess(newInstance, func() {
		if s.VersionManager.GetActive() == newInstance {
			s.handleProcessExit(newInstance)
		}
	})

	return nil
}

func (s *UpdateService) handleProcessExit(instance *models.BinaryInstance) {
	lastInstance := s.VersionManager.Rollback()
	if lastInstance != nil {
		port := s.BinaryManager.FindFreePort()
		path := fmt.Sprintf("bin/%s-v%s/%s", s.Config.BinaryPrefix, lastInstance.Version, s.Config.BinaryPrefix)
		cmd, err := s.BinaryManager.StartBinary(path, port)
		if err == nil && s.BinaryManager.VerifyBinary(port, lastInstance.Version.String()) {
			log.Printf("Rolled back to version %s on port %d", lastInstance.Version, port)
			lastInstance.Port = port
			lastInstance.Cmd = cmd
			s.VersionManager.SetActive(lastInstance)
			s.ProcessManager.MonitorProcess(lastInstance, nil)
		}
	}
}
