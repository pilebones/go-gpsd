package main

import (
	"fmt"
	"log"
	"sync"
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

func autodetect(timeout time.Duration) (*string, error) {

	pathQueue, errQueue := make(chan string), make(chan error)
	matcher := getGPSMatcher()

	go lookupExistingDevices(matcher, pathQueue, errQueue, timeout)
	go monitorDevices(matcher, pathQueue, errQueue, timeout)

	select {
	case err := <-errQueue:
		return nil, err
	case path := <-pathQueue:
		return &path, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("Reach timeout %s", timeout.String())
	}
}

func lookupExistingDevices(matcher netlink.Matcher, pathQueue chan string, errQueue chan error, timeout time.Duration) {

	queue := make(chan crawler.Device)
	loop := true
	quit := crawler.ExistingDevices(queue, errQueue, matcher)
	var wg sync.WaitGroup
	w := NewFileAnalyzerWorker()

	defer func() {
		quit <- struct{}{} // Close properly udev monitor
	}()

	for loop {
		select {
		case result := <-w.Analyzed:
			wg.Done()
			/*if result.Error != nil {
				log.Println(result.Path, "not GPS device", result.Error.Error())
			}*/
			if result.Found {
				pathQueue <- result.Path
				loop = false
			}
		case device := <-queue:
			if name, exists := device.Env["DEVNAME"]; exists {
				path := fmt.Sprintf("/dev/%s", name)

				wg.Add(1)
				w.CheckFile(path, timeout)
			}
		case <-time.After(timeout):
			loop = false
			errQueue <- fmt.Errorf("Reach timeout %s", timeout.String())
		}
	}

	wg.Wait() // Wait batch of goroutines
}

func monitorDevices(matcher netlink.Matcher, pathQueue chan string, errQueue chan error, timeout time.Duration) {

	conn := new(netlink.UEventConn)
	if err := conn.Connect(); err != nil {
		errQueue <- fmt.Errorf("Unable to connect to kernel netlink socket, err: %s", err.Error())
		return
	}

	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, errQueue, matcher)
	loop := true
	var wg sync.WaitGroup
	w := NewFileAnalyzerWorker()

	defer func() {
		quit <- struct{}{} // Close properly udev monitor
		conn.Close()
	}()

	for loop {
		select {
		case result := <-w.Analyzed:
			wg.Done()
			/*if result.Error != nil {
				log.Println(result.Path, "not GPS device", result.Error.Error())
			}*/
			if result.Found {
				pathQueue <- result.Path
				loop = false
			}
		case uevent := <-queue:
			log.Println("Handle uevent:", uevent.String())
			loop = false
			wg.Add(1)
			w.CheckFile(fmt.Sprintf("/dev/%s", uevent.Env["DEVNAME"]), timeout)
		case <-time.After(timeout):
			loop = false
			errQueue <- fmt.Errorf("Reach timeout %s", timeout.String())
		}
	}

	wg.Wait() // Wait batch of goroutines
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

type FileAnalyzerWorker struct {
	Analyzed chan FileAnalyzerResult
}

func NewFileAnalyzerWorker() FileAnalyzerWorker {
	return FileAnalyzerWorker{
		Analyzed: make(chan FileAnalyzerResult),
	}
}

func (w *FileAnalyzerWorker) CheckFile(path string, timeout time.Duration) {
	log.Println("DetectGPSDeviceWorker check", path)

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
		if _, err := gpsDev.ReadSentence(timeout); err != nil {
			w.Analyzed <- NewFileAnalyzerResult(false, path, err)
			return
		}

		log.Println("DetectGPSDeviceWorker found", path)
		w.Analyzed <- NewFileAnalyzerResult(true, path, nil)
		return
	}()

	return
}
