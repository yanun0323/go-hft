package btcc

import (
	"context"
	"main/internal/adapter"
	"net/http"
)

type Delegator struct{}

func (d Delegator) Send(ctx context.Context, req adapter.OrderRequest) (*http.Response, error) {
	panic("Implement me")
	// TODO: Implement me
}
