package main

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"time"

	"github.com/pilebones/go-udev/crawler"
	"github.com/pilebones/go-udev/netlink"
)

var conn *netlink.UEventConn

func init() {
	conn = new(netlink.UEventConn)
	if err := conn.Connect(netlink.UdevEvent); err != nil {
		log.Fatalln("Unable to connect to kernel netlink socket, err: %s", err.Error())
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

// Autodetect try to detect new or existing plugged GPS device
// This function should be run as root to have right privilege
func Autodetect(ctx context.Context) (*string, error) {
	startTime := time.Now()
	pathQueue, errQueue := make(chan string), make(chan error)
	worker := NewFileAnalyzerWorker()

	go lookupExistingDevices(getGPSMatcher(), pathQueue, errQueue, ctx)
	go monitorDevices(getGPSMatcherByAction(netlink.ADD), pathQueue, errQueue, ctx)

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
			return nil, fmt.Errorf("Reach timeout after %s", time.Since(startTime).String())
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
		case device, more := <-queue:
			if !more {
				loop = false
			}

			if name, exists := device.Env["DEVNAME"]; exists {
				pathQueue <- fmt.Sprintf("/dev/%s", name)
			}
		}
	}
}

// monitorDevices listen UEvent kernel socket and try to match rules from matcher with handled uevent
func monitorDevices(matcher netlink.Matcher, pathQueue chan string, errQueue chan error, ctx context.Context) {
	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, errQueue, matcher)
	loop := true

	defer func() {
		quit <- struct{}{} // Close properly udev monitor
		conn.Close()
	}()

	fn := func(uevent netlink.UEvent) {
		log.Println("Handle uevent:", uevent.String())
		pathQueue <- fmt.Sprintf("/dev/%s", uevent.Env["DEVNAME"])
	}

	for loop {
		if ctx == nil {
			select {
			case uevent := <-queue:
				fn(uevent)
			}
		} else {
			select {
			case uevent := <-queue:
				fn(uevent)
			case <-ctx.Done():
				loop = false
			}
		}
	}
}
