package entrypushd

import (
	"log/slog"

	"go.uber.org/zap"
	"go.uber.org/zap/exp/zapslog"
)

// NewSlog returns a *slog.Logger backed by the given zap.Logger.
// event-driven (eventmux, sqs, sns, fxmux) uses slog exclusively.
func NewSlog(logger *zap.Logger) *slog.Logger {
	return slog.New(zapslog.NewHandler(logger.Core()))
}
