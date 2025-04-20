package service

import (
	"log"
	"os"
	"time"

	"updater/pkg/models"
)

type ProcessManager struct{}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{}
}

func (pm *ProcessManager) GracefulShutdown(inst *models.BinaryInstance) {
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

func (pm *ProcessManager) MonitorProcess(inst *models.BinaryInstance, onExit func()) {
	go func() {
		err := inst.Cmd.Wait()
		log.Printf("Process for %s exited: %v", inst.Version, err)
		if onExit != nil {
			onExit()
		}
	}()
}
