package session

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSession_IsActive(t *testing.T) {
	tests := []struct {
		name      string
		stoppedAt *time.Time
		want      bool
	}{
		{"ongoing session", nil, true},
		{"stopped session", &time.Time{}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Session{StoppedAt: tt.stoppedAt}
			assert.Equal(t, tt.want, s.IsActive())
		})
	}
}

func TestSession_EnergyKWh(t *testing.T) {
	t.Run("ongoing session returns nil", func(t *testing.T) {
		s := Session{MeterStartWh: 1000, MeterStopWh: nil}
		assert.Nil(t, s.EnergyKWh())
	})

	t.Run("completed session returns kWh", func(t *testing.T) {
		stop := 5500.0
		s := Session{MeterStartWh: 1000, MeterStopWh: &stop}
		energy := s.EnergyKWh()
		require.NotNil(t, energy)
		assert.InDelta(t, 4.5, *energy, 0.001)
	})

	t.Run("zero energy session", func(t *testing.T) {
		stop := 1000.0
		s := Session{MeterStartWh: 1000, MeterStopWh: &stop}
		energy := s.EnergyKWh()
		require.NotNil(t, energy)
		assert.InDelta(t, 0.0, *energy, 0.001)
	})
}
