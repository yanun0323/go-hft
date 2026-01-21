```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A["main"] --> B["run"]
    B --> C["Parse flags"]
    C --> D{"platform empty?"}
    D -->|"yes"| D1["return error: missing platform"]
    D -->|"no"| E{"socket-topic empty?"}
    E -->|"yes"| E1["return error: missing socket topic"]
    E -->|"no"| F["parsePlatform"]
    F -->|"error"| F1["return error"]
    F --> G["parseTopic"]
    G -->|"error"| G1["return error"]
    G --> H["buildArg"]
    H -->|"error"| H1["return error"]
    H --> I["build socket path"]
    I --> J["uds.NewClient"]
    J -->|"error"| J1["return error"]
    J --> K["client.Dial"]
    K -->|"error"| K1["return error"]
    K --> L["setup ctx + signal; goroutine closes conn"]
    L --> M["build MarketDataRequest"]
    M --> N["EncodeMarketDataRequest"]
    N -->|"error"| N1["return error"]
    N --> O["writeFull(conn, payload)"]
    O -->|"error"| O1["return error"]
    O --> P["readResponse (subscribe ack)"]
    P -->|"error"| P1["return error"]
    P --> Q["log ack"]
    Q --> R["loop readResponse"]
    R -->|"error & ctx done"| R1["return nil"]
    R -->|"error"| R2["return error"]
    R --> S["log response"]
    S --> T{"kind == Depth?"}
    T -->|"yes"| U["Depth.Decode + Debug log"]
    T -->|"no"| R
    U --> R
```

```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    H2{"arg text empty?"}
    H2 -->|"no"| H3["use raw arg bytes"]
    H2 -->|"yes"| H4{"symbol-id valid?"}
    H4 -->|"no"| H5["error: missing arg/symbol-id"]
    H4 -->|"yes"| H6{"topic"}
    H6 -->|"depth"| H7{"interval empty?"}
    H7 -->|"yes"| H8["error: missing interval"]
    H7 -->|"no"| H9["EncodeMarketDataArgDepth"]
    H6 -->|"order"| H10["EncodeMarketDataArgOrder"]
    H6 -->|"other"| H11["error: unknown topic"]
```

```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    P2["read 4-byte header"]
    P2 --> P3["parse platform/topic/argLen"]
    P3 --> P4["read arg bytes"]
    P4 --> P5["KindFromTopic"]
    P5 -->|"error"| P6["return error"]
    P5 --> P7["payloadSize(kind)"]
    P7 -->|"error"| P8["return error"]
    P7 --> P9["read payload bytes"]
    P9 --> P10["return response"]
```

MarketData (internal/ingest/market_data.go) - Subscribe + group wiring
```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A1["Subscribe(ctx, platform, apiKey, topic, arg, consumer)"]
    A1 --> B1{"m == nil?"}
    B1 -->|"yes"| B1E["return ErrInvalidMarketDataRequest"]
    B1 -->|"no"| C1{"platform/topic valid AND arg len > 0?"}
    C1 -->|"no"| C1E["return ErrInvalidMarketDataRequest"]
    C1 -->|"yes"| D1{"consumer == nil?"}
    D1 -->|"yes"| D1E["return ErrNilConsumer"]
    D1 -->|"no"| E1["getOrCreateGroup(ctx, platform, apiKey)"]
    E1 -->|"error"| E1E["return error"]
    E1 --> F1{"apiKey empty?"}
    F1 -->|"yes"| H1
    F1 -->|"no"| H1
    H1 -->|"error"| H1E["return error"]
    H1 --> I1["manager.AddConsumer(topicID, consumer)"]
    I1 -->|"error"| I1E["return error"]
    I1 --> J1["lock group; state.refCount++"]
    J1 --> K1["return nil"]
```

MarketData (internal/ingest/market_data.go) - Unsubscribe
```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A2["Unsubscribe(platform, apiKey, topic, arg, consumer)"]
    A2 --> B2{"m == nil?"}
    B2 -->|"yes"| B2E["return ErrInvalidMarketDataRequest"]
    B2 -->|"no"| C2{"platform/topic valid AND arg len > 0?"}
    C2 -->|"no"| C2E["return ErrInvalidMarketDataRequest"]
    C2 -->|"yes"| D2{"consumer == nil?"}
    D2 -->|"yes"| D2E["return ErrNilConsumer"]
    D2 -->|"no"| E2["getGroup(platform, apiKey)"]
    E2 -->|"nil"| E2E["return ErrUnknownTopic"]
    E2 --> F2["lookup topic state by key"]
    F2 -->|"nil"| F2E["return ErrUnknownTopic"]
    F2 --> G2["manager.RemoveConsumer(topicID, consumer)"]
    G2 -->|"error"| G2E["return error"]
    G2 --> H2["lock group; state.refCount--"]
    H2 --> I2{"refCount <= 0?"}
    I2 -->|"no"| J2["return nil"]
    I2 -->|"yes"| K2["delete state from maps"]
    K2 --> L2["codec.Unregister(topicID)"]
    L2 --> M2["manager.Unsubscribe(topicID)"]
    M2 --> J2
```

MarketData (internal/ingest/market_data.go) - getOrCreateGroup
```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A3["getOrCreateGroup(ctx, platform, apiKey)"]
    A3 --> B3{"platform valid?"}
    B3 -->|"no"| B3E["return ErrInvalidMarketDataRequest"]
    B3 -->|"yes"| C3["lock groups map"]
    C3 --> D3{"group exists?"}
    D3 -->|"yes"| D3A["unlock; group.start(ctx)"] --> D3R["return group"]
    D3 -->|"no"| E3["newGroup(platform, apiKey)"]
    E3 -->|"error"| E3E["unlock; return error"]
    E3 --> F3["store group; unlock"]
    F3 --> G3["group.start(ctx)"]
    G3 --> D3R

    H3["if apiKey not empty: RegisterAuth(reqID, apiKey)"]
    H3 -->|"error"| H3E["return error"]
    H3 --> H3R["return group"]
```

Binance payload (internal/ingest/binance/payload.go) - Decode dispatch
```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A4["DecodeMarketDataPayload(topic, arg, payload)"]
    A4 --> B4{"topic.IsAvailable?"}
    B4 -->|"no"| B4E["return ErrInvalidRequest"]
    B4 -->|"yes"| C4{"payload len > 0?"}
    C4 -->|"no"| C4E["return ErrInvalidPayload"]
    C4 -->|"yes"| D4["decodeSymbol(arg)"]
    D4 -->|"error"| D4E["return error"]
    D4 --> E4["decodeBinancePayload(topic, symbol, payload)"]

    E4 --> F4{"topic == Depth?"}
    F4 -->|"yes"| G4["decodeBinanceDepth(payload, symbol)"]
    F4 -->|"no"| H4{"topic == Order?"}
    H4 -->|"yes"| I4["decodeBinanceOrder(payload, symbol)"]
    H4 -->|"no"| J4["return ErrInvalidRequest"]
```

Binance payload (internal/ingest/binance/payload.go) - Depth parsing
```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A5["decodeBinanceDepth(payload, symbol)"]
    A5 --> B5["init Depth{SymbolID, Platform, RecvTsNano}"]
    B5 --> C5["scan event time ('E') -> EventTsNano"]
    C5 --> D5["scanDepthSide('b') or scanDepthSide('bids')"]
    D5 --> E5["scanDepthSide('a') or scanDepthSide('asks')"]
    E5 --> F5["set BidsLength/AsksLength"]
    F5 --> G5{"both lengths == 0?"}
    G5 -->|"yes"| G5E["return ErrInvalidPayload"]
    G5 -->|"no"| G5R["return Depth"]
```

Binance payload (internal/ingest/binance/payload.go) - Order parsing
```mermaid

---
config:
    markdownAutoWrap: false
    flowchart:
        wrappingWidth: 1000
---

flowchart TD
    A6["decodeBinanceOrder(payload, symbol)"]
    A6 --> B6["init Order{SymbolID, Platform, RecvTsNano}"]
    B6 --> C6["scan event time ('E') -> EventTsNano"]
    C6 --> D6["scan update time ('T') -> UpdatedTime"]
    D6 --> E6["scan order fields: id/side/type/status/tif"]
    E6 --> F6["scan qty ('q') -> Quantity"]
    F6 --> G6["scan price ('p') -> Price"]
    G6 --> H6["scan cum qty ('z') -> LeftQuantity"]
    H6 --> I6["set Source='binance'"]
    I6 --> J6{"order.ID == 0 AND no qty?"}
    J6 -->|"yes"| J6E["return ErrInvalidPayload"]
    J6 -->|"no"| J6R["return Order"]
```
