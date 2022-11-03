package syncronizedmap

import (
	"fmt"
	"sync"
)

//go:generate mockgen -source=syncronizedmap.go -package=syncronizedmap -destination=mock_syncronizedmap.go

type KernelOsDtkMapping interface {
	SetNodeInfo(kernelVersion, osImage string)
	SetImageStreamInfo(osImage, dtkImage string)
	GetImage(kernelVersion string) (string, error)
}

type kernelOsDtkMapping struct {
	kernelToOsMutex *sync.RWMutex
	osToDtkMutext   *sync.RWMutex
	kernelToOs      map[string]string
	osToDtk         map[string]string
}

func NewKernelOsDtkMapping() KernelOsDtkMapping {

	return &kernelOsDtkMapping{
		kernelToOsMutex: &sync.RWMutex{},
		osToDtkMutext:   &sync.RWMutex{},
		kernelToOs:      map[string]string{},
		osToDtk:         map[string]string{},
	}
}

func (skom *kernelOsDtkMapping) SetNodeInfo(kernelVersion, osImage string) {

	skom.kernelToOsMutex.Lock()
	defer skom.kernelToOsMutex.Unlock()

	skom.kernelToOs[kernelVersion] = osImage
}

func (skom *kernelOsDtkMapping) SetImageStreamInfo(osImage, dtkImage string) {

	skom.osToDtkMutext.Lock()
	defer skom.osToDtkMutext.Unlock()

	skom.osToDtk[osImage] = dtkImage
}

func (skom *kernelOsDtkMapping) GetImage(kernelVersion string) (string, error) {

	skom.kernelToOsMutex.RLock()
	defer skom.kernelToOsMutex.RUnlock()

	osImage, ok := skom.kernelToOs[kernelVersion]
	if !ok {
		return "", fmt.Errorf("could not find kernel %s in kernel --> OS mapping", kernelVersion)
	}

	skom.osToDtkMutext.RLock()
	defer skom.osToDtkMutext.RUnlock()

	dtk, ok := skom.osToDtk[osImage]
	if !ok {
		return "", fmt.Errorf("could not find osImage %s in OS --> DTK mapping", osImage)
	}
	return dtk, nil
}
