package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/pilebones/go-udev/crawler"
	"github.com/pilebones/go-udev/netlink"
)

func getGPSMatcher() netlink.Matcher {
	action := netlink.ADD.String()
	return &netlink.RuleDefinition{
		Action: &action,
		Env: map[string]string{
			// "SUBSYSTEM": "tty",
			"DEVNAME": "ttyUSB\\d+",
		},
	}
}

// Autodetect try to detect new or existing plugged GPS device
// This function should be run as root to have right privilege
func Autodetect(timeout time.Duration) (*string, error) {
	pathQueue, errQueue := make(chan string), make(chan error)
	matcher := getGPSMatcher()
	worker := NewFileAnalyzerWorker()

	// Initialize context to stop goroutines when timeout or found device
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	go lookupExistingDevices(matcher, pathQueue, errQueue, ctx)
	go monitorDevices(matcher, pathQueue, errQueue, ctx)

	for {
		select {
		case result := <-worker.Analyzed:
			if result.Error != nil {
				errQueue <- result.Error
			}

			if result.Found {
				return &result.Path, nil
			}
		case err := <-errQueue:
			return nil, err
		case path := <-pathQueue:
			worker.CheckFile(path, ctx)
		case <-ctx.Done():
			return nil, fmt.Errorf("Reach timeout %s", timeout.String())
		}
	}
}

// lookupExistingDevices lookup inside /sys/devices uevent struct which match rules from matcher
func lookupExistingDevices(matcher netlink.Matcher, pathQueue chan string, errQueue chan error, ctx context.Context) {
	queue := make(chan crawler.Device)
	loop := true
	quit := crawler.ExistingDevices(queue, errQueue, matcher)

	defer func() {
		quit <- struct{}{} // Close properly udev monitor
	}()

	for loop {
		select {
		case <-ctx.Done():
			loop = false
		case device := <-queue:
			if name, exists := device.Env["DEVNAME"]; exists {
				pathQueue <- fmt.Sprintf("/dev/%s", name)
			}
		}
	}
}

// monitorDevices listen UEvent kernel socket and try to match rules from matcher with handled uevent
func monitorDevices(matcher netlink.Matcher, pathQueue chan string, errQueue chan error, ctx context.Context) {
	conn := new(netlink.UEventConn)

	if err := conn.Connect(); err != nil {
		errQueue <- fmt.Errorf("Unable to connect to kernel netlink socket, err: %s", err.Error())
		return
	}

	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, errQueue, matcher)
	loop := true

	defer func() {
		quit <- struct{}{} // Close properly udev monitor
		conn.Close()
	}()

	for loop {
		select {
		case uevent := <-queue:
			log.Println("Handle uevent:", uevent.String())
			pathQueue <- fmt.Sprintf("/dev/%s", uevent.Env["DEVNAME"])
		case <-ctx.Done():
			loop = false
		}
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

func (w *FileAnalyzerWorker) CheckFile(path string, ctx context.Context) {
	// log.Println("DetectGPSDeviceWorker check", path)

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
		// TODO: try multiple times to avoid misdetection when decode failure occured
		if _, err := gpsDev.ReadSentenceWithContext(ctx); err != nil {
			w.Analyzed <- NewFileAnalyzerResult(false, path, err)
			return
		}

		// log.Println("DetectGPSDeviceWorker found", path)
		w.Analyzed <- NewFileAnalyzerResult(true, path, nil)
		return
	}()

	return
}
