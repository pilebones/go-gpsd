package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	nmea "github.com/pilebones/go-nmea"
)

const (
	TIMEOUT            = time.Second * 5
	AUTODETECT_TIMEOUT = time.Second * 10
)

var (
	charDevicePath             *string
	autoDetectMode             *bool
	autoDetectTimeout, timeout *time.Duration
)

func init() {
	charDevicePath = flag.String("input", "/dev/ttyUSB0", "Char device path related to the serial port of the GPS device")
	autoDetectMode = flag.Bool("autodetect", false, "Allow to enable auto-detection of the GPS device (already plugged or hot-plugged)")
	timeout = flag.Duration("timeout", TIMEOUT, "Max duration allowed to read and parse a GPS sentence from serial-port")
	autoDetectTimeout = flag.Duration("autodetect-timeout", AUTODETECT_TIMEOUT, "Time spent to try to autodetect the GPS device (exit 2 if fail)")
}

func main() {
	flag.Parse()

	var err error

	if *autoDetectMode {
		if charDevicePath, err = autodetect(*autoDetectTimeout); err != nil {
			log.Fatalln(err.Error())
		}
	}

	if err = validateCharDevice(*charDevicePath); err != nil {
		log.Fatalln(err.Error())
	}

	// Try to open file or fatal
	f, err := os.Open(*charDevicePath)
	if err != nil {
		log.Fatalln(err)
	}

	// Begin to read GPS informations and parse them
	gpsDev := GPSDevice{f}
	queue, errors := make(chan nmea.NMEA), make(chan error)
	quit := gpsDev.Monitor(queue, errors, *timeout)

	// Signal handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("Exiting...")
		quit <- true
		os.Exit(0)
	}()

	for {
		select {
		case msg := <-queue:
			log.Printf("Handle NMEA message: %s (since: %s)", msg.Serialize())
		case err := <-errors:
			log.Println(err)
		}
	}
}

func validateCharDevice(path string) error {
	// Validate input file
	fi, err := os.Stat(path)

	if os.IsNotExist(err) {
		return fmt.Errorf("File", path, "doesn't exists")
	}

	if err != nil {
		return fmt.Errorf("Unable to use file", path, "as input, err:", err)
	}

	// Bitwises to validate right input file
	if fi.Mode()&os.ModeCharDevice == 0 || fi.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("Input file should be a char device file (got: %s, wanted: %s)\n", fi.Mode(), os.FileMode(os.ModeCharDevice|os.ModeDevice).String())
	}

	return nil
}
