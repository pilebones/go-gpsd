package main

import (
	"bytes"
	"fmt"
	"os"
	"time"

	nmea "github.com/pilebones/go-nmea"
)

type GPSDevice struct {
	*os.File
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
					errors <- fmt.Errorf("Invalid NMEA message, err:", err)
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

		if time.Since(t) > timeout {
			return string(sentence), fmt.Errorf("timeout")
		}

		sentence = append(sentence, buf[:count]...)
	}

	// log.Printf("Read complete GPS message: %q (since: %s)\n", msg, time.Since(now))
	return string(sentence), nil
}
