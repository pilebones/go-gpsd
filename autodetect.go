package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/pilebones/go-udev/netlink"
)

func gpsMatcher() netlink.Matcher {
	action := netlink.ADD.String()
	return &netlink.RuleDefinition{
		Action: &action,
		Env: map[string]string{
			"SUBSYSTEM": "tty",
			"DEVNAME":   "ttyUSB\\d+",
		},
	}
}

func validateCharDevice(path string) error {
	// Validate input file
	fi, err := os.Stat(path)

	if os.IsNotExist(err) {
		return fmt.Errorf("File %s doesn't exists", path)
	}

	if err != nil {
		return fmt.Errorf("Unable to use file %sas input, err: %s", path, err.Error())
	}

	// Bitwises to validate right input file
	if fi.Mode()&os.ModeCharDevice == 0 || fi.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("Input file should be a char device file (got: %s, wanted: %s)\n", fi.Mode(), os.FileMode(os.ModeCharDevice|os.ModeDevice).String())
	}

	return nil
}

func autodetect(timeout time.Duration) (charDevicePath *string, err error) {
	conn := new(netlink.UEventConn)
	if err = conn.Connect(); err != nil {
		return nil, fmt.Errorf("Unable to connect to kernel netlink socket, err: %s", err.Error())
	}
	defer conn.Close()

	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, gpsMatcher())
	loop := true

	closeFunc := func() {
		quit <- true
		loop = false
	}

	for loop {
		select {
		case uevent := <-queue:
			log.Println("Handle uevent:", uevent.String())
			absolutePath := fmt.Sprintf("/dev/%s", uevent.Env["DEVNAME"])
			charDevicePath = &absolutePath
			closeFunc()
		case <-time.After(timeout):
			closeFunc()
		}
	}

	// TODO: Implement autodetect if module already plugged:
	// Parse all /dev/ttyXX and read msg to check if could be
	// parse by go-nmea.Parse()

	return charDevicePath, nil
}
