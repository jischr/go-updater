package service

import (
	"sync"

	"updater/internal/config"
	"updater/pkg/models"

	"github.com/Masterminds/semver/v3"
)

type VersionManager struct {
	config     *config.Config
	mux        *sync.RWMutex
	active     *models.BinaryInstance
	previous   []*models.BinaryInstance
	onRollback func(oldVersion, newVersion *semver.Version)
}

func NewVersionManager(config *config.Config, mux *sync.RWMutex) *VersionManager {
	return &VersionManager{
		config:   config,
		mux:      mux,
		previous: make([]*models.BinaryInstance, 0),
	}
}

func (vm *VersionManager) SetActive(instance *models.BinaryInstance) {
	vm.mux.Lock()
	defer vm.mux.Unlock()

	if vm.active != nil {
		vm.previous = append(vm.previous, vm.active)
	}
	vm.active = instance
}

func (vm *VersionManager) GetActive() *models.BinaryInstance {
	vm.mux.RLock()
	defer vm.mux.RUnlock()
	return vm.active
}

func (vm *VersionManager) ShouldUpdate(newVersion *semver.Version) bool {
	vm.mux.RLock()
	defer vm.mux.RUnlock()

	if vm.active == nil {
		return true
	}
	return newVersion.GreaterThan(vm.active.Version)
}

func (vm *VersionManager) Rollback() *models.BinaryInstance {
	vm.mux.Lock()
	defer vm.mux.Unlock()

	if len(vm.previous) == 0 {
		return nil
	}

	last := vm.previous[len(vm.previous)-1]
	vm.previous = vm.previous[:len(vm.previous)-1]

	if vm.onRollback != nil {
		vm.onRollback(vm.active.Version, last.Version)
	}

	vm.active = last
	return last
}

func (vm *VersionManager) SetRollbackCallback(callback func(oldVersion, newVersion *semver.Version)) {
	vm.onRollback = callback
}
