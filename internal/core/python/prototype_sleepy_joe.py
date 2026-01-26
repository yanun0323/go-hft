from decimal import Decimal
from typing import Literal


class OrderRequest:
    symbol: str
    platform: Literal['BTCC']
    action: Literal['PLACE_ORDER', 'CANCEL_ORDER']  # required
    type: Literal['LIMIT', 'MARKET']                # required when PLACE_ORDER
    side: Literal['BUY', 'SELL']                    # required when PLACE_ORDER
    price: Decimal                                  # required when PLACE_ORDER + LIMIT
    quantity: Decimal                               # required when PLACE_ORDER
    timeInForce: Literal['GTC', 'FOK', 'IOC']       # optional
    cancel_order_id: str                            # required when CANCEL_ORDER
    
class DB:
    ico_price: Decimal
    ticker_size: Decimal
    spread_tick: Decimal
    refresh_time: int # second

    ask_total_amount: Decimal
    ask_pct_tob: Decimal    # Top Of Book
    ask_pct_mob: Decimal    # Middle Of Book
    ask_pct_dob: Decimal    # Deep Of Book
    ask_level_tob: Decimal  # Top Of Book
    ask_level_mob: Decimal  # Middle Of Book
    ask_level_dob: Decimal  # Deep Of Book

    bid_total_amount: Decimal
    bid_pct_tob: Decimal    # Top Of Book
    bid_pct_mob: Decimal    # Middle Of Book
    bid_pct_dob: Decimal    # Deep Of Book
    bid_level_tob: Decimal  # Top Of Book
    bid_level_mob: Decimal  # Middle Of Book
    bid_level_dob: Decimal  # Deep Of Book

class SleepyJoe:

    db: DB
    
    def cron_job(self) -> list[OrderRequest]:
        """
        This function invoked every second.
        You can handle refresh time event operation here.
        """

    def execute(self, mid_price: Decimal) -> list[OrderRequest]:
        """
        This function invoked immediately right after mid price changed.
        """
        # TODO: implement this
        ...
