package csmsfactory

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNew_RejectsMinAmpsBelow6(t *testing.T) {
	_, err := New(Config{
		Charging: ChargingConfig{MinAmps: 3, MaxAmps: 32},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "min_amps")
}

func TestNew_MinAmpsZeroDoesNotError(t *testing.T) {
	_, err := New(Config{
		Charging: ChargingConfig{MinAmps: 0, MaxAmps: 32},
		MQTT:     MQTTConfig{Broker: "tcp://invalid:1"},
	})
	if err != nil {
		assert.NotContains(t, err.Error(), "min_amps")
	}
}

func TestNew_RejectsMaxAmpsAbove32(t *testing.T) {
	_, err := New(Config{
		Charging: ChargingConfig{MinAmps: 6, MaxAmps: 48},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "max_amps")
}

func TestNew_AppliesDefaultsBeforeConnection(t *testing.T) {
	_, err := New(Config{
		MQTT: MQTTConfig{Broker: "tcp://invalid:1"},
	})
	if err != nil {
		assert.NotContains(t, err.Error(), "min_amps")
		assert.NotContains(t, err.Error(), "max_amps")
	}
}
