package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	nmea "github.com/pilebones/go-nmea"
)

const (
	THRESHOLD = 2048 // To precise
)

type GPSDevice struct {
	*os.File
}

func NewGPSDevice(absPath string) (*GPSDevice, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	return &GPSDevice{f}, nil
}

func (d *GPSDevice) Monitor(queue chan nmea.NMEA, errors chan error, timeout time.Duration) chan bool {
	quit := make(chan bool, 1)
	go func() {
		loop := true
		for loop {
			select {
			case <-quit:
				loop = false
				break
			default:
				sentence, err := d.ReadSentence(timeout)
				if err != nil {
					errors <- fmt.Errorf("Unable to read sentence, err: %s", err.Error())
					<-quit // Fatal error
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

func (d *GPSDevice) ReadSentence(timeout time.Duration) (string, error) {
	buf := make([]byte, os.Getpagesize())
	sentence := make([]byte, 0)
	for {
		select {
		case <-time.After(timeout):
			return string(sentence), fmt.Errorf("timeout")
		default:
			count, err := d.Read(buf)
			if err != nil {
				return "", err
			}

			// log.Printf("Read %d bytes: %q (since: %s)\n", count, buf[:count], time.Since(now))
			if count == 0 || bytes.Equal(buf[:count], []byte("\n")) {
				// log.Printf("Read complete GPS message: %q (since: %s)\n", msg, time.Since(now))
				return string(sentence), nil
			}

			sentence = append(sentence, buf[:count]...)

			if len(sentence) > THRESHOLD {
				return "", fmt.Errorf("Message too long to be a GPS sentence")
			}
		}
	}

}
