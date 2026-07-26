package webui

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"sync"

	"github.com/chiabcc/panya-charge-oss/internal/config"
)

// NewServer constructs a WebUI server.
//
// Parameters:
//   - configPath: path to config.yaml (read fresh per request)
//   - listenAddr: bind address from WebUIConfig.Listen
//   - token: authentication token (empty = no auth on loopback)
//   - isLoopback: true if listenAddr resolves to 127.0.0.1 / ::1
func NewServer(configPath string, listenAddr string, token string, isLoopback bool) *Server {
	mux := http.NewServeMux()

	srv := &Server{
		mux:        mux,
		configPath: configPath,
		listenAddr: listenAddr,
		token:      token,
		isLoopback: isLoopback,
		template: template.Must(template.ParseFS(staticFS, "templates/login.html")),
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
	mu         sync.Mutex
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
	http.Error(w, "not implemented", http.StatusNotImplemented)
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