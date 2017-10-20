package main

import (
	"bytes"
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
	TIMEOUT = time.Second * 5
)

var (
	charDevicePath *string
)

type GPSDevice struct {
	*os.File
}

func (d *GPSDevice) ReadSentence() (string, error) {
	buf := make([]byte, os.Getpagesize())
	sentence := make([]byte, 0)
	t := time.Now()
	for {
		count, err := d.Read(buf)
		if err != nil {
			return "", err
		}

		// log.Printf("Read %d bytes: %q (since: %s)\n", count, buf[:count], time.Since(now))

		if count == 0 || bytes.Equal(buf[:count], []byte("\n")) {
			break
		}

		if time.Since(t) > TIMEOUT {
			return string(sentence), fmt.Errorf("timeout")
		}

		sentence = append(sentence, buf[:count]...)
	}

	// log.Printf("Read complete GPS message: %q (since: %s)\n", msg, time.Since(now))
	return string(sentence), nil
}

func init() {
	charDevicePath = flag.String("input", "/dev/ttyUSB0", "Char device path related to the serial port of the GPS device")
}

func main() {
	flag.Parse()

	// Validate input file
	fi, err := os.Stat(*charDevicePath)

	if os.IsNotExist(err) {
		log.Panicln("File", *charDevicePath, "doesn't exists")
	}

	if err != nil {
		log.Panicln("Unable to use file", *charDevicePath, "as input, err:", err)
	}

	// Bitwises to validate right input file
	if fi.Mode()&os.ModeCharDevice == 0 || fi.Mode()&os.ModeDevice == 0 {
		log.Panicf("Input file should be a char device file (got: %s, wanted: %s)\n", fi.Mode(), os.FileMode(os.ModeCharDevice|os.ModeDevice).String())
	}

	// Try to open file or fatal
	f, err := os.Open(*charDevicePath)
	if err != nil {
		log.Fatalln(err)
	}

	start := time.Now()

	// Signal handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	go func() {
		<-signals
		log.Println("Exiting...")
		os.Exit(0)
	}()

	// Begin to read GPS informations and parse them
	gpsDev := GPSDevice{f}
	for {
		sentence, err := gpsDev.ReadSentence()
		if err != nil {
			log.Fatalln("Unable to read sentence, err:", err)
		}

		msg, err := nmea.Parse(sentence)
		if err != nil {
			log.Println("Invalid NMEA message, err:", err)
			continue
		}

		log.Printf("Handle NMEA message: %s (since: %s)", msg.Serialize(), time.Since(start))
	}
}
