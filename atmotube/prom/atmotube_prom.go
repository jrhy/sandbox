//go:build linux
// +build linux

package prom

import (
	"github.com/jrhy/sandbox/atmotube"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	temperature_c = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_temperature_celsius",
	})
	humidity_pct = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_humidity_pct",
	})
	pressure_mbar = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_pressure_mbar",
	})
	pm1 = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_pm1",
		Help: "The number of ≤1µ particles per 100mL",
	})
	pm25 = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_pm2_5",
		Help: "The number of ≤2.5µ particles per 100mL",
	})
	pm4 = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_pm4",
		Help: "The number of ≤4µ particles per 100mL",
	})
	pm10 = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_pm10",
		Help: "The number of ≤10µ particles per 100mL",
	})
	voc = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "atmotube_voc_ppm",
	})
)

func MetricVOC(a atmotube.SGPC3) {
	voc.Set(a.VOCPPM)
}

func MetricPM(a atmotube.PM) {
	if a.PM10 >= 167772.0 {
		return
	}
	pm1.Set(a.PM1)
	pm25.Set(a.PM25)
	pm4.Set(a.PM4)
	pm10.Set(a.PM10)
}

func MetricEnv(a atmotube.BME280) {
	humidity_pct.Set(a.HumidityPercent)
	temperature_c.Set(a.TemperatureCelsius)
	pressure_mbar.Set(a.PressureMBar)
}
