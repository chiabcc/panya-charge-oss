package ocpp

import (
	"context"
	"log/slog"
	"sync/atomic"

	"github.com/xBlaz3kx/ocpp-go/logging"
	ocpp16 "github.com/xBlaz3kx/ocpp-go/ocpp1.6"
)

type Server struct {
	cs      ocpp16.CentralSystem
	handler *Handler
	port    int
	path    string
	logger  *slog.Logger
	running atomic.Bool
}

func NewServer(port int, path string, handler *Handler, logger *slog.Logger) (*Server, error) {
	var ocppLogger logging.Logger = &logging.VoidLogger{}
	if logger.Enabled(context.Background(), slog.LevelDebug) {
		ocppLogger = newSlogLogger(logger)
	}

	cs, err := ocpp16.NewCentralSystem(nil, nil, ocppLogger)
	if err != nil {
		return nil, err
	}

	cs.SetCoreHandler(handler)
	cs.SetNewChargePointHandler(func(cp ocpp16.ChargePointConnection) {
		handler.OnConnect(cp.ID())
	})
	cs.SetChargePointDisconnectedHandler(func(cp ocpp16.ChargePointConnection) {
		handler.OnDisconnect(cp.ID())
	})

	return &Server{
		cs:      cs,
		handler: handler,
		port:    port,
		path:    path,
		logger:  logger,
	}, nil
}

func (s *Server) CentralSystem() ocpp16.CentralSystem {
	return s.cs
}

func (s *Server) Start() {
	s.logger.Info("starting ocpp csms server", "port", s.port, "path", s.path)
	s.running.Store(true)
	go s.cs.Start(s.port, s.path)
}

func (s *Server) Stop() {
	s.logger.Info("stopping ocpp csms server")
	s.running.Store(false)
	s.cs.Stop()
}

func (s *Server) IsRunning() bool {
	return s.running.Load()
}
