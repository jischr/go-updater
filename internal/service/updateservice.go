package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"updater/internal/clients"
	"updater/internal/config"
	"updater/pkg/models"

	"github.com/Masterminds/semver/v3"
)

type UpdateService struct {
	Config       *config.Config
	GitHubClient clients.GitHubClientInterface
	mux          *sync.RWMutex
	Active       *models.BinaryInstance
	previous     []*models.BinaryInstance
}

func NewUpdateService(config *config.Config, githubClient clients.GitHubClientInterface, mux *sync.RWMutex, active *models.BinaryInstance) *UpdateService {
	return &UpdateService{
		Config:       config,
		GitHubClient: githubClient,
		mux:          mux,
		Active:       active,
	}
}

func (s *UpdateService) downloadRelease(release *models.GitHubRelease, newVersion *semver.Version) (string, error) {
	assetURL := release.GetAssetURL(runtime.GOOS, runtime.GOARCH)
	if assetURL == "" {
		return "", fmt.Errorf("no matching asset found")
	}

	return downloadAndExtract(assetURL, newVersion, s.Config.BinaryPrefix)
}

func (s *UpdateService) CheckForUpdates() error {
	release, newVersion, err := s.GitHubClient.FetchLatestRelease()
	if err != nil {
		return err
	}

	// Check if the current version is nil or if the new version is greater than the current version
	// May want to remove this in case a version is removed and we need rollback
	s.mux.RLock()
	if s.Active != nil && !newVersion.GreaterThan(s.Active.Version) {
		s.mux.RUnlock()
		return nil
	}
	s.mux.RUnlock()

	binPath, err := s.downloadRelease(release, newVersion)
	if err != nil {
		return err
	}

	port := s.findFreePort()
	cmd, err := startBinary(binPath, port)
	if err != nil {
		return err
	}

	if !verifyBinary(port, newVersion.String()) {
		log.Printf("Verification failed for version %s on port %d", newVersion, port)
		cmd.Process.Kill()
		return fmt.Errorf("verification failed")
	}

	s.mux.Lock()
	old := s.Active
	s.Active = &models.BinaryInstance{Version: newVersion, Port: port, Cmd: cmd}
	if old != nil {
		s.previous = append(s.previous, old)
		go gracefulShutdown(old)
	}
	s.mux.Unlock()
	go s.monitorAndRollback(s.Active, old, s.Config)
	return nil
}

func downloadAndExtract(url string, version *semver.Version, binaryPrefix string) (string, error) {
	name := filepath.Base(url)
	archivePath := filepath.Join("bin", name)
	out, err := os.Create(archivePath)
	if err != nil {
		return "", err
	}
	defer out.Close()
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return "", err
	}

	extractDir := filepath.Join("bin", fmt.Sprintf("%s-v%s", binaryPrefix, version))
	os.MkdirAll(extractDir, 0755)

	var binPath string
	if strings.HasSuffix(name, ".tar.gz") {
		file, _ := os.Open(archivePath)
		defer file.Close()
		gzReader, _ := gzip.NewReader(file)
		tarReader := tar.NewReader(gzReader)
		for {
			hdr, err := tarReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				return "", err
			}
			target := filepath.Join(extractDir, hdr.Name)
			if hdr.FileInfo().IsDir() {
				os.MkdirAll(target, 0755)
				continue
			}
			os.MkdirAll(filepath.Dir(target), 0755)
			outFile, err := os.Create(target)
			if err != nil {
				return "", err
			}
			io.Copy(outFile, tarReader)
			outFile.Close()
			os.Chmod(target, 0755)
			if strings.Contains(hdr.Name, binaryPrefix) && hdr.FileInfo().Mode().IsRegular() && (hdr.FileInfo().Mode()&0111 != 0) {
				binPath = target
			}
		}
	} else if strings.HasSuffix(name, ".zip") {
		r, err := zip.OpenReader(archivePath)
		if err != nil {
			return "", err
		}
		defer r.Close()
		for _, f := range r.File {
			outPath := filepath.Join(extractDir, f.Name)
			if f.FileInfo().IsDir() {
				os.MkdirAll(outPath, 0755)
				continue
			}
			os.MkdirAll(filepath.Dir(outPath), 0755)
			rc, _ := f.Open()
			outFile, _ := os.Create(outPath)
			io.Copy(outFile, rc)
			rc.Close()
			outFile.Close()
			os.Chmod(outPath, 0755)
			if strings.Contains(f.Name, binaryPrefix) && (f.Mode()&0111 != 0) {
				binPath = outPath
			}
		}
	} else {
		return "", fmt.Errorf("unsupported archive format: %s", name)
	}

	if binPath == "" {
		return "", fmt.Errorf("no binary found in archive")
	}
	return binPath, nil
}

func startBinary(path string, port int) (*exec.Cmd, error) {
	log.Printf("Starting binary %s on port %d", path, port)
	cmd := exec.Command(path, strconv.Itoa(port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	return cmd, err
}

func verifyBinary(port int, expected string) bool {
	for i := 0; i < 5; i++ {
		time.Sleep(1 * time.Second)
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/version", port))
		if err != nil {
			log.Printf("Verification attempt %d failed: %v", i+1, err)
			continue
		}

		versionBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Printf("Error reading response body: %v", err)
			continue
		}

		// Convert the bytes to a string and compare
		version := string(versionBytes)
		if version == expected {
			log.Printf("Verified binary on port %d with version %s", port, version)
			return true
		}
	}
	log.Printf("Failed to verify binary on port %d", port)
	return false
}

func (s *UpdateService) findFreePort() int {
	start := s.Config.StartingPort + 1
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			ln.Close()
			return p
		}
	}
	panic("no free port found")
}

func gracefulShutdown(inst *models.BinaryInstance) {
	log.Printf("Shutting down version %s on port %d", inst.Version, inst.Port)
	inst.Cmd.Process.Signal(os.Interrupt)
	done := make(chan error, 1)
	go func() { done <- inst.Cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Printf("Force killing process for version %s", inst.Version)
		inst.Cmd.Process.Kill()
	}
}

func (s *UpdateService) monitorAndRollback(newInst, oldInst *models.BinaryInstance, config *config.Config) {
	err := newInst.Cmd.Wait()
	log.Printf("Process for %s exited: %v", newInst.Version, err)
	s.mux.Lock()
	if s.Active == newInst && len(s.previous) > 0 {
		last := s.previous[len(s.previous)-1]
		s.previous = s.previous[:len(s.previous)-1]
		port := s.findFreePort()
		path := filepath.Join("bin", fmt.Sprintf("%s-v%s", config.BinaryPrefix, last.Version))
		files, _ := os.ReadDir(path)
		for _, f := range files {
			if strings.Contains(f.Name(), config.BinaryPrefix) {
				fullPath := filepath.Join(path, f.Name())
				cmd, err := startBinary(fullPath, port)
				if err == nil && verifyBinary(port, last.Version.String()) {
					log.Printf("Rolled back to version %s on port %d", last.Version, port)
					s.Active = &models.BinaryInstance{Version: last.Version, Port: port, Cmd: cmd}
				}
				break
			}
		}
	}
	s.mux.Unlock()
}
