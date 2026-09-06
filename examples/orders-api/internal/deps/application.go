package deps

import (
	"context"
	"fmt"

	"example.com/orders-api/gen/serverhttp"
	"example.com/orders-api/internal/orders"
	"github.com/devctllabs/go-libs/di"
	"github.com/devctllabs/go-libs/postgresdb"
	"github.com/labstack/echo/v5"
)

// application is the user-owned composition root. Add application dependencies here.
type application struct {
	orders *orders.Handler
}

func (a *application) RegisterHTTP(server *echo.Echo) {
	serverhttp.RegisterHandlers(server, serverhttp.NewStrictHandler(a.orders, nil))
}

// provideApplication is created once. Add newly scaffolded provider calls manually.
func provideApplication(ctx context.Context, graph *di.Container, cfg *Config) error {
	if err := di.Provide[HTTPRegistrar](graph, func(resolver di.Resolver) (HTTPRegistrar, error) {
		reader, err := di.ResolveNamed[*postgresdb.Endpoint](resolver, storagePrimaryConnectionName+".reader")
		if err != nil {
			return nil, fmt.Errorf("resolve primary reader: %w", err)
		}
		writer, err := di.ResolveNamed[*postgresdb.Endpoint](resolver, storagePrimaryConnectionName+".writer")
		if err != nil {
			return nil, fmt.Errorf("resolve primary writer: %w", err)
		}
		store := orders.NewPostgresStore(reader, writer)
		return &application{orders: orders.NewHandler(store)}, nil
	}); err != nil {
		return fmt.Errorf("di.Provide HTTPRegistrar: %w", err)
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
	if err := provideRuntime(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provideRuntime: %w", err)
	}
	return nil
}
