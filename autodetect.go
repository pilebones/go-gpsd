package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/pilebones/go-udev/crawler"
	"github.com/pilebones/go-udev/netlink"
)

var (
	conn *netlink.UEventConn

	// errors handling
	ErrGPSDeviceAutodetectionTimeout = errors.New("reach timeout for GPS device autodetection")
)

func init() {
	conn = new(netlink.UEventConn)
	if err := conn.Connect(netlink.UdevEvent); err != nil {
		log.Fatalf("Unable to connect to kernel netlink socket, err: %v", err)
	}
}

// Default GPS device matcher used by go-udev
func getGPSMatcher() *netlink.RuleDefinition {
	return &netlink.RuleDefinition{
		Env: map[string]string{
			"SUBSYSTEM": "tty",
			"DEVNAME":   `(/dev/)?ttyUSB\d+`,
		},
	}
}

// getGPSMatcherByAction allocate a GPS matcher with specified action
func getGPSMatcherByAction(a netlink.KObjAction) netlink.Matcher {
	matcher := getGPSMatcher()
	action := regexp.QuoteMeta(a.String())
	matcher.Action = &action
	return matcher
}

func getDevPathByEnv(env map[string]string) string {
	if name, exists := env["DEVNAME"]; exists {
		if strings.HasPrefix(name, "/dev") {
			return name
		}
		return filepath.Join("/dev", name)
	}
	return ""
}

// Autodetect try to detect a GPS devices in those already plugged or hot-plugged
// NOTE: need elaveted privileges so this function may be run as root !
func Autodetect(conf *Config) (string, error) {

	// For performance reason, we do two searches in parallel in order to find a GPS type device.
	// One for the devices already pluggued to the host and another one by monitoring the devices hot-plugging.
	// Each goroutine will fill two channels (ie: pathQueue, errQueue) with data related to the matched devices by matcher.
	// Then the path is passing to a worker to be analyzed to be sure it is a GPS device (by we can read some NMEA sentences).
	// If some errors occured during the research or the analyze, the related channel is filled.
	ctx, cancel := context.WithTimeout(context.Background(), conf.AutodetectTimeout)
	defer cancel()

	var wg sync.WaitGroup

	pathQueue := make(chan string) // filepaths matched by the matcher rules
	errQueue := make(chan error)   // errors occured during lookup and monitoring
	wg.Add(1)
	go lookupExistingDevices(ctx, &wg, getGPSMatcher(), pathQueue, errQueue)
	wg.Add(1)
	go monitorDevices(ctx, &wg, getGPSMatcherByAction(netlink.ADD), pathQueue, errQueue)
	/*if err := monitorDevices(ctx, getGPSMatcherByAction(netlink.ADD), pathQueue, errQueue); err != nil {
		return "", err
	}*/

	// Start worker to analyze auto-detected char devices
	// to verify if we can gather NMEA sentences
	worker := NewCharDevAnalyzerWorker()
	quit := worker.Start(conf.GPSReaderTimeout)

	defer func() {
		close(quit) // Stop worker
		cancel()    // stop jobs (lookup and monitor)
		wg.Wait()   // wait routines to stop property
		log.Println("GPS autodetection terminated.")
	}()

	for {
		select {
		case <-ctx.Done():
			return "", ErrGPSDeviceAutodetectionTimeout
		case result := <-worker.ResultQueue:
			return result.Path, result.Error
		case err := <-errQueue:
			return "", err
		case path := <-pathQueue:
			log.Println("Matcher rules permit to found a device at", path, "and should be analyzed.")
			worker.Analyze(path)
		}
	}
}

// monitor if device unplugged notify when the device is unplugged
func NotifyIfDeviceUnplugged(ctx context.Context, path string, onUnplugged chan struct{}) {

	matcher := &netlink.RuleDefinition{
		Env: map[string]string{
			"SUBSYSTEM": "tty",
			"DEVNAME":   regexp.QuoteMeta(path),
		},
	}
	action := regexp.QuoteMeta(netlink.REMOVE.String())
	matcher.Action = &action
	log.Println(*matcher.Action, matcher.Env)
	pathQueue := make(chan string) // filepaths matched by the matcher rules
	errQueue := make(chan error)   // errors occured during lookup and monitoring

	var wg sync.WaitGroup
	wg.Add(1)
	go monitorDevices(ctx, &wg, matcher, pathQueue, errQueue)

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case e := <-errQueue:
			log.Println("Error:", e)
		case p := <-pathQueue:
			if p != path {
				log.Printf("Another device has been deconnected (match: %s, expected: %s)", p, path)
				continue loop
			}

			log.Println("GPS device has been unplugged:", p)
			close(onUnplugged)
			break loop
		}
	}

	wg.Wait() // wait routines to stop property
}

// lookupExistingDevices lookup inside /sys/devices uevent struct which match rules from matcher
func lookupExistingDevices(ctx context.Context, wg *sync.WaitGroup, matcher netlink.Matcher, pathQueue chan string, errQueue chan error) {
	defer wg.Done()
	queue := make(chan crawler.Device)
	stop := crawler.ExistingDevices(queue, errQueue, matcher)
	defer close(stop)

	for {
		select {
		case <-ctx.Done():
			return
		case device := <-queue:
			if devpath := getDevPathByEnv(device.Env); devpath != "" {
				// log.Println("Matcher permit to found a plugged device:", device.KObj)
				pathQueue <- devpath
			}
		}
	}
}

// monitorDevices listen UEvent kernel socket and try to match rules from matcher with handled uevent
func monitorDevices(ctx context.Context, wg *sync.WaitGroup, matcher netlink.Matcher, pathQueue chan string, errQueue chan error) {
	defer wg.Done()
	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, errQueue, matcher)
	defer close(quit)

	for {
		select {
		case <-ctx.Done():
			return
		case uevent := <-queue:
			if devpath := getDevPathByEnv(uevent.Env); devpath != "" {
				// log.Println("Matcher handle uevent:", uevent.String())
				pathQueue <- devpath
			}
		}
	}
}
