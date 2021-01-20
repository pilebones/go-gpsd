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

// getGPSMatcher return default matcher to select GPS device
func getGPSMatcher() netlink.Matcher {
	action := regexp.QuoteMeta(netlink.ADD.String())
	return &netlink.RuleDefinition{
		Action: &action,
		Env: map[string]string{
			"SUBSYSTEM": "tty",
			"DEVNAME":   "ttyUSB\\d+",
		},
	}
}

// Autodetect try to detect new or existing plugged GPS device
// This function should be run as root to have right privilege
func Autodetect(ctx context.Context) (*string, error) {
	startTime := time.Now()
	pathQueue, errQueue := make(chan string), make(chan error)
	matcher := getGPSMatcher()
	worker := NewFileAnalyzerWorker()

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
			return nil, fmt.Errorf("Reach timeout after %s", time.Now().Sub(startTime).String())
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
	conn := new(netlink.UEventConn)

	if err := conn.Connect(netlink.UdevEvent); err != nil {
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
