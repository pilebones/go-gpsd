package main

import (
	"fmt"
	"strings"
	"time"

	nmea "github.com/pilebones/go-nmea"
)

var (
	state GPSData
)

// processMessage handle NMEA message and enrich GPS state data
func processMessage(msg nmea.NMEA) {

	if msg == nil {
		return
	}

	// Update first
	state.LastUpdate = time.Now()

	switch msg.(type) {
	case *nmea.GPGGA:
		gpgga := msg.(*nmea.GPGGA)
		state.Latitude, state.Longitude = &gpgga.Latitude, &gpgga.Longitude
	case *nmea.GPGLL:
		gpgll := msg.(*nmea.GPGLL)
		state.Latitude, state.Longitude = &gpgll.Latitude, &gpgll.Longitude
	case *nmea.GPGSA:
		gpgsa := msg.(*nmea.GPGSA)
		status := gpgsa.FixStatus.String()
		state.Status = &status
	case *nmea.GPGSV:
		gpgsv := msg.(*nmea.GPGSV)
		state.NbSatellite = &(gpgsv.SatellitesInView)
	case *nmea.GPRMC:
		gprmc := msg.(*nmea.GPRMC)
		state.Speed = &gprmc.Speed
		state.Latitude, state.Longitude = &gprmc.Latitude, &gprmc.Longitude
	case *nmea.GPTXT:
		gptxt := msg.(*nmea.GPTXT)
		state.AntennaStatus = gptxt.AntennaStatus()
	case *nmea.GPVTG:
		gpvtg := msg.(*nmea.GPVTG)
		state.Speed = &gpvtg.SpeedKmh
	}
}

type GPSData struct {
	LastUpdate time.Time `json:"last_update"`

	AntennaStatus *string `json:"ant_status"`

	Status *string `json:"status"`

	Latitude  *nmea.LatLong `json:"latitude"`
	Longitude *nmea.LatLong `json:"longitude"`
	Altitude  *float64      `json:"altitude"` // ft
	Speed     *float64      `json:"speed"`    // mph
	Climb     *float64      `json:"climb"`    // ft/min

	NbSatellite *int `json:"nb_satellite"`

	LatitudeAccuracyErr  *uint `json:"latitude_accuracy_err"`
	LongitudeAccuracyErr *uint `json:"longitude_accuracy_err"`
	AltitudeAccuracyErr  *uint `json:"altitude_accuracy_err"`
	SpeedAccuracyErr     *uint `json:"speed_accuracy_err"`
	CourseAccuracyErr    *uint `json:"course_accuracy_err"`
}

func (d GPSData) String() string {

	rv := fmt.Sprintf("Time: %s\n", d.LastUpdate.Format("2006-01-02 15:04:05"))

	if d.AntennaStatus != nil {
		rv += fmt.Sprintf("Antenna status: %v\n", *d.AntennaStatus)
	}

	if d.Status != nil {
		rv += fmt.Sprintf("Status: %s\n", *d.Status)
	}

	if d.Latitude != nil {
		rv += fmt.Sprintf("Latitude: %v\n", *d.Latitude)
	}

	if d.Longitude != nil {
		rv += fmt.Sprintf("Longitude: %v\n", *d.Longitude)
	}

	if d.Altitude != nil {
		rv += fmt.Sprintf("Altitude: %v\n", *d.Altitude)
	}

	if d.Speed != nil {
		rv += fmt.Sprintf("Speed: %v\n", *d.Speed)
	}

	if d.Climb != nil {
		rv += fmt.Sprintf("Climb: %v\n", *d.Climb)
	}

	if d.LatitudeAccuracyErr != nil {
		rv += fmt.Sprintf("Latitude Err: %v\n", *d.LatitudeAccuracyErr)
	}

	if d.LongitudeAccuracyErr != nil {
		rv += fmt.Sprintf("Longitude Err: %v\n", *d.LongitudeAccuracyErr)
	}

	if d.AltitudeAccuracyErr != nil {
		rv += fmt.Sprintf("Altitude Err: %v\n", *d.AltitudeAccuracyErr)
	}

	if d.SpeedAccuracyErr != nil {
		rv += fmt.Sprintf("Speed Err: %v\n", *d.SpeedAccuracyErr)
	}

	if d.CourseAccuracyErr != nil {
		rv += fmt.Sprintf("Course Err: %v\n", *d.CourseAccuracyErr)
	}

	return strings.TrimSpace(rv)
}
