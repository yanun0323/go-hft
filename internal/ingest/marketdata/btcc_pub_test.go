package marketdata

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBtccPubDepth(t *testing.T) {
	bp := NewBtccPub(t.Context(), false)
	err := bp.StartWebsocket(t.Context())
	require.NoError(t, err)

	err = bp.SubscribeDepth(t.Context(), "SOLUSDT", 20, 0.00000001)
	require.NoError(t, err)

	res := make(chan BtccDepth, 1)
	defer close(res)

	cancel := bp.ObserveDepth(t.Context(), func(d BtccDepth) {
		res <- d
	})
	defer cancel()

	select {
	case d := <-res:
		assert.Truef(t, len(d.Orderbook.Asks) != 0 || len(d.Orderbook.Bids) != 0,
			"asks/bids should not be empty, asks %d, bids %d",
			len(d.Orderbook.Asks), len(d.Orderbook.Bids),
		)
	case <-t.Context().Done():
		t.Fatal("timeout")
	}
}
