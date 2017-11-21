package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	nmea "github.com/pilebones/go-nmea"
)

const (
	THRESHOLD = 256 // To precise
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
func (d *GPSDevice) Monitor(queue chan nmea.NMEA, errors chan error, timeout time.Duration) chan struct{} {
	quit := make(chan struct{}, 1)
	go func() {
		loop := true
		for loop {
			select {
			case <-quit:
				loop = false
				break
			default:
				// Begin to read GPS informations and parse them
				ctx, cancel := context.WithTimeout(context.Background(), timeout)
				defer cancel()

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
func (d *GPSDevice) ReadSentence(ctx context.Context) (sentence string, err error) {

	i := 0
	found := false
	scanner := bufio.NewScanner(d)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", fmt.Errorf("timeout")
		default:
			i += 1
			line := scanner.Bytes()

			if err = scanner.Err(); err != nil {
				return
			}

			// Drop this part if it's not expected magic-code or if first part is end of message
			if !found && (!bytes.HasPrefix(bytes.TrimSpace(line), []byte{'$'}) ||
				bytes.Equal(line, []byte{'\r', '\n'})) {
				// fmt.Println("drop part", string(line))
				continue
			}

			if len(line) == 0 || bytes.Equal(line, []byte{'\r', '\n'}) {
				// log.Printf("Read complete GPS message: %q\n", sentence)
				return
			}

			found = true
			sentence += string(bytes.TrimSpace(line))

			if sentence[len(sentence)-3] == '*' {
				// fmt.Println("end of msg by *")
				return
			}

			if len(sentence) > THRESHOLD {
				return sentence, fmt.Errorf("Message too long to be a GPS sentence")
			}
		}
	}

	return

}
