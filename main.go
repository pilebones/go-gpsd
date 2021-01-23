package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	nmea "github.com/pilebones/go-nmea"
)

func main() {
	conf := new(Config)
	flag.StringVar(&conf.GPSCharDevPath, "input", DefaultCharDevicePath, "char device path related to the serial port of the GPS device")
	flag.BoolVar(&conf.EnableGPSAutodetection, "autodetect", DefaultEnableAutodetect, "allow to enable GPS device auto-detection (already plugged or hot-plugged)")
	flag.DurationVar(&conf.GPSReaderTimeout, "timeout", DefaultGPSReaderTimeout, "max duration allowed to read and parse a GPS sentence from serial-port")
	flag.DurationVar(&conf.AutodetectTimeout, "autodetect-timeout", DefaultAutodetectTimeout, "max duration for autodetecting GPS device")
	flag.StringVar(&conf.ListenAddr, "addr", DefaultListenAddr, "Listen address for HTTP daemon")
	flag.Parse()

	if conf.EnableGPSAutodetection {
		log.Println("Starting GPS device auto-detection...")
		var err error
		if conf.GPSCharDevPath, err = Autodetect(conf); err != nil {
			log.Fatalln(err)
		}
	} else {
		log.Println("Analyzing", conf.GPSCharDevPath, "...")
		if err := checkFile(conf.GPSReaderTimeout, conf.GPSCharDevPath); err != nil {
			log.Fatalln(err)
		}
	}

	log.Println("Selecting device", conf.GPSCharDevPath)

	onUnplugged := make(chan struct{}, 1)
	go NotifyIfDeviceUnplugged(conf.GPSCharDevPath, onUnplugged)

	// Try to open file or fatal
	gps, err := NewGPSDevice(conf.GPSCharDevPath, conf.GPSReaderTimeout)
	if err != nil {
		log.Fatalln(err)
	}

	// Signals handler to quit properly monitor mode
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)

	// Monitor GPS device to gather NMEA sentences
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	queue, errors := make(chan nmea.NMEA), make(chan error)
	go gps.Monitor(ctx, queue, errors)

	// NMEA sentences handling
	go func() {
		for {
			select {
			case <-signals:
				log.Println("Exiting...")
				cancel() // close properly monitor mode
				os.Exit(0)
			case <-onUnplugged:
				log.Println("GPS device has been unplugged, exiting...")
				cancel() // close properly monitor mode
				os.Exit(1)
			case msg := <-queue:
				if msg == nil {
					continue
				}
				log.Printf("Handle NMEA message: %s", msg.Serialize())
				processMessage(msg)
				// log.Println(state.String())
			case err := <-errors:
				log.Println(err)
				/*if !gpsDev.StillExists() {
					log.Println("GPS Device deconnected")
					loopMonitor = false
					quit <- struct{}{}
				}*/
			}
		}
	}()

	// Run web server to provided GPS informations over an api
	s := &http.Server{Addr: conf.ListenAddr}
	router()
	log.Println("Listening on", conf.ListenAddr)
	if err := s.ListenAndServe(); err != nil {
		log.Fatal(err)
	}

}
