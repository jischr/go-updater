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
	"strconv"
	"strings"
	"time"

	"updater/internal/config"

	"github.com/Masterminds/semver/v3"
)

type BinaryManager struct {
	config *config.Config
}

func NewBinaryManager(config *config.Config) *BinaryManager {
	return &BinaryManager{
		config: config,
	}
}

func (bm *BinaryManager) DownloadAndExtract(url string, version *semver.Version) (string, error) {
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

	extractDir := filepath.Join("bin", fmt.Sprintf("%s-v%s", bm.config.BinaryPrefix, version))
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
			if strings.Contains(hdr.Name, bm.config.BinaryPrefix) && hdr.FileInfo().Mode().IsRegular() && (hdr.FileInfo().Mode()&0111 != 0) {
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
			if strings.Contains(f.Name, bm.config.BinaryPrefix) && (f.Mode()&0111 != 0) {
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

func (bm *BinaryManager) StartBinary(path string, port int) (*exec.Cmd, error) {
	log.Printf("Starting binary %s on port %d", path, port)
	cmd := exec.Command(path, strconv.Itoa(port))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Start()
	return cmd, err
}

func (bm *BinaryManager) VerifyBinary(port int, expected string) bool {
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

		version := string(versionBytes)
		if version == expected {
			log.Printf("Verified binary on port %d with version %s", port, version)
			return true
		}
	}
	log.Printf("Failed to verify binary on port %d", port)
	return false
}

func (bm *BinaryManager) FindFreePort() int {
	start := bm.config.StartingPort + 1
	for p := start; p < start+100; p++ {
		ln, err := net.Listen("tcp", fmt.Sprintf(":%d", p))
		if err == nil {
			ln.Close()
			return p
		}
	}
	panic("no free port found")
}
