package main

import (
	"errors"
	"time"
)

type CharDevAnalyzerResult struct {
	Path  string
	Error error
}

// FileAnalyzerWorker is a worker to analyze asynchronously a char device to
// detect if the content match NMEA protocol as a GPS device
type CharDevAnalyzerWorker struct {
	pathQueue   chan string
	ResultQueue chan CharDevAnalyzerResult
}

func NewCharDevAnalyzerWorker() *CharDevAnalyzerWorker {
	return &CharDevAnalyzerWorker{
		pathQueue:   make(chan string),
		ResultQueue: make(chan CharDevAnalyzerResult),
	}
}

func (w *CharDevAnalyzerWorker) Start(timeout time.Duration) chan struct{} {
	quit := make(chan struct{}, 1)
	go func() {
		for {
			select {
			case <-quit:
				return
			case p := <-w.pathQueue:
				w.ResultQueue <- CharDevAnalyzerResult{
					Path:  p,
					Error: checkFile(timeout, p),
				}
			}
		}
	}()
	return quit
}

func (w *CharDevAnalyzerWorker) Analyze(path string) {
	w.pathQueue <- path
}

// CheckFile asynchronously then return result in the channel
func checkFile(timeout time.Duration, path string) error {
	if path == "" {
		return errors.New("empty path")
	}

	var err error
	if err = IsCharDevice(path); err != nil {
		return err
	}

	var dev *GPSDevice
	if dev, err = NewGPSDevice(path, timeout); err != nil {
		return err
	}

	// Now try to read at least one NMEA message from char device
	// TODO: try multiple times to avoid misdetection when decode failure occurred
	_, err = dev.ReadSentence()
	return err
}
