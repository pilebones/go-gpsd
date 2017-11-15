package main

import (
	"bytes"
	"context"
	"fmt"
	"os"

	nmea "github.com/pilebones/go-nmea"
)

const (
	THRESHOLD = 2048 // To precise
)

// IsCharDevice return true if device exists and if is it a char device
func IsCharDevice(path string) (err error) {
	var fi os.FileInfo
	if fi, err = os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("File %s doesn't exists", path)
		}
		return fmt.Errorf("Unable to use file %sas input, err: %s", path, err.Error())
	}

	// Bitwises to validate right input file
	if fi.Mode()&os.ModeCharDevice == 0 || fi.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("Input file should be a char device file (got: %s, wanted: %s)\n",
			fi.Mode(), os.FileMode(os.ModeCharDevice|os.ModeDevice).String())
	}

	return nil
}

type GPSDevice struct {
	*os.File
}

// NewGPSDevice create an instance of GPSDevice from path
func NewGPSDevice(absPath string) (*GPSDevice, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	return &GPSDevice{f}, nil
}

// Monitor run daemon to watch device and append to chan read messages
func (d *GPSDevice) Monitor(queue chan nmea.NMEA, errors chan error, ctx context.Context) chan struct{} {
	quit := make(chan struct{}, 1)
	go func() {
		loop := true
		for loop {
			select {
			case <-quit:
				loop = false
				break
			default:
				sentence, err := d.ReadSentence(ctx)
				if err != nil {
					errors <- fmt.Errorf("Unable to read sentence, err: %s", err.Error())
					quit <- struct{}{} // Fatal error
				}

				msg, err := nmea.Parse(sentence)
				if err != nil {
					errors <- fmt.Errorf("Wrong NMEA message format, err: %s", err.Error())
					continue
				}
				if msg == nil {
					errors <- fmt.Errorf("Invalid NMEA message, got: %s", sentence)
					continue
				}
				queue <- msg
			}
		}
	}()
	return quit
}

// ReadSentence read a single NMEA message from device
func (d *GPSDevice) ReadSentence(ctx context.Context) (string, error) {
	buf := make([]byte, os.Getpagesize())
	sentence := make([]byte, 0)
	for {
		select {
		case <-ctx.Done():
			return string(sentence), fmt.Errorf("timeout")
		default:
			count, err := d.Read(buf)
			if err != nil {
				return "", err
			}

			// log.Printf("Read %d bytes: %q\n", count, buf[:count])
			if count == 0 || bytes.Equal(buf[:count], []byte("\n")) {
				// log.Printf("Read complete GPS message: %q\n", sentence)
				return string(sentence), nil
			}

			sentence = append(sentence, buf[:count]...)

			if len(sentence) > THRESHOLD {
				return "", fmt.Errorf("Message too long to be a GPS sentence")
			}
		}
	}
}
