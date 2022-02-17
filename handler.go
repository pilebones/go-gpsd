package main

import (
	"encoding/json"
	"log"
	"net/http"
)

func router() {
	http.HandleFunc("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { GPSHandler(w, r) }))
}

// GPSHandler http handler to provide GPS data
func GPSHandler(resp http.ResponseWriter, req *http.Request) {
	data, err := json.Marshal(state)
	if err != nil {
		log.Println("Unable to serialize response data, err:", err.Error())
		resp.WriteHeader(http.StatusInternalServerError)
		_, _ = resp.Write([]byte("Unable to serialize response data"))
		return
	}

	resp.WriteHeader(http.StatusOK)
	_, _ = resp.Write(data)
}
