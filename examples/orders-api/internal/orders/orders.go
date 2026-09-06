package orders

import (
	"context"
	"errors"
	"fmt"
	"time"

	"example.com/orders-api/gen/serverhttp"
	"github.com/devctllabs/go-libs/postgresdb"
	"github.com/jackc/pgx/v5"
)

var ErrNotFound = errors.New("order not found")

type Order struct {
	ID           int64
	CustomerName string
	TotalCents   int64
	CreatedAt    time.Time
}

type Store interface {
	Create(ctx context.Context, customerName string, totalCents int64) (Order, error)
	Get(ctx context.Context, id int64) (Order, error)
}

type PostgresStore struct {
	reader *postgresdb.Endpoint
	writer *postgresdb.Endpoint
}

func NewPostgresStore(reader, writer *postgresdb.Endpoint) *PostgresStore {
	return &PostgresStore{reader: reader, writer: writer}
}

func (s *PostgresStore) Create(ctx context.Context, customerName string, totalCents int64) (Order, error) {
	const query = `
		INSERT INTO orders (customer_name, total_cents)
		VALUES ($1, $2)
		RETURNING id, customer_name, total_cents, created_at`

	var order Order
	err := s.writer.QueryRow(ctx, query, customerName, totalCents).Scan(
		&order.ID,
		&order.CustomerName,
		&order.TotalCents,
		&order.CreatedAt,
	)
	if err != nil {
		return Order{}, fmt.Errorf("insert order: %w", err)
	}
	return order, nil
}

func (s *PostgresStore) Get(ctx context.Context, id int64) (Order, error) {
	const query = `
		SELECT id, customer_name, total_cents, created_at
		FROM orders
		WHERE id = $1`

	var order Order
	err := s.reader.QueryRow(ctx, query, id).Scan(
		&order.ID,
		&order.CustomerName,
		&order.TotalCents,
		&order.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return Order{}, ErrNotFound
	}
	if err != nil {
		return Order{}, fmt.Errorf("select order: %w", err)
	}
	return order, nil
}

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

func (h *Handler) CreateOrder(ctx context.Context, request serverhttp.CreateOrderRequestObject) (serverhttp.CreateOrderResponseObject, error) {
	if request.Body == nil {
		return nil, errors.New("create order body is required")
	}
	order, err := h.store.Create(ctx, request.Body.CustomerName, request.Body.TotalCents)
	if err != nil {
		return nil, err
	}
	return serverhttp.CreateOrder201JSONResponse(toAPIOrder(order)), nil
}

func (h *Handler) GetOrder(ctx context.Context, request serverhttp.GetOrderRequestObject) (serverhttp.GetOrderResponseObject, error) {
	order, err := h.store.Get(ctx, request.Id)
	if errors.Is(err, ErrNotFound) {
		return serverhttp.GetOrder404JSONResponse{Message: "order not found"}, nil
	}
	if err != nil {
		return nil, err
	}
	return serverhttp.GetOrder200JSONResponse(toAPIOrder(order)), nil
}

func toAPIOrder(order Order) serverhttp.Order {
	return serverhttp.Order{
		Id:           order.ID,
		CustomerName: order.CustomerName,
		TotalCents:   order.TotalCents,
		CreatedAt:    order.CreatedAt,
	}
}
