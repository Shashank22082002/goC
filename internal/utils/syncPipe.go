package utils

import (
	"fmt"
	"os"
)

// Sync Pipe handles parent child synchronization

type SyncPipe struct {
	// Pipe has 2 ends
	// read end and write end
	r *os.File
	w *os.File
}

func NewSyncPipe() (*SyncPipe, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("Failed to create pipe: %v", err)
	}
	return &SyncPipe{r: r, w: w}, nil
}

func (sp *SyncPipe) Close() {
	if sp.r != nil {
		sp.r.Close()
	}
	if sp.w != nil {
		sp.w.Close()
	}
}


func (sp *SyncPipe) SignalReady() {
    sp.w.Close()
}

func (sp *SyncPipe) GetReadFile() *os.File {
    return sp.r
}
