package main

import (
	"context"
	"fmt"
)

// FileAnalyzerWorker is a worker to analyze a file to
// detect if the content match NMEA protocol
type FileAnalyzerWorker struct {
	Analyzed chan FileAnalyzerResult
}

func NewFileAnalyzerWorker() FileAnalyzerWorker {
	return FileAnalyzerWorker{
		Analyzed: make(chan FileAnalyzerResult),
	}
}

type FileAnalyzerResult struct {
	Found bool
	Error error
	Path  string
}

func NewFileAnalyzerResult(found bool, path string, err error) FileAnalyzerResult {
	return FileAnalyzerResult{
		Found: found,
		Path:  path,
		Error: err,
	}
}

func (w *FileAnalyzerWorker) CheckFile(path string, ctx context.Context) {
	// log.Println("DetectGPSDeviceWorker check", path)

	if path == "" {
		w.Analyzed <- NewFileAnalyzerResult(false, path, fmt.Errorf("empty path"))
		return
	}

	if err := IsCharDevice(path); err != nil {
		w.Analyzed <- NewFileAnalyzerResult(false, path, err)
		return
	}

	gpsDev, err := NewGPSDevice(path)
	if err != nil {
		w.Analyzed <- NewFileAnalyzerResult(false, path, err)
		return
	}

	go func() {
		// TODO: try multiple times to avoid misdetection when decode failure occurred
		if _, err := gpsDev.ReadSentence(ctx); err != nil {
			w.Analyzed <- NewFileAnalyzerResult(false, path, err)
			return
		}

		// log.Println("DetectGPSDeviceWorker found", path)
		w.Analyzed <- NewFileAnalyzerResult(true, path, nil)
	}()
}
