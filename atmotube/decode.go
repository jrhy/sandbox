package atmotube

import (
	"bytes"
	"encoding/binary"
)

type BME280 struct {
	HumidityPercent    float64
	TemperatureCelsius float64
	PressureMBar       float64
}

type BME280Wire struct {
	HumidityPercent              byte
	TemperatureCelsius           byte
	PressureMBarHundredths       uint32
	TemperatureCelsiusHundredths uint16
}

func DecodeBME280(b []byte) (BME280, error) {
	var r BME280Wire
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &r)
	return BME280{
		HumidityPercent:    float64(r.HumidityPercent),
		PressureMBar:       float64(r.PressureMBarHundredths) / 100.0,
		TemperatureCelsius: float64(r.TemperatureCelsiusHundredths) / 100.0,
	}, err
}

type SGPC3 struct {
	VOCPPM float64
}

type SGPC3Wire struct {
	VOCPPB uint16
}

func DecodeSGPC3(b []byte) (SGPC3, error) {
	var r SGPC3Wire
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &r)
	return SGPC3{
		VOCPPM: float64(r.VOCPPB) / 1000.0,
	}, err
}

type PM struct {
	PM1  float64
	PM25 float64
	PM10 float64
	PM4  float64
}

type Uint24 struct{ A, B, C byte }

func (u Uint24) ToFloat() float64 {
	return float64(uint32(u.C)<<16 | uint32(u.B)<<8 | uint32(u.A))
}

type PMWire struct {
	PM1Hundredths  Uint24
	PM25Hundredths Uint24
	PM10Hundredths Uint24
	PM4Hundredths  Uint24
}

func DecodePM(b []byte) (PM, error) {
	var r PMWire
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &r)
	return PM{
		PM1:  r.PM1Hundredths.ToFloat() / 100.0,
		PM25: r.PM25Hundredths.ToFloat() / 100.0,
		PM10: r.PM10Hundredths.ToFloat() / 100.0,
		PM4:  r.PM4Hundredths.ToFloat() / 100.0,
	}, err
}

type Status struct {
	SGPC3Heating     bool
	ChargingRecently bool
	ChargingNow      bool
	DeviceBonded     bool
	DeviceErrored    bool
	PMSensorOn       bool
	BatteryPercent   byte
}

type StatusWire struct {
	Info    byte
	Battery byte
}

func DecodeStatus(b []byte) (Status, error) {
	var r StatusWire
	err := binary.Read(bytes.NewReader(b), binary.LittleEndian, &r)
	return Status{
		SGPC3Heating:     r.Info&0x40 == 0,
		ChargingRecently: r.Info&0x10 != 0,
		ChargingNow:      r.Info&0x08 != 0,
		DeviceBonded:     r.Info&0x04 != 0,
		DeviceErrored:    r.Info&0x02 != 0,
		PMSensorOn:       r.Info&0x01 != 0,
		BatteryPercent:   r.Battery,
	}, err
}
