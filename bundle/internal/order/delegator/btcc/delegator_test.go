package btcc

import (
	"main/libs/adapter"
	"main/libs/adapter/enum"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

func TestDelegator(t *testing.T) {
	accessID := os.Getenv("BTCC_TESTNET_ACCESS_ID")
	secretKey := os.Getenv("BTCC_TESTNET_SECRET_KEY")

	if len(accessID) == 0 || len(secretKey) == 0 {
		t.Fatalf("access id or secret key should not be empty! access key: %s, secret key: %s",
			accessID, secretKey)
	}

	token := adapter.NewToken(accessID, secretKey)
	d := NewDelegator(http.DefaultClient, true)
	now := time.Now().UnixMilli()

	{ // place market order
		resp, err := d.Send(t.Context(), adapter.OrderRequest{
			Intent: adapter.OrderIntent{
				Symbol:        adapter.NewSymbol("SOL", "USDT"),
				Platform:      enum.PlatformBTCC,
				Action:        enum.OrderActionPlaceOrder,
				Type:          enum.OrderTypeMarket,
				Side:          enum.OrderSideBuy,
				Quantity:      adapter.NewDecimal(100, 0),
				CreatedTime:   now,
				ClientOrderID: adapter.NewStr64("hft-" + strconv.FormatInt(now-1, 10)),
				Source:        adapter.NewStr64("hft-test-function"),
			},
			Token: token,
		})
		if err != nil {
			t.Fatalf("send place market order request, err: %+v", err)
		}

		if err := resp.Error(); err != nil {
			t.Fatalf("send place market order response, response: %+v , err: %+v", resp, err)
		}
	}

	{ // place market order
		resp, err := d.Send(t.Context(), adapter.OrderRequest{
			Intent: adapter.OrderIntent{
				Symbol:        adapter.NewSymbol("SOL", "USDT"),
				Platform:      enum.PlatformBTCC,
				Action:        enum.OrderActionPlaceOrder,
				Type:          enum.OrderTypeMarket,
				Side:          enum.OrderSideBuy,
				Quantity:      adapter.NewDecimal(100, 0),
				CreatedTime:   now,
				ClientOrderID: adapter.NewStr64("hft-" + strconv.FormatInt(now-1, 10)),
				Source:        adapter.NewStr64("hft-b-123456789012345678901234"),
			},
			Token: token,
		})
		if err != nil {
			t.Fatalf("send place market order request, err: %+v", err)
		}

		if err := resp.Error(); err != nil {
			t.Fatalf("send place market order response, response: %+v , err: %+v", resp, err)
		}
	}

	cid := "hft-" + strconv.FormatInt(now, 10)
	id := adapter.NewStr64("")
	{ // place limit order
		resp, err := d.Send(t.Context(), adapter.OrderRequest{
			Intent: adapter.OrderIntent{
				Symbol:        adapter.NewSymbol("SOL", "USDT"),
				Platform:      enum.PlatformBTCC,
				Action:        enum.OrderActionPlaceOrder,
				Type:          enum.OrderTypeLimit,
				Side:          enum.OrderSideBuy,
				Price:         adapter.NewDecimal(120, 0),
				Quantity:      adapter.NewDecimal(10, 0),
				TimeInForce:   enum.OrderTimeInForceGTC,
				CreatedTime:   now,
				ClientOrderID: adapter.NewStr64(cid),
				Source:        adapter.NewStr64("hft-test-function"),
			},
			Token: token,
		})
		if err != nil {
			t.Fatalf("send place limit order request, err: %+v", err)
		}

		if err := resp.Error(); err != nil {
			t.Fatalf("send place limit order response, response: %+v , err: %+v", resp, err)
		}

		id = resp.OrderID
	}

	{ // cancel order
		resp, err := d.Send(t.Context(), adapter.OrderRequest{
			Intent: adapter.OrderIntent{
				Symbol:        adapter.NewSymbol("SOL", "USDT"),
				Platform:      enum.PlatformBTCC,
				Action:        enum.OrderActionCancelOrder,
				CreatedTime:   now,
				ID:            id,
				ClientOrderID: adapter.NewStr64(cid),
			},
			Token: token,
		})
		if err != nil {
			t.Fatalf("send cancel order request, err: %+v", err)
		}

		if err := resp.Error(); err != nil {
			t.Fatalf("send cancel order response, response: %+v , err: %+v", resp, err)
		}
	}
}

/*
stock_gts2.stock_user_deal_history
office.t_customer
office.t_customer
stock_gts2.t_stock_currency
stock_gts2.stock_order_history
*/
