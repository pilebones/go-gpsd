package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
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
		return err
	}

	// Bitwises to validate right input file
	if fi.Mode()&os.ModeCharDevice == 0 || fi.Mode()&os.ModeDevice == 0 {
		return fmt.Errorf("Input file should be a char device file (got: %s, wanted: %s)",
			fi.Mode(), os.FileMode(os.ModeCharDevice|os.ModeDevice).String())
	}

	return nil
}

type GPSDevice struct {
	*os.File
	gpsReaderTimeout time.Duration
}

// NewGPSDevice create an instance of GPSDevice from path
func NewGPSDevice(absPath string, gpsReaderTimeout time.Duration) (*GPSDevice, error) {
	f, err := os.Open(absPath)
	if err != nil {
		return nil, err
	}
	return &GPSDevice{
		File:             f,
		gpsReaderTimeout: gpsReaderTimeout,
	}, nil
}

func (d *GPSDevice) StillExists() bool {
	_, err := d.Stat()
	return os.IsNotExist(err)
}

// Monitor run daemon to watch device and append to chan read messages
func (d *GPSDevice) Monitor(ctx context.Context, queue chan nmea.NMEA, errs chan error) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Begin to read GPS informations and parse them
			sentence, err := d.ReadSentence()
			if err != nil {
				errs <- fmt.Errorf("Unable to read sentence, err: %w", err)
				continue
			}
			if sentence == "" {
				errs <- errors.New("Empty NMEA sentence")
				time.Sleep(time.Second)
				return
			}

			msg, err := nmea.Parse(sentence)
			if err != nil {
				errs <- fmt.Errorf("Wrong NMEA message format, err: %w", err)
				continue
			}
			if msg == nil {
				errs <- fmt.Errorf("Invalid NMEA message, got: %s", sentence)
				continue
			}
			queue <- msg
		}
	}
}

// ReadSentence read a single NMEA message from device
func (d *GPSDevice) ReadSentence() (sentence string, err error) {

	ctx, cancel := context.WithTimeout(context.Background(), d.gpsReaderTimeout)
	defer cancel()

	i := 0
	found := false
	scanner := bufio.NewScanner(d)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
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
				return sentence, errors.New("Message too long to be a GPS sentence")
			}
		}
	}

	return
}
