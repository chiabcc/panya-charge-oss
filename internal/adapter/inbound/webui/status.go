package webui

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/chiabcc/panya-charge-oss/pkg/csms"
)

// StatusProvider provides runtime status data for the read-only status page.
// The csms.Facade satisfies this interface.
type StatusProvider interface {
	Chargers() []csms.ChargerInfo
	MQTTStatus() (connected bool, broker string)
	ChargingState() csms.ChargingState
}

type inputTopics struct {
	GridPower        string
	SolarPower       string
	ConsumptionPower string
}

type statusPageData struct {
	IngressPath   string
	StatusOnly    bool
	MQTTConnected bool
	MQTTBroker    string
	Chargers      []chargerStatus
	Charging      csms.ChargingState
	OCPPPort      int
	OCPPPath      string
	InputTopics   inputTopics
}

type statusResponse struct {
	MQTT     mqttStatus      `json:"mqtt"`
	Chargers []chargerStatus `json:"chargers"`
	Charging csms.ChargingState `json:"charging"`
	OCPP     ocppInfo        `json:"ocpp"`
}

type mqttStatus struct {
	Connected bool   `json:"connected"`
	Broker    string `json:"broker"`
}

type chargerStatus struct {
	ID            string  `json:"id"`
	Vendor        string  `json:"vendor"`
	Model         string  `json:"model"`
	Status        string  `json:"status"`
	ConnectorID   int     `json:"connector_id"`
	ChargingPower float64 `json:"charging_power"`
	LimitAmps     int     `json:"limit_amps"`
}

func (c chargerStatus) ChargerPowerKW() string {
	return formatPower(c.ChargingPower)
}

type ocppInfo struct {
	Port int    `json:"port"`
	Path string `json:"path"`
}

// WithStatus enables the /status and /api/status routes.
// Must be called before Start().
func (s *Server) WithStatus(provider StatusProvider, ocppPort int, ocppPath string) {
	s.statusProvider = provider
	s.ocppPort = ocppPort
	s.ocppPath = ocppPath
	s.mux.HandleFunc("GET /status", s.handleStatus)
	s.mux.HandleFunc("GET /api/status", s.handleStatusJSON)
}

// WithStatusOnly marks this server as status-only (no config editor).
// GET / redirects to /status so HA ingress lands on the status page.
// Must be called before Start().
func (s *Server) WithStatusOnly() {
	s.statusOnly = true
}

// SetInputTopics sets the MQTT input topic paths shown on the status page.
func (s *Server) SetInputTopics(grid, solar, consumption string) {
	s.inputTopics = inputTopics{
		GridPower:        grid,
		SolarPower:       solar,
		ConsumptionPower: consumption,
	}
}

func toChargerStatus(info []csms.ChargerInfo) []chargerStatus {
	out := make([]chargerStatus, len(info))
	for i, c := range info {
		out[i] = chargerStatus{
			ID:            c.ID,
			Vendor:        c.Vendor,
			Model:         c.Model,
			Status:        c.Status,
			ConnectorID:   c.ConnectorID,
			ChargingPower: c.ChargingPower,
			LimitAmps:     c.LimitAmps,
		}
	}
	return out
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if s.statusProvider == nil {
		http.Error(w, "status unavailable", http.StatusInternalServerError)
		return
	}

	connected, broker := s.statusProvider.MQTTStatus()
	state := s.statusProvider.ChargingState()
	chargers := s.statusProvider.Chargers()

	data := statusPageData{
		IngressPath:   r.Header.Get("X-Ingress-Path"),
		StatusOnly:    s.statusOnly,
		MQTTConnected: connected,
		MQTTBroker:    broker,
		Chargers:      toChargerStatus(chargers),
		Charging:      state,
		OCPPPort:      s.ocppPort,
		OCPPPath:      s.ocppPath,
		InputTopics:   s.inputTopics,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := s.template.ExecuteTemplate(w, "status.html", data); err != nil {
		slog.Error("render status.html template", "error", err)
	}
}

func (s *Server) handleStatusJSON(w http.ResponseWriter, r *http.Request) {
	if s.statusProvider == nil {
		http.Error(w, "status unavailable", http.StatusInternalServerError)
		return
	}

	connected, broker := s.statusProvider.MQTTStatus()
	state := s.statusProvider.ChargingState()
	chargers := s.statusProvider.Chargers()

	resp := statusResponse{
		MQTT: mqttStatus{
			Connected: connected,
			Broker:    broker,
		},
		Chargers: toChargerStatus(chargers),
		Charging: state,
		OCPP: ocppInfo{
			Port: s.ocppPort,
			Path: s.ocppPath,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("encode status JSON", "error", err)
	}
}

func formatPower(watts float64) string {
	kw := watts / 1000.0
	return strconv.FormatFloat(kw, 'f', 1, 64) + " kW"
}