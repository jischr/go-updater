package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Masterminds/semver/v3"
)

const (
	repoOwner       = "jischr"
	repoName        = "simple-server"
	checkInterval   = 30 * time.Second
	binaryPrefix    = "simple-server"
	proxyPort       = 8080
	startingPort    = 5000
	githubAPILatest = "https://api.github.com/repos/%s/%s/releases/latest"
)

type GitHubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

type BinaryInstance struct {
	Version *semver.Version
	Port    int
	Cmd     *exec.Cmd
}

var (
	active   *BinaryInstance
	previous []*BinaryInstance
	mux      sync.RWMutex
)

func main() {
	os.MkdirAll("bin", 0755)
	go startReverseProxy()
	go updateLoop()
	select {}
}

func updateLoop() {
	for {
		log.Println("Checking for updates...")
		if err := checkForUpdates(); err != nil {
			log.Println("Update error:", err)
		}
		time.Sleep(checkInterval)
	}
}

func checkForUpdates() error {
	release, err := fetchLatestRelease()
	if err != nil {
		return err
	}
	tag := strings.TrimPrefix(release.TagName, "v")
	newVersion, err := semver.NewVersion(tag)
	if err != nil {
		return err
	}

	mux.RLock()
	if active != nil && !newVersion.GreaterThan(active.Version) {
		mux.RUnlock()
		return nil
	}
	mux.RUnlock()

	assetURL := findAssetURL(release)
	if assetURL == "" {
		return fmt.Errorf("no matching asset found")
	}

	binPath, err := downloadAndExtract(assetURL, newVersion)
	if err != nil {
		return err
	}

	port := findFreePort(startingPort + 1)
	cmd, err := startBinary(binPath, port)
	if err != nil {
		return err
	}

	if !verifyBinary(port, newVersion.String()) {
		log.Printf("Verification failed for version %s on port %d", newVersion, port)
		cmd.Process.Kill()
		return fmt.Errorf("verification failed")
	}

	mux.Lock()
	old := active
	active = &BinaryInstance{Version: newVersion, Port: port, Cmd: cmd}
	if old != nil {
		previous = append(previous, old)
		go gracefulShutdown(old)
	}
	mux.Unlock()
	go monitorAndRollback(active, old)
	return nil
}

func fetchLatestRelease() (*GitHubRelease, error) {
	url := fmt.Sprintf(githubAPILatest, repoOwner, repoName)
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var release GitHubRelease
	err = json.NewDecoder(resp.Body).Decode(&release)
	return &release, err
}

func findAssetURL(release *GitHubRelease) string {
	os := runtime.GOOS
	arch := runtime.GOARCH
	for _, asset := range release.Assets {
		assetName := strings.ToLower(asset.Name)
		if strings.Contains(assetName, os) && strings.Contains(assetName, arch) {
			return asset.BrowserDownloadURL
		}
	}
	return ""
}

func downloadAndExtract(url string, version *semver.Version) (string, error) {
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

func startReverseProxy() {
	log.Printf("Proxy listening on :%d", proxyPort)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.RLock()
		inst := active
		mux.RUnlock()
		if inst == nil {
			log.Println("No active instance available to serve the request")
			http.Error(w, "no instance available", http.StatusServiceUnavailable)
			return
		}
		target := fmt.Sprintf("http://localhost:%d", inst.Port)
		log.Printf("Forwarding request to %s", target)
		u, _ := url.Parse(target)
		proxy := httputil.NewSingleHostReverseProxy(u)
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
			log.Printf("Proxy error: %v", err)
			http.Error(w, "proxy error", http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	})
	log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", proxyPort), handler))
}

func findFreePort(start int) int {
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			ln.Close()
			return p
		}
	}
	panic("no free port found")
}

func gracefulShutdown(inst *BinaryInstance) {
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

func monitorAndRollback(newInst, oldInst *BinaryInstance) {
	err := newInst.Cmd.Wait()
	log.Printf("Process for %s exited: %v", newInst.Version, err)
	mux.Lock()
	if active == newInst && len(previous) > 0 {
		last := previous[len(previous)-1]
		previous = previous[:len(previous)-1]
		port := findFreePort(startingPort + 1)
		path := filepath.Join("bin", fmt.Sprintf("%s-v%s", binaryPrefix, last.Version))
		files, _ := os.ReadDir(path)
		for _, f := range files {
			if strings.Contains(f.Name(), binaryPrefix) {
				fullPath := filepath.Join(path, f.Name())
				cmd, err := startBinary(fullPath, port)
				if err == nil && verifyBinary(port, last.Version.String()) {
					log.Printf("Rolled back to version %s on port %d", last.Version, port)
					active = &BinaryInstance{Version: last.Version, Port: port, Cmd: cmd}
				}
				break
			}
		}
	}
	mux.Unlock()
}
