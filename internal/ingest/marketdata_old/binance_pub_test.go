package marketdata_old

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/yanun0323/pkg/sys"
)

func TestBinancePubDepthMeasure(t *testing.T) {
	return
	alloc, bytes := sys.MeasureMem(func() {
		TestBinancePubDepth(t)
	})
	t.Logf("a: %d, b: %d", alloc, bytes)
}

func TestBinancePubDepth(t *testing.T) {
	return
	bp := NewBinancePub(t.Context())
	err := bp.StartWebsocket(t.Context())
	require.NoError(t, err)

	err = bp.SubscribeDepth(t.Context(), "SOLUSDT")
	require.NoError(t, err)
	res := make(chan BinanceDepth, 1)
	defer close(res)

	cancel := bp.ObserveDepth(t.Context(), func(d BinanceDepth) {
		res <- d
	})
	defer cancel()

	select {
	case d := <-res:
		assert.Truef(t, len(d.Asks) != 0 || len(d.Bids) != 0,
			"asks/bids should not be empty, asks %d, bids %d",
			len(d.Asks), len(d.Bids),
		)
	case <-t.Context().Done():
		t.Fatal("timeout")
	}
}

func TestBinancePubPPartialBookDepth(t *testing.T) {
	return
	bp := NewBinancePub(t.Context())
	err := bp.StartWebsocket(t.Context())
	require.NoError(t, err)

	err = bp.SubscribePartialBookDepth(t.Context(), "SOLUSDT")
	require.NoError(t, err)
	res := make(chan BinancePartialBookDepth, 1)
	defer close(res)

	cancel := bp.ObservePartialBookDepth(t.Context(), func(d BinancePartialBookDepth) {
		res <- d
	})
	defer cancel()

	select {
	case d := <-res:
		assert.Truef(t, len(d.Asks) != 0 || len(d.Bids) != 0,
			"asks/bids should not be empty, asks %d, bids %d",
			len(d.Asks), len(d.Bids),
		)
	case <-t.Context().Done():
		t.Fatal("timeout")
	}
}
