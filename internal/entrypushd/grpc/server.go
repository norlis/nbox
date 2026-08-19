package grpc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"

	"github.com/norlis/httpgate/logging"
	"go.uber.org/fx"
	googlegrpc "google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
	streamv1 "nbox/gen/stream/v1"
)

// ListenAddr is the TCP address the gRPC server binds to (e.g. ":9337").
// Named type so fx can wire it from entrypushd.Config without an import
// cycle (entrypushd → grpc → entrypushd via Config would cycle).
type ListenAddr string

// NewServer builds the gRPC server, registers KVStreamServer + reflection,
// and binds its lifecycle to fx (listen on start, GracefulStop on stop).
// authInterceptor is applied to all unary and server-streaming RPCs.
func NewServer(
	streamSrv *StreamServer,
	listen ListenAddr,
	logger *slog.Logger,
	authInterceptor *AuthInterceptor,
	lc fx.Lifecycle,
) *googlegrpc.Server {
	srv := googlegrpc.NewServer(
		googlegrpc.UnaryInterceptor(authInterceptor.Unary),
		googlegrpc.StreamInterceptor(authInterceptor.Stream),
	)
	streamv1.RegisterKVStreamServer(srv, streamSrv)
	reflection.Register(srv)

	addr := string(listen)

	lc.Append(fx.Hook{
		OnStart: func(_ context.Context) error {
			lis, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("grpc listen %s: %w", addr, err)
			}
			go func() {
				if err := srv.Serve(lis); err != nil && !errors.Is(err, googlegrpc.ErrServerStopped) {
					logger.Error("grpc server stopped",
						slog.String("addr", addr),
						logging.Err(err),
					)
				}
			}()
			logger.Info("server listening", slog.String("addr", addr))
			return nil
		},
		OnStop: func(ctx context.Context) error {
			done := make(chan struct{})
			go func() {
				srv.GracefulStop()
				close(done)
			}()
			select {
			case <-done:
				logger.Info("grpc server stopped gracefully")
			case <-ctx.Done():
				logger.WarnContext(ctx, "server stop forced", logging.Err(ctx.Err()))
				srv.Stop()
			}
			return nil
		},
	})

	return srv
}
