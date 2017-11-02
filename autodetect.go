package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

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

func autodetect(timeout time.Duration) (*string, error) {

	pathQueue, errQueue := make(chan string), make(chan error)
	matcher := getGPSMatcher()
	go crawlExistingDevices(matcher, pathQueue, errQueue, timeout)
	go monitorDevices(matcher, pathQueue, errQueue, timeout)

	select {
	case err := <-errQueue:
		return nil, err
	case d := <-pathQueue:
		return &d, nil
	case <-time.After(timeout):
		return nil, fmt.Errorf("Reach timeout %s", timeout.String())
	}
}

func getEventFromUEventFile(path string) (rv map[string]string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}

	defer f.Close()

	data, err := ioutil.ReadAll(f)
	if err != nil {
		return nil, err
	}

	rv = make(map[string]string, 0)
	buf := bufio.NewScanner(bytes.NewBuffer(data))

	var line string
	for buf.Scan() {
		line = buf.Text()
		field := strings.SplitN(line, "=", 2)
		if len(field) != 2 {
			return
		}
		rv[field[0]] = field[1]
	}
	return
}

func crawlExistingDevices(matcher netlink.Matcher, p chan string, e chan error, timeout time.Duration) {

	var wg sync.WaitGroup
	i := 0 // Counter of goroutines

	err := filepath.Walk("/sys/devices", func(path string, info os.FileInfo, err error) error {
		// err := filepath.Walk("/sys/dev/char", func(path string, info os.FileInfo, err error) error {
		// err := filepath.Walk("/sys/devices/pci0000:00/0000:00:14.0/usb1/1-2/1-2:1.0/", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || info.Name() != "uevent" {
			return nil
		}

		env, err := getEventFromUEventFile(path)
		if err != nil {
			return err
		}

		if matcher.EvaluateEnv(env) {
			if devname, exists := env["DEVNAME"]; exists {

				devpath := fmt.Sprintf("/dev/%s", devname)

				fi, err := os.Stat(devpath)
				if err != nil {
					return err
				}

				if fi.IsDir() || fi.Mode()&os.ModeCharDevice == 0 || fi.Mode()&os.ModeDevice == 0 {
					return nil
				}

				// Try to read in char dev to detect GPS message inside
				gpsDev, err := NewGPSDevice(devpath)
				if err != nil {
					return err
				}

				i += 1
				wg.Add(1)

				go func() {
					defer wg.Done()
					// TODO: try multiple times to avoid misdetection when decode failure occured
					if _, err = gpsDev.ReadSentence(timeout); err != nil {
						return // do not display error because file not contains GPS msg
					}

					log.Println("Device crawler: found", devpath)
					p <- devpath
				}()
			}
		}
		return nil
	})

	if err != nil {
		log.Println("An error occured, err:", err)
		e <- err
		return
	}

	wg.Wait() // Wait batch of goroutines

}

func monitorDevices(matcher netlink.Matcher, p chan string, e chan error, timeout time.Duration) {

	conn := new(netlink.UEventConn)
	if err := conn.Connect(); err != nil {
		e <- fmt.Errorf("Unable to connect to kernel netlink socket, err: %s", err.Error())
		return
	}

	queue := make(chan netlink.UEvent)
	quit := conn.Monitor(queue, matcher)
	loop := true

	defer func() {
		quit <- true // Close properly udev monitor
		conn.Close()
	}()

	for loop {
		select {
		case uevent := <-queue:
			log.Println("Handle uevent:", uevent.String())
			loop = false
			p <- fmt.Sprintf("/dev/%s", uevent.Env["DEVNAME"])
		case <-time.After(timeout):
			loop = false
			e <- fmt.Errorf("Reach timeout %s", timeout.String())
		}
	}
}
