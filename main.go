package main

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"sync"
	"time"

	"updater/internal/clients"
	"updater/internal/config"
	"updater/internal/service"
	"updater/pkg/models"
)

var (
	mux sync.RWMutex
)

func main() {
	os.MkdirAll("bin", 0755)

	config := config.DefaultConfig()

	// Create a shared pointer to the active instance
	var activeInstance *models.BinaryInstance

	updateService := service.NewUpdateService(config, clients.NewGitHubClient(config), &mux, activeInstance)

	// Pass a function that gets the current active instance
	go startReverseProxy(config.ProxyPort, func() *models.BinaryInstance {
		mux.RLock()
		defer mux.RUnlock()
		return updateService.Active
	})
	go updateLoop(updateService)
	select {}
}

func updateLoop(updateService *service.UpdateService) {
	// TODO: Update to use CRON job or time ticker
	for {
		log.Println("Checking for updates...")
		if err := updateService.CheckForUpdates(); err != nil {
			log.Println("Update error:", err)
		}
		time.Sleep(updateService.Config.CheckInterval)
	}
}

func startReverseProxy(proxyPort int, getActiveInstance func() *models.BinaryInstance) {
	log.Printf("Proxy listening on :%d", proxyPort)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inst := getActiveInstance()
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
