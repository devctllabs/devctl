package deps

import (
	"context"
	"fmt"

	"github.com/devctllabs/go-libs/di"
	"github.com/labstack/echo/v5"
	"google.golang.org/grpc"
)

// application is the user-owned composition root. Add application dependencies here.
type application struct{}

func (*application) RegisterHTTP(*echo.Echo)   {}
func (*application) RegisterGRPC(*grpc.Server) {}

// provideApplication is created once. Add newly scaffolded provider calls manually.
func provideApplication(ctx context.Context, graph *di.Container, cfg *Config) error {
	if err := di.Provide[HTTPRegistrar](graph, func(di.Resolver) (HTTPRegistrar, error) { return &application{}, nil }); err != nil {
		return fmt.Errorf("di.Provide HTTPRegistrar: %w", err)
	}
	if err := di.Provide[GRPCRegistrar](graph, func(di.Resolver) (GRPCRegistrar, error) { return &application{}, nil }); err != nil {
		return fmt.Errorf("di.Provide GRPCRegistrar: %w", err)
	}
	if err := provideLogging(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideLogging: %w", err)
	}
	if err := provideTelemetry(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideTelemetry: %w", err)
	}
	if err := provideStoragePrimary(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideStoragePrimary: %w", err)
	}
	if err := provideStorageAnalytics(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideStorageAnalytics: %w", err)
	}
	if err := provideAuditConsumer(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideAuditConsumer: %w", err)
	}
	if err := provideInvoiceConsumer(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideInvoiceConsumer: %w", err)
	}
	if err := provideEventsKafkaProducer(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideEventsKafkaProducer: %w", err)
	}
	if err := provideCatalogHTTPClient(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideCatalogHTTPClient: %w", err)
	}
	if err := provideBillingGRPCClient(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideBillingGRPCClient: %w", err)
	}
	if err := provideRedisCache(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideRedisCache: %w", err)
	}
	if err := provideS3Default(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideS3Default: %w", err)
	}
	if err := provideS3MediaBucket(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideS3MediaBucket: %w", err)
	}
	if err := provideRuntime(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideRuntime: %w", err)
	}
	return nil
}
