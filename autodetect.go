package main

import (
	"context"
	"errors"
	"log"
	"path/filepath"
	"regexp"
	"strings"

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
			"DEVNAME":   "ttyUSB\\d+",
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

	pathQueue := make(chan string) // filepaths matched by the matcher rules
	errQueue := make(chan error)   // errors occured during lookup and monitoring
	go lookupExistingDevices(ctx, getGPSMatcher(), pathQueue, errQueue)
	go monitorDevices(ctx, getGPSMatcherByAction(netlink.ADD), pathQueue, errQueue)

	// Start worker to analyze auto-detected char devices
	// to verify if we can gather NMEA sentences
	worker := NewCharDevAnalyzerWorker()
	worker.Start(conf.GPSReaderTimeout)

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

// lookupExistingDevices lookup inside /sys/devices uevent struct which match rules from matcher
func lookupExistingDevices(ctx context.Context, matcher netlink.Matcher, pathQueue chan string, errQueue chan error) {
	queue := make(chan crawler.Device)
	stop := crawler.ExistingDevices(queue, errQueue, matcher)

	defer func() {
		stop <- struct{}{} // Close properly udev monitor
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case device, more := <-queue:
			if !more {
				return
			}

			if devpath := getDevPathByEnv(device.Env); devpath != "" {
				log.Println("Matcher permit to found a plugged device:", device.KObj)
				pathQueue <- devpath
			}
		}
	}
}

// monitorDevices listen UEvent kernel socket and try to match rules from matcher with handled uevent
func monitorDevices(ctx context.Context, matcher netlink.Matcher, pathQueue chan string, errQueue chan error) {
	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, errQueue, matcher)

	defer func() {
		quit <- struct{}{} // Close properly udev monitor
		conn.Close()
	}()

	for {
		select {
		case uevent := <-queue:
			if devpath := getDevPathByEnv(uevent.Env); devpath != "" {
				log.Println("Matcher handle uevent:", uevent.String())
				pathQueue <- devpath
			}
		case <-ctx.Done():
			return
		}
	}
}
