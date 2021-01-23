package main

import "time"

const (
	// Thresholds for different context
	DefaultAutodetectTimeout = time.Second * 30
	DefaultGPSReaderTimeout  = time.Second * 5

	// GPS settings
	DefaultCharDevicePath   = "/dev/ttyUSB0"
	DefaultEnableAutodetect = false

	// API settings
	DefaultListenAddr = "127.0.0.1:1234"
)

type Config struct {
	ListenAddr             string
	GPSCharDevPath         string
	EnableGPSAutodetection bool

	AutodetectTimeout time.Duration
	GPSReaderTimeout  time.Duration
}
