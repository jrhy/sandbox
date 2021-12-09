package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/jrhy/sandbox/atmotube"
	"tinygo.org/x/bluetooth"
)

var adapter = bluetooth.DefaultAdapter

func main() {
	// Enable BLE interface.
	must("enable BLE stack", adapter.Enable())

	// Start scanning.
	println("scanning...")
	var addr bluetooth.Addresser
	err := adapter.Scan(func(adapter *bluetooth.Adapter, device bluetooth.ScanResult) {
		println("found device:", device.Address.String(), device.RSSI, device.LocalName())
		if strings.Contains(device.LocalName(), "ATMO") {
			addr = device.Address
			adapter.StopScan()
			return
		}
		//_, err := bluetooth.ParseMAC(device.Address.String())
		//must(device.Address.String(), err)
	})
	must("start scan", err)
	if addr == nil {
		panic("no atmo found")
	}

	adapter.SetConnectHandler(func(device bluetooth.Addresser, connected bool) {
		fmt.Printf("connected=%v device=%v\n", connected, device)
	})

	fmt.Printf("connecting to %v\n", addr)
	device, err := adapter.Connect(addr, bluetooth.ConnectionParams{})
	must("connect", err)
	//uuid, err := bluetooth.ParseUUID("DB450001-8E9A-4818-ADD7-6ED94A328AB4")
	// atmotube pro
	svcuuid, err := bluetooth.ParseUUID("db450001-8e9a-4818-add7-6ed94a328ab4")
	must("parse svc uuid", err)
	pmchar, err := bluetooth.ParseUUID("db450005-8e9a-4818-add7-6ed94a328ab4")
	must("parse pmchar uuid", err)
	fmt.Printf("pmchar: %+v\n", pmchar)

	services, err := device.DiscoverServices([]bluetooth.UUID{svcuuid})
	must("discover services", err)
	for _, service := range services {
		fmt.Printf("service: %+v\n", service)
		chars, err := service.DiscoverCharacteristics([]bluetooth.UUID{})
		must("discover characteristics", err)
		for _, char := range chars {
			fmt.Printf("char: %+v\n", char)
			switch char.String() {
			case "db450002-8e9a-4818-add7-6ed94a328ab4":
				registerPM("sgpc3", char, func(b []byte) (interface{}, error) {
					return atmotube.DecodeSGPC3(b)
				})
			case "db450004-8e9a-4818-add7-6ed94a328ab4":
				registerPM("status", char, func(b []byte) (interface{}, error) {
					return atmotube.DecodeStatus(b)
				})
			case "db450003-8e9a-4818-add7-6ed94a328ab4":
				registerPM("bme280", char, func(b []byte) (interface{}, error) {
					return atmotube.DecodeBME280(b)
				})
			case "db450005-8e9a-4818-add7-6ed94a328ab4":
				registerPM("pm", char, func(b []byte) (interface{}, error) {
					return atmotube.DecodePM(b)
				})
			}

		}
	}
	must("discover", err)
	fmt.Printf("services: %+v\n", services)
	sigint := make(chan os.Signal)
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
