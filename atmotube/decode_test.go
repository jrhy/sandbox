package atmotube_test

import (
	"testing"

	"github.com/jrhy/sandbox/atmotube"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeSGPC3(t *testing.T) {
	b := []byte{0x94, 0x02}
	actual, err := atmotube.DecodeSGPC3(b)
	require.NoError(t, err)
	assert.Equal(t, atmotube.SGPC3{
		VOCPPM: 0.66,
	}, actual)
}

func TestDecodeBME280(t *testing.T) {
	b := []byte{0x1e, 0x1c, 0xe0, 0x82, 0x01, 0x00, 0x0a, 0x0a}
	actual, err := atmotube.DecodeBME280(b)
	require.NoError(t, err)
	assert.Equal(t, atmotube.BME280{
		HumidityPercent:    30.0,
		TemperatureCelsius: 25.7,
		PressureMBar:       990.40,
	}, actual)
}

func TestDecodePM(t *testing.T) {
	b := []byte{
		0xad, 0x03, 0x00,
		0xad, 0x03, 0x00,
		0xad, 0x03, 0x00,
		0xad, 0x03, 0x00,
	}
	actual, err := atmotube.DecodePM(b)
	require.NoError(t, err)
	assert.Equal(t, atmotube.PM{
		PM1:  9.41,
		PM25: 9.41,
		PM10: 9.41,
		PM4:  9.41,
	}, actual)
}
