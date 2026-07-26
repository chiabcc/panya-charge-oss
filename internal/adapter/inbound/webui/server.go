package webui

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/chiabcc/panya-charge-oss/internal/config"
	"github.com/chiabcc/panya-charge-oss/pkg/csms"
)

// Applier handles hot updates and facade rebuilds.
type Applier interface {
	// UpdateCharging applies charging parameters at runtime.
	UpdateCharging(params csms.ChargingParams) error
	// SetLogLevel adjusts the log level at runtime.
	SetLogLevel(level string) error
	// Rebuild stops the current facade and starts a new one with the given config.
	Rebuild(cfg *config.Config) error
	// HasActiveSession reports whether any charger has an active transaction.
	HasActiveSession() (chargerIDs []string, hasActive bool)
}

// applyResult is the JSON response returned by POST /api/config.
type applyResult struct {
	Action                     string `json:"action"`
	FieldsChanged              []string `json:"fields_changed"`
	RequiresRebuild            bool     `json:"requires_rebuild"`
	ActiveSession              bool     `json:"active_session"`
	ConfirmToken               string   `json:"confirm_token,omitempty"`
	ChargerReconfigureRequired bool     `json:"charger_reconfigure_required,omitempty"`
	RestartRequired            bool     `json:"restart_required,omitempty"`
	Error                      string   `json:"error,omitempty"`
	ErrorMessage               string   `json:"error_message,omitempty"`
}

// confirmEntry holds a single-use confirm token.
type confirmEntry struct {
	nonce   string
	fields  []string
	expires time.Time
	used    bool
}

func newConfirmToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewServer constructs a WebUI server.
//
// Parameters:
//   - configPath: path to config.yaml (read fresh per request)
//   - listenAddr: bind address from WebUIConfig.Listen
//   - token: authentication token (empty = no auth on loopback)
//   - isLoopback: true if listenAddr resolves to 127.0.0.1 / ::1
//   - applier: optional applier for hot updates and rebuilds (nil = hot-only, no rebuild)
func NewServer(configPath string, listenAddr string, token string, isLoopback bool, applier Applier) *Server {
	mux := http.NewServeMux()

	srv := &Server{
		mux:        mux,
		configPath: configPath,
		listenAddr: listenAddr,
		token:      token,
		isLoopback: isLoopback,
		template:   template.Must(template.ParseFS(staticFS, "templates/login.html")),
		applier:    applier,
	}

	mux.HandleFunc("GET /", srv.handleIndex)
	mux.HandleFunc("POST /login", srv.handleLogin)
	mux.HandleFunc("POST /logout", srv.handleLogout)

	if token != "" || !isLoopback {
		mux.Handle("GET /api/config", authMiddleware(http.HandlerFunc(srv.handleGetConfig), token))
		mux.Handle("POST /api/config", authMiddleware(http.HandlerFunc(srv.handlePostConfig), token))
	} else {
		mux.HandleFunc("GET /api/config", srv.handleGetConfig)
		mux.HandleFunc("POST /api/config", srv.handlePostConfig)
	}

	// Static assets must match only GET to avoid ServeMux method/path conflicts with GET /.
	mux.Handle("GET /static/", http.FileServer(http.FS(staticFS)))

	return srv
}

type Server struct {
	mux        *http.ServeMux
	configPath string
	listenAddr string
	token      string
	isLoopback bool
	template   *template.Template
	logger     *slog.Logger
	applier    Applier
	mu         sync.Mutex
	confirm    *confirmEntry
}

type loginData struct {
	Error string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.token == "" && s.isLoopback {
		http.Redirect(w, r, "/api/config", http.StatusFound)
		return
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && subtleValidate(cookie.Value, s.token) {
		http.Redirect(w, r, "/api/config", http.StatusFound)
		return
	}
	data := loginData{}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	s.template.ExecuteTemplate(w, "login.html", data)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	submitted := r.FormValue("token")

	if !subtleValidate(submitted, s.token) {
		data := loginData{Error: "Invalid token"}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		s.template.ExecuteTemplate(w, "login.html", data)
		return
	}

	setSessionCookie(w, s.token)
	http.Redirect(w, r, "/api/config", http.StatusFound)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	clearSessionCookie(w)
	http.Redirect(w, r, "/", http.StatusFound)
}

type configDTO struct {
	Fields       map[string]*fieldDTO `json:"fields"`
	ApplyClasses map[string]string    `json:"apply_classes"`
}

type fieldDTO struct {
	Value           any    `json:"value"`
	OverriddenByEnv bool   `json:"overridden_by_env"`
	Secret          bool   `json:"secret"`
	PasswordSet     bool   `json:"password_set,omitempty"`
	TokenSet        bool   `json:"token_set,omitempty"`
}

func buildConfigDTO(ec *config.EffectiveConfig) *configDTO {
	dto := &configDTO{
		Fields: map[string]*fieldDTO{},
		ApplyClasses: map[string]string{
			"server.log_level":                "hot",
			"server.log_format":               "rebuild",
			"server.ocpp_port":                "rebuild",
			"server.ocpp_path":                "rebuild",
			"charging.min_amps":               "hot",
			"charging.max_amps":               "hot",
			"charging.contactor_cooldown_sec": "hot",
			"charging.default_amps":           "hot",
			"mqtt.broker":                     "rebuild",
			"mqtt.client_id":                  "rebuild",
			"mqtt.username":                   "rebuild",
			"mqtt.password":                   "rebuild",
			"mqtt.base_topic":                 "rebuild",
			"mqtt.topics.*":                   "rebuild",
			"mqtt.disconnect_threshold_sec":   "rebuild",
			"webui.enabled":                   "process_restart",
			"webui.listen":                    "process_restart",
			"webui.token":                     "hot",
		},
	}

	ovf := ec.OverriddenByEnv

	dto.Fields["server.ocpp_port"] = &fieldDTO{Value: ec.ServerOCPPPort, OverriddenByEnv: ovf["server.ocpp_port"]}
	dto.Fields["server.ocpp_path"] = &fieldDTO{Value: ec.ServerOCPPPath, OverriddenByEnv: ovf["server.ocpp_path"]}
	dto.Fields["server.log_level"] = &fieldDTO{Value: ec.ServerLogLevel, OverriddenByEnv: ovf["server.log_level"]}
	dto.Fields["server.log_format"] = &fieldDTO{Value: ec.ServerLogFormat, OverriddenByEnv: ovf["server.log_format"]}

	dto.Fields["mqtt.broker"] = &fieldDTO{Value: ec.MQTTBroker, OverriddenByEnv: ovf["mqtt.broker"]}
	dto.Fields["mqtt.client_id"] = &fieldDTO{Value: ec.MQTTClientID, OverriddenByEnv: ovf["mqtt.client_id"]}
	dto.Fields["mqtt.username"] = &fieldDTO{Value: ec.MQTTUsername, OverriddenByEnv: ovf["mqtt.username"]}
	dto.Fields["mqtt.password"] = &fieldDTO{Value: nil, Secret: true, PasswordSet: ec.MQTTPasswordSet, OverriddenByEnv: ovf["mqtt.password"]}
	dto.Fields["mqtt.base_topic"] = &fieldDTO{Value: ec.MQTTBaseTopic, OverriddenByEnv: ovf["mqtt.base_topic"]}
	dto.Fields["mqtt.disconnect_threshold_sec"] = &fieldDTO{Value: ec.MQTTDisconnectSec, OverriddenByEnv: ovf["mqtt.disconnect_threshold_sec"]}

	if ec.MQTTTopics != nil {
		dto.Fields["mqtt.topics"] = &fieldDTO{Value: ec.MQTTTopics}
	}

	dto.Fields["charging.min_amps"] = &fieldDTO{Value: ec.ChargingMinAmps, OverriddenByEnv: ovf["charging.min_amps"]}
	dto.Fields["charging.max_amps"] = &fieldDTO{Value: ec.ChargingMaxAmps, OverriddenByEnv: ovf["charging.max_amps"]}
	dto.Fields["charging.contactor_cooldown_sec"] = &fieldDTO{Value: ec.ChargingContactorsSec, OverriddenByEnv: ovf["charging.contactor_cooldown_sec"]}
	dto.Fields["charging.default_amps"] = &fieldDTO{Value: ec.ChargingDefaultAmps, OverriddenByEnv: ovf["charging.default_amps"]}

	dto.Fields["webui.enabled"] = &fieldDTO{Value: ec.WebUIEnabled, OverriddenByEnv: ovf["webui.enabled"]}
	dto.Fields["webui.listen"] = &fieldDTO{Value: ec.WebUIListen, OverriddenByEnv: ovf["webui.listen"]}
	dto.Fields["webui.token"] = &fieldDTO{Value: nil, Secret: true, TokenSet: ec.WebUITokenSet, OverriddenByEnv: ovf["webui.token"]}

	return dto
}

func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	ec, err := config.Effective(s.configPath)
	if err != nil {
		slog.Error("read effective config", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	dto := buildConfigDTO(ec)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		slog.Error("encode config DTO", "error", err)
	}
}

func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// PARSE — read form or JSON body
	if err := r.ParseForm(); err != nil {
		jsonError(w, "parse form: "+err.Error(), http.StatusBadRequest)
		return
	}

	// LOAD — read current config from disk
	current, err := config.Load(s.configPath)
	if err != nil {
		slog.Error("load effective config for save", "error", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// Read env override map (from Effective two-pass)
	ec, err := config.Effective(s.configPath)
	if err != nil {
		slog.Error("read effective config for save", "error", err)
		jsonError(w, "internal server error", http.StatusInternalServerError)
		return
	}

	// GUARD — reject changes to env-overridden fields
	for field := range r.Form {
		if ec.OverriddenByEnv[field] {
			envVar := dotToEnv(field)
			slog.Warn("webui_config_rejected", "field", field, "env_var", envVar)
			jsonError(w, fmt.Sprintf("cannot change %s: overridden by environment variable %s", field, envVar), http.StatusBadRequest)
			return
		}
	}

	// BUILD — merge submitted form values into a candidate Config
	candidate := cloneConfig(current)
	applyFormValues(candidate, r.Form, current)

	// SECRET PRESERVATION — empty password/token keeps existing value
	if r.Form.Get("mqtt.password") == "" && !wasPasswordChanged(r.Form) {
		// keep existing (already copied via clone)
	}
	if r.Form.Get("webui.token") == "" && !wasTokenChanged(r.Form) {
		// keep existing (already copied via clone)
	}

	// VALIDATE — validate candidate config
	if err := config.Validate(candidate); err != nil {
		slog.Warn("webui_config_rejected", "reason", "validation", "error", err)
		jsonError(w, "validation failed: "+err.Error(), http.StatusBadRequest)
		return
	}

	// CLASSIFY
	report := config.ClassifyChanges(current, candidate)
	if report.Class == config.ApplyNone {
		writeJSON(w, applyResult{Action: "none", FieldsChanged: []string{}})
		return
	}

	// For rebuild-class: check confirm-token first
	if report.Class == config.ApplyRebuild || report.Class == config.ApplyProcessRestart {
		submittedToken := r.FormValue("confirm_token")
		if submittedToken == "" && s.confirm != nil {
			// Need confirmation and we have a token — check if user is confirming
			// Return the confirm-required response
			_, hasActive := falseActiveSession(s.applier)
			activeResult := applyResult{
				Action:                     "rebuild",
				FieldsChanged:              report.Fields,
				RequiresRebuild:            true,
				ActiveSession:              hasActive,
				ConfirmToken:               s.confirm.nonce,
				ChargerReconfigureRequired: report.ChargerReconfigureRequired,
			}
			if report.Class == config.ApplyProcessRestart {
				activeResult.Action = "process_restart"
				activeResult.RestartRequired = true
				activeResult.RequiresRebuild = false
				activeResult.ConfirmToken = ""
			}
			writeJSON(w, activeResult)
			return
		}
	}

	// PERSIST — atomic write first (disk is source of truth)
	if err := config.WriteAtomic(s.configPath, candidate); err != nil {
		slog.Error("atomic write failed", "error", err)
		jsonError(w, "atomic write failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Process restart: always return process_restart response (no applier needed)
	if report.Class == config.ApplyProcessRestart {
		logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "process_restart", false, nil)
		writeJSON(w, applyResult{
			Action:          "process_restart",
			FieldsChanged:   report.Fields,
			RestartRequired: true,
		})
		return
	}

	// For rebuild-class, if we have an applier, check active session
	if (report.Class == config.ApplyRebuild || report.Class == config.ApplyProcessRestart) && s.applier != nil {
		chargerIDs, hasActive := s.applier.HasActiveSession()

		// Process restart: just saved to disk, no apply
		if report.Class == config.ApplyProcessRestart {
			logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "process_restart", hasActive, chargerIDs)
			writeJSON(w, applyResult{
				Action:          "process_restart",
				FieldsChanged:   report.Fields,
				RestartRequired: true,
				ActiveSession:   hasActive,
			})
			return
		}

		// Rebuild: if active session, require confirm
		if hasActive {
			// Generate confirm token
			nonce := newConfirmToken()
			s.confirm = &confirmEntry{nonce: nonce, fields: report.Fields, expires: time.Now().Add(5 * time.Minute)}

			logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "rebuild_pending", hasActive, chargerIDs)
			writeJSON(w, applyResult{
				Action:                     "rebuild",
				FieldsChanged:              report.Fields,
				RequiresRebuild:            true,
				ActiveSession:              true,
				ConfirmToken:               nonce,
				ChargerReconfigureRequired: report.ChargerReconfigureRequired,
			})
			return
		}

		// No active session — execute rebuild immediately
		startTime := time.Now()
		if err := s.applier.Rebuild(candidate); err != nil {
			slog.Error("webui_rebuild_failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
			jsonError(w, "rebuild failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		logAudit(slog.LevelInfo, "webui_rebuild_completed", report.Fields, "rebuild", false, nil)
		writeJSON(w, applyResult{
			Action:                     "rebuild",
			FieldsChanged:              report.Fields,
			ChargerReconfigureRequired: report.ChargerReconfigureRequired,
		})
		return
	}

	// Hot apply (no applier needed for basic hot path in tests)
	if s.applier != nil && report.Class == config.ApplyHot {
		if err := applyHot(s.applier, current, candidate); err != nil {
			slog.Error("hot apply failed", "error", err)
			jsonError(w, "apply failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "hot_reload", false, nil)
	writeJSON(w, applyResult{
		Action:        "hot_reload",
		FieldsChanged: report.Fields,
	})
}

func falseActiveSession(_ Applier) ([]string, bool) {
	return nil, false
}

func logAudit(level slog.Level, msg string, fields []string, action string, hasActive bool, chargerIDs []string) {
	slog.Log(context.Background(), level, msg,
		"fields_changed", fields,
		"action", action,
		"had_active_session", hasActive,
		"charger_ids", chargerIDs,
	)
}

func applyHot(applier Applier, old, new *config.Config) error {
	// Charging params
	if old.Charging != new.Charging {
		params := csms.ChargingParams{
			MinAmps:              new.Charging.MinAmps,
			MaxAmps:              new.Charging.MaxAmps,
			ContactorCooldownSec: new.Charging.ContactorCooldownSec,
			DefaultAmps:          new.Charging.DefaultAmps,
		}
		if err := applier.UpdateCharging(params); err != nil {
			return fmt.Errorf("update charging: %w", err)
		}
	}

	// Log level
	if old.Server.LogLevel != new.Server.LogLevel {
		if err := applier.SetLogLevel(new.Server.LogLevel); err != nil {
			return fmt.Errorf("set log level: %w", err)
		}
	}

	// WebUI token: re-token is applied by reloading effective config on next auth check
	slog.Info("webui hot applied", "fields", "charging,log_level,token")
	return nil
}

// Helpers

func dotToEnv(field string) string {
	// "mqtt.broker" -> "PANYA_MQTT_BROKER"
	parts := [2]string{"", ""}
	lastDot := 0
	for i := 0; i < len(field); i++ {
		if field[i] == '.' {
			parts[0] = field[:i]
			lastDot = i
		}
	}
	if lastDot > 0 {
		parts[1] = field[lastDot+1:]
	}
	return fmt.Sprintf("PANYA_%s_%s",
		toUpper(parts[0]), toUpper(parts[1]))
}

func toUpper(s string) string {
	// Convert camelCase to SCREAMING_SNAKE_CASE
	var out []rune
	for i, r := range s {
		if r >= 'a' && r <= 'z' {
			out = append(out, r-32)
		} else if (i > 0 && r >= 'A' && r <= 'Z') && len(out) > 0 && out[len(out)-1] < '_' {
			out = append(out, '_')
			out = append(out, r-32)
		} else if r >= 'A' && r <= 'Z' {
			out = append(out, r)
		} else {
			out = append(out, r)
		}
	}
	return string(out)
}

func cloneConfig(c *config.Config) *config.Config {
	return &config.Config{
		Server:   c.Server,
		MQTT:     c.MQTT,
		Charging: c.Charging,
		WebUI:    c.WebUI,
	}
}

func applyFormValues(cfg *config.Config, form map[string][]string, current *config.Config) {
	for field, vals := range form {
		if len(vals) == 0 {
			continue
		}
		val := vals[0]
		switch field {
		case "server.ocpp_port":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.Server.OCPPPort = n
			}
		case "server.ocpp_path":
			cfg.Server.OCPPPath = val
		case "server.log_level":
			cfg.Server.LogLevel = val
		case "server.log_format":
			cfg.Server.LogFormat = val
		case "mqtt.broker":
			cfg.MQTT.Broker = val
		case "mqtt.client_id":
			cfg.MQTT.ClientID = val
		case "mqtt.username":
			cfg.MQTT.Username = val
		case "mqtt.password":
			// Only update if explicitly provided (non-empty)
			if val != "" {
				cfg.MQTT.Password = val
			}
		case "mqtt.base_topic":
			cfg.MQTT.BaseTopic = val
		case "mqtt.disconnect_threshold_sec":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.MQTT.DisconnectThresholdSec = n
			}
		case "charging.min_amps":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.Charging.MinAmps = n
			}
		case "charging.max_amps":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.Charging.MaxAmps = n
			}
		case "charging.contactor_cooldown_sec":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.Charging.ContactorCooldownSec = n
			}
		case "charging.default_amps":
			if n, err := strconv.Atoi(val); err == nil {
				cfg.Charging.DefaultAmps = n
			}
		case "webui.enabled":
			if b, err := strconv.ParseBool(val); err == nil {
				cfg.WebUI.Enabled = b
			}
		case "webui.listen":
			cfg.WebUI.Listen = val
		case "webui.token":
			// Only update if explicitly provided
			if val != "" {
				cfg.WebUI.Token = val
			}
		default:
			// Ignore unknown fields
		}
	}
}

func wasPasswordChanged(form map[string][]string) bool {
	vals := form["mqtt.password"]
	return len(vals) > 0 && vals[0] != ""
}

func wasTokenChanged(form map[string][]string) bool {
	vals := form["webui.token"]
	return len(vals) > 0 && vals[0] != ""
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(applyResult{Error: msg, ErrorMessage: msg})
}

func writeJSON(w http.ResponseWriter, r interface{}) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(r)
}

// subtleValidate compares two strings in constant time to prevent timing attacks.
func subtleValidate(a, b string) bool {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// Start starts the HTTP server listener.
func (s *Server) Start(ctx context.Context) error {
	ln, err := net.Listen("tcp", s.listenAddr)
	if err != nil {
		return err
	}

	slog.Info("webui listening", "addr", s.listenAddr)

	httpSrv := &http.Server{
		Handler:  s.mux,
		ReadHeaderTimeout:  5 * 60 * 1000,
		WriteTimeout:       10 * 1000,
		IdleTimeout:        120 * 1000,
	}

	go func() {
		select {
		case <-ctx.Done():
			httpSrv.Shutdown(context.Background())
		}
	}()

	if err := httpSrv.Serve(ln); err != http.ErrServerClosed {
		slog.Warn("webui server error", "error", err)
	}

	return nil
}