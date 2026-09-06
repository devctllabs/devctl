package orders

import (
	"context"
	"testing"
	"time"

	"example.com/orders-api/gen/serverhttp"
	"github.com/stretchr/testify/require"
)

type stubStore struct {
	created Order
	got     Order
	err     error
}

func (s *stubStore) Create(context.Context, string, int64) (Order, error) {
	return s.created, s.err
}

func (s *stubStore) Get(context.Context, int64) (Order, error) {
	return s.got, s.err
}

func TestHandlerCreatesOrder(t *testing.T) {
	t.Parallel()

	want := Order{ID: 7, CustomerName: "Ada", TotalCents: 1250, CreatedAt: time.Unix(1, 0).UTC()}
	handler := NewHandler(&stubStore{created: want})

	response, err := handler.CreateOrder(t.Context(), serverhttp.CreateOrderRequestObject{
		Body: &serverhttp.CreateOrder{CustomerName: "Ada", TotalCents: 1250},
	})

	require.NoError(t, err)
	require.Equal(t, serverhttp.CreateOrder201JSONResponse(toAPIOrder(want)), response)
}

func TestHandlerReturnsNotFound(t *testing.T) {
	t.Parallel()

	handler := NewHandler(&stubStore{err: ErrNotFound})

	response, err := handler.GetOrder(t.Context(), serverhttp.GetOrderRequestObject{Id: 404})

	require.NoError(t, err)
	require.Equal(t, serverhttp.GetOrder404JSONResponse{Message: "order not found"}, response)
}
