package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	nmea "github.com/pilebones/go-nmea"
)

const (
	TIMEOUT            = time.Second * 5
	AUTODETECT_TIMEOUT = time.Second * 5
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
		// Initialize context to stop goroutines when timeout or found device
		ctx, cancel := context.WithTimeout(context.Background(), *autoDetectTimeout)
		defer cancel()

		log.Println("Start autodetecting GPS devices.")
		// run job to autodetect GPS device
		if charDevicePath, err = Autodetect(ctx); err != nil {
			log.Fatalln("Unable to autodetect GPS device, err:", err.Error())
		}

		if charDevicePath == nil {
			log.Fatalln("Unable to autodetect GPS device in", autoDetectTimeout.String())
		}

		log.Println("Autodetect", *charDevicePath, " as a GPS device.")
	} else {
		// Initialize context to stop goroutines when timeout or read a valid GPS sentence
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()

		log.Println("Analyzing", *charDevicePath, "...")

		// Initialize and run worker
		worker := NewFileAnalyzerWorker()
		worker.CheckFile(*charDevicePath, ctx)

		select {
		case <-ctx.Done():
			log.Fatalln("Unable to validate GPS device in", timeout.String())
		case result := <-worker.Analyzed:
			if result.Error != nil {
				log.Fatalln(*charDevicePath, " is not a valid GPS device, err:", result.Error.Error())
			}
			if !result.Found {
				log.Fatalln(*charDevicePath, " doesn't contains NMEA message like a GPS device")
			}
		}
	}

	// Try to open file or fatal
	gpsDev, err := NewGPSDevice(*charDevicePath)
	if err != nil {
		log.Fatalln(err)
	}

	queue, errors := make(chan nmea.NMEA), make(chan error)
	quit := gpsDev.Monitor(queue, errors, *timeout)

	// Signal handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("Exiting...")
		quit <- struct{}{}
		os.Exit(0)
	}()

	for {
		select {
		case msg := <-queue:
			log.Printf("Handle NMEA message: %s", msg.Serialize())
		case err := <-errors:
			log.Println(err)
		}
	}
}
