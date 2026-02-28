//go:build linux
// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"tinygo.org/x/bluetooth"

	"github.com/jrhy/sandbox/atmotube"
	"github.com/jrhy/sandbox/atmotube/prom"
)

var adapter = bluetooth.DefaultAdapter
var sampleTime time.Time
var sampleMutex sync.Mutex

func deadman() {
	for {
		lastDeadman := time.Now()
		time.Sleep(2 * time.Minute)
		sampleMutex.Lock()
		if sampleTime.Before(lastDeadman) {
			panic("deadman")
		}
		sampleMutex.Unlock()
	}
}

func main() {

	println("starting prometheus metrics on :2112")
	http.Handle("/metrics", promhttp.Handler())
	go http.ListenAndServe(":2112", nil)

	go deadman()

	// Enable BLE interface.
	must("enable BLE stack", adapter.Enable())

	// Start scanning.
	println("scanning...")
	var addr bluetooth.Address
	var found bool
	err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		println("found device:", device.Address.MAC.String(), device.RSSI, device.LocalName())
		if strings.Contains(device.LocalName(), "ATMO") {
			addr = device.Address
			found = true
			adapter.StopScan()
			return
		}
		//_, err := bluetooth.ParseMAC(device.Address.String())
		//must(device.Address.String(), err)
	})
	must("start scan", err)
	if !found {
		panic("no atmo found")
	}

	adapter.SetConnectHandler(func(device bluetooth.Device, connected bool) {
		fmt.Printf("connected=%v device=%v\n", connected, device.Address.MAC.String())
	})

	fmt.Printf("connecting to %v\n", addr)
	device, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	must("connect", err)
	//uuid, err := bluetooth.ParseUUID("DB450001-8E9A-4818-ADD7-6ED94A328AB4")
	// atmotube pro
	svcuuid := mustParseUUID("db450001-8e9a-4818-add7-6ed94a328ab4")

	var chars []bluetooth.DeviceCharacteristic
	fmt.Printf("discover services %v...\n", svcuuid)
	services, err := device.DiscoverServices([]bluetooth.UUID{svcuuid})
	must("discover services", err)
	service := services[0]
	fmt.Printf("service: %+v\n", service)
	chars, err = service.DiscoverCharacteristics([]bluetooth.UUID{
		mustParseUUID("db450002-8e9a-4818-add7-6ed94a328ab4"),
		mustParseUUID("db450003-8e9a-4818-add7-6ed94a328ab4"),
		mustParseUUID("db450004-8e9a-4818-add7-6ed94a328ab4"),
		mustParseUUID("db450005-8e9a-4818-add7-6ed94a328ab4"),
	})
	if err != nil {
		panic(err)
	}
	for _, char := range chars {
		fmt.Printf("char: %+v ", char)
		switch char.String() {
		case "db450002-8e9a-4818-add7-6ed94a328ab4":
			fmt.Printf(" sgpc3\n")
			registerPM("sgpc3", char, func(b []byte) (interface{}, error) {
				d, err := atmotube.DecodeSGPC3(b)
				if err == nil {
					prom.MetricVOC(d)
				}
				return d, err
			})
		case "db450004-8e9a-4818-add7-6ed94a328ab4":
			fmt.Printf(" status\n")
			registerPM("status", char, func(b []byte) (interface{}, error) {
				return atmotube.DecodeStatus(b)
			})
		case "db450003-8e9a-4818-add7-6ed94a328ab4":
			fmt.Printf(" bme280\n")
			registerPM("bme280", char, func(b []byte) (interface{}, error) {
				d, err := atmotube.DecodeBME280(b)
				if err == nil {
					prom.MetricEnv(d)
				}
				return d, err
			})
		case "db450005-8e9a-4818-add7-6ed94a328ab4":
			fmt.Printf(" pm\n")
			registerPM("pm", char, func(b []byte) (interface{}, error) {
				d, err := atmotube.DecodePM(b)
				if err == nil {
					prom.MetricPM(d)
				}
				return d, err
			})
		default:
			fmt.Printf("???\n")
		}
	}
	must("discover", err)
	sigint := make(chan os.Signal, 1)
	signal.Notify(sigint, os.Interrupt)
	<-sigint
	device.Disconnect()
}

func registerPM(name string, char bluetooth.DeviceCharacteristic, f func([]byte) (interface{}, error)) {
	must("enable "+name+" notifications",
		char.EnableNotifications(func(buf []byte) {
			v, err := f(buf)
			if err != nil {
				log.Printf("decode %s: %v", name, err)
			} else {
				logData(name, v)
			}
		}))
}

func logData(name string, value interface{}) {
	sampleMutex.Lock()
	sampleTime = time.Now()
	sampleMutex.Unlock()
	type Record struct {
		Timestamp time.Time   `json:"timestamp"`
		Type      string      `json:"type"`
		Data      interface{} `json:"data"`
	}
	r := Record{
		Timestamp: time.Now(),
		Type:      name,
		Data:      value,
	}
	b, err := json.Marshal(r)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(b))
}

func must(action string, err error) {
	if err != nil {
		panic("failed to " + action + ": " + err.Error())
	}
}

func mustParseUUID(s string) bluetooth.UUID {
	uuid, err := bluetooth.ParseUUID(s)
	if err != nil {
		panic(fmt.Errorf("parse uuid: %s: %w", s, err))
	}
	return uuid
}
