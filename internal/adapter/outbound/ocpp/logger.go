package ocpp

import (
	"fmt"
	"log/slog"
)

type slogLogger struct{ l *slog.Logger }

func newSlogLogger(l *slog.Logger) *slogLogger {
	return &slogLogger{l: l.With("component", "ocpp-go")}
}

func (s *slogLogger) Debug(args ...interface{}) {
	s.l.Debug(fmt.Sprint(args...))
}

func (s *slogLogger) Debugf(format string, args ...interface{}) {
	s.l.Debug(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Info(args ...interface{}) {
	s.l.Info(fmt.Sprint(args...))
}

func (s *slogLogger) Infof(format string, args ...interface{}) {
	s.l.Info(fmt.Sprintf(format, args...))
}

func (s *slogLogger) Error(args ...interface{}) {
	s.l.Error(fmt.Sprint(args...))
}

func (s *slogLogger) Errorf(format string, args ...interface{}) {
	s.l.Error(fmt.Sprintf(format, args...))
}
