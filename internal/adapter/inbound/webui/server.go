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
	"maps"
	"net"
	"net/http"
	"strconv"
	"strings"
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

// confirmEntry holds a single-use confirm token and the pending candidate.
type confirmEntry struct {
	nonce     string
	fields    []string
	expires   time.Time
	candidate *config.Config
}

func newConfirmToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

type configFieldVM struct {
	Name         string
	Value        string
	Disabled     bool
	Secret       bool
	ApplyClass   string
	PasswordSet  bool
	TokenSet     bool
}

type configSectionVM struct {
	Title  string
	Fields []configFieldVM
}

type configFormVM struct {
	Sections map[string]*configSectionVM
	Result   template.HTML
}

type resultVM struct {
	Message                string
	FieldsChanged          []string
	ActiveSession          bool
	ChargerReconfigureRequired bool
	ConfirmToken           string
	ResultClass            string
}

func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"envBadge": func(field string) string {
			return dotToEnv(field)
		},
		"renderError": func(msg string) template.HTML {
			return template.HTML(fmt.Sprintf(`<div class="frag-error">%s</div>`,
				template.HTMLEscapeString(msg)))
		},
		"dict": func(args ...any) map[string]any {
			if len(args) == 0 || len(args)%2 != 0 {
				return nil
			}
			d := make(map[string]any, len(args)/2)
			for i := 0; i < len(args); i += 2 {
				if key, ok := args[i].(string); ok {
					d[key] = args[i+1]
				}
			}
			return d
		},
		"formatPower": func(watts float64) string {
			kw := watts / 1000.0
			return strconv.FormatFloat(kw, 'f', 1, 64) + " kW"
		},
	}
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

	tmpl, err := template.New("webui").Funcs(templateFuncs()).ParseFS(staticFS,
		"templates/login.html",
		"templates/config.html",
		"templates/fragments.html",
		"templates/status.html",
	)
	if err != nil {
		slog.Error("parse webui templates", "error", err)
		tmpl = template.Must(template.ParseFS(staticFS, "templates/login.html"))
	}

	srv := &Server{
		mux:        mux,
		configPath: configPath,
		listenAddr: listenAddr,
		token:      token,
		isLoopback: isLoopback,
		template:   tmpl,
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
	mux            *http.ServeMux
	configPath     string
	listenAddr     string
	token          string
	isLoopback     bool
	statusOnly     bool
	template       *template.Template
	applier        Applier
	statusProvider StatusProvider
	ocppPort       int
	ocppPath       string
	mu             sync.Mutex
	confirm        *confirmEntry
}

type loginData struct {
	Error string
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if s.statusOnly {
		s.handleStatus(w, r)
		return
	}
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
	if err := s.template.ExecuteTemplate(w, "login.html", data); err != nil {
		slog.Error("execute login template", "error", err)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	submitted := r.FormValue("token")

	if !subtleValidate(submitted, s.token) {
		data := loginData{Error: "Invalid token"}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.template.ExecuteTemplate(w, "login.html", data); err != nil {
			slog.Error("execute login template", "error", err)
		}
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

	accept := r.Header.Get("Accept")
	// HTMX partials first, then browser HTML, then API JSON
	if isHtmxRequest(r) {
		vm := buildConfigFormVM(ec)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.template.ExecuteTemplate(w, "fragments.html", vm); err != nil {
			slog.Error("render config fragment template", "error", err)
		}
		return
	}

	if strings.Contains(accept, "text/html") {
		vm := buildConfigFormVM(ec)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := s.template.ExecuteTemplate(w, "config.html", vm); err != nil {
			slog.Error("render config.html template", "error", err)
		}
		return
	}

	dto := buildConfigDTO(ec)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(dto); err != nil {
		slog.Error("encode config DTO", "error", err)
	}
}

var applyClasses = map[string]string{
	"server.ocpp_port":                "rebuild",
	"server.ocpp_path":                "rebuild",
	"server.log_level":                "hot",
	"server.log_format":               "rebuild",
	"mqtt.broker":                     "rebuild",
	"mqtt.client_id":                  "rebuild",
	"mqtt.username":                   "rebuild",
	"mqtt.password":                   "rebuild",
	"mqtt.base_topic":                 "rebuild",
	"mqtt.topics.*":                   "rebuild",
	"mqtt.disconnect_threshold_sec":   "rebuild",
	"charging.min_amps":               "hot",
	"charging.max_amps":               "hot",
	"charging.contactor_cooldown_sec": "hot",
	"charging.default_amps":           "hot",
	"webui.enabled":                   "process_restart",
	"webui.listen":                    "process_restart",
	"webui.token":                     "hot",
}

func buildConfigFormVM(ec *config.EffectiveConfig) *configFormVM {
	ovf := ec.OverriddenByEnv
	sections := map[string]*configSectionVM{
		"server":   {Title: "Server", Fields: []configFieldVM{}},
		"mqtt":     {Title: "MQTT", Fields: []configFieldVM{}},
		"charging": {Title: "Charging", Fields: []configFieldVM{}},
		"webui":    {Title: "WebUI", Fields: []configFieldVM{}},
	}

	vf := func(section, name string, val any, secret, passwordSet, tokenSet bool) {
		sval := ""
		switch v := val.(type) {
		case int:
			sval = strconv.Itoa(v)
		case bool:
			sval = strconv.FormatBool(v)
		case string:
			sval = v
		}
		sections[section].Fields = append(sections[section].Fields, configFieldVM{
			Name:        name,
			Value:       sval,
			Disabled:    ovf[name],
			Secret:      secret,
			ApplyClass:  applyClasses[name],
			PasswordSet: passwordSet,
			TokenSet:    tokenSet,
		})
	}

	vf("server", "server.ocpp_port", ec.ServerOCPPPort, false, false, false)
	vf("server", "server.ocpp_path", ec.ServerOCPPPath, false, false, false)
	vf("server", "server.log_level", ec.ServerLogLevel, false, false, false)
	vf("server", "server.log_format", ec.ServerLogFormat, false, false, false)

	vf("mqtt", "mqtt.broker", ec.MQTTBroker, false, false, false)
	vf("mqtt", "mqtt.client_id", ec.MQTTClientID, false, false, false)
	vf("mqtt", "mqtt.username", ec.MQTTUsername, false, false, false)
	vf("mqtt", "mqtt.password", "", true, ec.MQTTPasswordSet, false)
	vf("mqtt", "mqtt.base_topic", ec.MQTTBaseTopic, false, false, false)
	vf("mqtt", "mqtt.disconnect_threshold_sec", ec.MQTTDisconnectSec, false, false, false)

	vf("charging", "charging.min_amps", ec.ChargingMinAmps, false, false, false)
	vf("charging", "charging.max_amps", ec.ChargingMaxAmps, false, false, false)
	vf("charging", "charging.contactor_cooldown_sec", ec.ChargingContactorsSec, false, false, false)
	vf("charging", "charging.default_amps", ec.ChargingDefaultAmps, false, false, false)

	vf("webui", "webui.enabled", ec.WebUIEnabled, false, false, false)
	vf("webui", "webui.listen", ec.WebUIListen, false, false, false)
	vf("webui", "webui.token", "", true, false, ec.WebUITokenSet)

	return &configFormVM{Sections: sections}
}

func (s *Server) handlePostConfig(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	htmx := isHtmxRequest(r)

	if err := r.ParseForm(); err != nil {
		errMsg := "parse form: " + err.Error()
		if htmx {
			renderHTMLError(w, errMsg, http.StatusBadRequest)
		} else {
			jsonError(w, errMsg, http.StatusBadRequest)
		}
		return
	}

	current, err := config.Load(s.configPath)
	if err != nil {
		slog.Error("load effective config for save", "error", err)
		errMsg := "internal server error"
		if htmx {
			renderHTMLError(w, errMsg, http.StatusInternalServerError)
		} else {
			jsonError(w, errMsg, http.StatusInternalServerError)
		}
		return
	}

	ec, err := config.Effective(s.configPath)
	if err != nil {
		slog.Error("read effective config for save", "error", err)
		errMsg := "internal server error"
		if htmx {
			renderHTMLError(w, errMsg, http.StatusInternalServerError)
		} else {
			jsonError(w, errMsg, http.StatusInternalServerError)
		}
		return
	}

	for field := range r.Form {
		if ec.OverriddenByEnv[field] {
			envVar := dotToEnv(field)
			slog.Warn("webui_config_rejected", "field", field, "env_var", envVar)
			errMsg := fmt.Sprintf("cannot change %s: overridden by environment variable %s", field, envVar)
			if htmx {
				renderHTMLError(w, errMsg, http.StatusBadRequest)
			} else {
				jsonError(w, errMsg, http.StatusBadRequest)
			}
			return
		}
	}

	candidate := cloneConfig(current)
	applyFormValues(candidate, r.Form, current)

	if err := config.Validate(candidate); err != nil {
		slog.Warn("webui_config_rejected", "reason", "validation", "error", err)
		errMsg := "validation failed: " + err.Error()
		if htmx {
			renderHTMLError(w, errMsg, http.StatusBadRequest)
		} else {
			jsonError(w, errMsg, http.StatusBadRequest)
		}
		return
	}

	report := config.ClassifyChanges(current, candidate)

	submittedToken := r.FormValue("confirm_token")
	if submittedToken == "" && s.confirm != nil {
		if report.Class == config.ApplyRebuild || report.Class == config.ApplyProcessRestart {
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
			s.renderResult(w, r, &activeResult)
			return
		}
	}

	if report.Class == config.ApplyNone {
		s.renderResult(w, r, &applyResult{Action: "none", FieldsChanged: []string{}})
		return
	}

	// Process restart: save to disk immediately, no rebuild possible
	if report.Class == config.ApplyProcessRestart {
		if err := config.WriteAtomic(s.configPath, candidate); err != nil {
			slog.Error("atomic write failed", "error", err)
			errMsg := "atomic write failed: " + err.Error()
			if htmx {
				renderHTMLError(w, errMsg, http.StatusInternalServerError)
			} else {
				jsonError(w, errMsg, http.StatusInternalServerError)
			}
			return
		}
		logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "process_restart", false, nil)
		s.renderResult(w, r, &applyResult{
			Action:          "process_restart",
			FieldsChanged:   report.Fields,
			RestartRequired: true,
		})
		return
	}

	// Rebuild: check active session before saving
	if report.Class == config.ApplyRebuild && s.applier != nil {
		chargerIDs, hasActive := s.applier.HasActiveSession()

		if hasActive {
			nonce := newConfirmToken()
			s.confirm = &confirmEntry{
				nonce:     nonce,
				fields:    report.Fields,
				expires:   time.Now().Add(5 * time.Minute),
				candidate: candidate,
			}
			logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "rebuild_pending", hasActive, chargerIDs)
			s.renderResult(w, r, &applyResult{
				Action:                     "rebuild",
				FieldsChanged:              report.Fields,
				RequiresRebuild:            true,
				ActiveSession:              true,
				ConfirmToken:               nonce,
				ChargerReconfigureRequired: report.ChargerReconfigureRequired,
			})
			return
		}

		// PERSIST + REBUILD — no active session, safe to proceed
		if err := config.WriteAtomic(s.configPath, candidate); err != nil {
			slog.Error("atomic write failed", "error", err)
			errMsg := "atomic write failed: " + err.Error()
			if htmx {
				renderHTMLError(w, errMsg, http.StatusInternalServerError)
			} else {
				jsonError(w, errMsg, http.StatusInternalServerError)
			}
			return
		}

		startTime := time.Now()
		if err := s.applier.Rebuild(candidate); err != nil {
			slog.Error("webui_rebuild_failed", "error", err, "duration_ms", time.Since(startTime).Milliseconds())
			errMsg := "rebuild failed: " + err.Error()
			if htmx {
				renderHTMLError(w, errMsg, http.StatusInternalServerError)
			} else {
				jsonError(w, errMsg, http.StatusInternalServerError)
			}
			return
		}

		logAudit(slog.LevelInfo, "webui_rebuild_completed", report.Fields, "rebuild", false, nil)
		s.renderResult(w, r, &applyResult{
			Action:                     "rebuild",
			FieldsChanged:              report.Fields,
			ChargerReconfigureRequired: report.ChargerReconfigureRequired,
		})
		return
	}

	// No applier: hot reload or fallback
	if report.Class == config.ApplyHot {
		if s.applier != nil {
			if err := applyHot(s.applier, current, candidate); err != nil {
				slog.Error("hot apply failed", "error", err)
				errMsg := "apply failed: " + err.Error()
				if htmx {
					renderHTMLError(w, errMsg, http.StatusInternalServerError)
				} else {
					jsonError(w, errMsg, http.StatusInternalServerError)
				}
				return
			}
		}
	}

	if err := config.WriteAtomic(s.configPath, candidate); err != nil {
		slog.Error("atomic write failed", "error", err)
		errMsg := "atomic write failed: " + err.Error()
		if htmx {
			renderHTMLError(w, errMsg, http.StatusInternalServerError)
		} else {
			jsonError(w, errMsg, http.StatusInternalServerError)
		}
		return
	}

	logAudit(slog.LevelInfo, "webui_config_saved", report.Fields, "hot_reload", false, nil)
	s.renderResult(w, r, &applyResult{
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
	m := c.MQTT
	m.Topics = maps.Clone(c.MQTT.Topics)
	return &config.Config{
		Server:   c.Server,
		MQTT:     m,
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

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(applyResult{Error: msg, ErrorMessage: msg}); err != nil {
		slog.Error("encode json error response", "error", err)
	}
}

func writeJSON(w http.ResponseWriter, r any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(r)
}

func isHtmxRequest(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

func renderHTMLError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(code)
	_, _ = fmt.Fprintf(w, `<div class="frag-error">%s</div>`, template.HTMLEscapeString(msg))
}

func (s *Server) renderResult(w http.ResponseWriter, r *http.Request, ar *applyResult) {
	if !isHtmxRequest(r) {
		writeJSON(w, ar)
		return
	}

	vm := &resultVM{
		FieldsChanged:              ar.FieldsChanged,
		ActiveSession:              ar.ActiveSession,
		ChargerReconfigureRequired: ar.ChargerReconfigureRequired,
		ConfirmToken:               ar.ConfirmToken,
	}

	switch ar.Action {
	case "hot_reload":
		vm.ResultClass = "fragHot"
	case "process_restart":
		vm.ResultClass = "fragProcessRestart"
	case "rebuild", "active_session":
		if ar.ConfirmToken != "" {
			vm.ResultClass = "fragConfirm"
		} else if ar.ChargerReconfigureRequired {
			vm.ResultClass = "fragRebuildOcpp"
		} else {
			vm.ResultClass = "fragRebuild"
		}
	case "none":
		vm.ResultClass = "fragNone"
	default:
		vm.ResultClass = "fragHot"
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("HX-Retry", "false")
	_ = s.template.ExecuteTemplate(w, vm.ResultClass+".html", vm)
}
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
		Handler:           s.mux,
		ReadHeaderTimeout: 5 * time.Minute,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	go func() {
		<-ctx.Done()
		_ = httpSrv.Shutdown(context.Background())
	}()

	if err := httpSrv.Serve(ln); err != http.ErrServerClosed {
		slog.Warn("webui server error", "error", err)
	}

	return nil
}