# go-gpsd [![Go Report Card](https://goreportcard.com/badge/github.com/pilebones/go-gpsd)](https://goreportcard.com/report/github.com/pilebones/go-gpsd) [![GoDoc](https://godoc.org/github.com/pilebones/go-gpsd?status.svg)](https://godoc.org/github.com/pilebones/go-gpsd) [![Build Status](https://travis-ci.org/pilebones/go-gpsd.svg?branch=master)](https://travis-ci.org/pilebones/go-gpsd)

GPSd provides a human readable and HTTP interface about GPS-device informations.

__/!\ Work in progress /!\__

Tested with this [GPS Module](http://wiki.52pi.com/index.php/USB-Port-GPS_Module_SKU:EZ-0048) cover [L80 gps protocol specification v1.0.pdf](http://wiki.52pi.com/index.php/File:L80_gps_protocol_specification_v1.0.pdf).

## Features

- Read GPS message from USB-Serial device like `/dev/ttyUSBx`.
- Auto-detect GPS device mode (hot-plug like udev or lookup existing device).
- Provide HTTP API to get GPS informations and state.

## How to

### Get sources

```
go get github.com/pilebones/go-gpsd
```

### Unit test

```
go test ./...
```

### Compile

```
go build
```

### Usage

```
./go-gpsd -help
  -autodetect
        Allow to enable auto-detection of the GPS device (already plugged or hot-plugged)
  -autodetect-timeout duration
        Time spent to try to autodetect the GPS device (exit 2 if fail) (default 5s)
  -input string
        Char device path related to the serial port of the GPS device (default "/dev/ttyUSB0")
  -target string
        HTTP listener to get GPS state (default "127.0.0.1:1234")
  -timeout duration
        Max duration allowed to read and parse a GPS sentence from serial-port (default 5s)
```

Note: you should run binary by user with elevated privileges to have access to /dev kernel struct.

### Example

To auto-detect GPS device and handle GPS message:

```
(sudo) ./go-gpsd -autodetect
```

## Throubleshooting

Don't hesitate to notice if you detect a problem with this tool or library.

## Documentation

- [GoDoc Reference](http://godoc.org/github.com/pilebones/go-gpsd).

## License

go-udev is available under the [GNU GPL v3 - Clause License](https://opensource.org/licenses/GPL-3.0).
