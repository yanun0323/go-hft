package ops

import (
	"encoding/json"
	"fmt"
	"os"

	"main/internal/risk"
	"main/internal/schema"
)

// FileConfig mirrors the JSON config layout.
type FileConfig struct {
	Registry RegistryConfig `json:"registry"`
	Risk     risk.Config    `json:"risk"`
	Order    OrderConfig    `json:"order"`
	Features FeatureFlagsConfig `json:"features"`
}

// RegistryConfig defines venue and symbol mappings.
type RegistryConfig struct {
	Venues  []VenueConfig  `json:"venues"`
	Symbols []SymbolConfig `json:"symbols"`
}

// VenueConfig describes a venue entry.
type VenueConfig struct {
	Name string `json:"name"`
}

// SymbolConfig describes a symbol entry.
type SymbolConfig struct {
	Name  string          `json:"name"`
	Venue string          `json:"venue"`
	Scale schema.ScaleSpec `json:"scale"`
}

// OrderConfig describes the dummy order to publish.
type OrderConfig struct {
	OrderID     uint64           `json:"orderId"`
	StrategyID  uint32           `json:"strategyId"`
	Symbol      string           `json:"symbol"`
	Side        schema.OrderSide `json:"side"`
	Type        schema.OrderType `json:"type"`
	TimeInForce schema.TimeInForce `json:"timeInForce"`
	Price       schema.Price     `json:"price"`
	Qty         schema.Quantity  `json:"qty"`
}

// FeatureFlagsConfig captures optional runtime flags.
type FeatureFlagsConfig struct {
	EnableOrderFlow *bool `json:"enableOrderFlow"`
	EnableFills     *bool `json:"enableFills"`
}

// FeatureFlags are resolved runtime flags.
type FeatureFlags struct {
	EnableOrderFlow bool
	EnableFills     bool
}

// Loaded is the resolved configuration ready for use.
type Loaded struct {
	Registry *schema.Registry
	Risk     risk.Config
	Order    OrderSpec
	Features FeatureFlags
}

// OrderSpec is the resolved order definition.
type OrderSpec struct {
	OrderID     uint64
	StrategyID  uint32
	SymbolID    schema.SymbolID
	Side        schema.OrderSide
	Type        schema.OrderType
	TimeInForce schema.TimeInForce
	Price       schema.Price
	Qty         schema.Quantity
}

// Load reads a JSON config file and builds the registry.
func Load(path string) (Loaded, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Loaded{}, err
	}
	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Loaded{}, err
	}
	registry, err := buildRegistry(cfg.Registry)
	if err != nil {
		return Loaded{}, err
	}
	orderSpec, err := resolveOrderSpec(cfg.Order, registry)
	if err != nil {
		return Loaded{}, err
	}
	features := resolveFeatures(cfg.Features)
	return Loaded{
		Registry: registry,
		Risk:     cfg.Risk,
		Order:    orderSpec,
		Features: features,
	}, nil
}

// LoadRegistry reads a JSON config file and only builds the registry.
func LoadRegistry(path string) (*schema.Registry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg FileConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return buildRegistry(cfg.Registry)
}

func buildRegistry(cfg RegistryConfig) (*schema.Registry, error) {
	reg := schema.NewRegistry()
	for _, venue := range cfg.Venues {
		if _, err := reg.AddVenue(venue.Name); err != nil {
			return nil, err
		}
	}
	for _, sym := range cfg.Symbols {
		venueID, ok := reg.VenueIDByName(sym.Venue)
		if !ok {
			return nil, fmt.Errorf("venue not found: %s", sym.Venue)
		}
		if err := validateScale(sym.Scale); err != nil {
			return nil, fmt.Errorf("invalid scale for %s: %w", sym.Name, err)
		}
		if _, err := reg.AddSymbol(sym.Name, venueID, sym.Scale); err != nil {
			return nil, err
		}
	}
	return reg, nil
}

func validateScale(scale schema.ScaleSpec) error {
	if scale.PriceScale < 0 || scale.QuantityScale < 0 || scale.NotionalScale < 0 || scale.FeeScale < 0 {
		return fmt.Errorf("scale must be >= 0")
	}
	return nil
}

func resolveOrderSpec(cfg OrderConfig, reg *schema.Registry) (OrderSpec, error) {
	if cfg.Symbol == "" {
		return OrderSpec{}, fmt.Errorf("order symbol is empty")
	}
	symbolID, ok := reg.SymbolIDByName(cfg.Symbol)
	if !ok {
		return OrderSpec{}, fmt.Errorf("order symbol not found: %s", cfg.Symbol)
	}
	if cfg.Qty <= 0 {
		return OrderSpec{}, fmt.Errorf("order qty must be > 0")
	}
	if cfg.Side == schema.OrderSideUnknown {
		return OrderSpec{}, fmt.Errorf("order side is unknown")
	}
	if cfg.Type == schema.OrderTypeUnknown {
		return OrderSpec{}, fmt.Errorf("order type is unknown")
	}
	if cfg.TimeInForce == schema.TimeInForceUnknown {
		return OrderSpec{}, fmt.Errorf("order timeInForce is unknown")
	}
	if cfg.Type == schema.OrderTypeLimit && cfg.Price <= 0 {
		return OrderSpec{}, fmt.Errorf("order price must be > 0 for limit orders")
	}
	if cfg.OrderID == 0 {
		cfg.OrderID = 1001
	}
	if cfg.StrategyID == 0 {
		cfg.StrategyID = 1
	}
	return OrderSpec{
		OrderID:     cfg.OrderID,
		StrategyID:  cfg.StrategyID,
		SymbolID:    symbolID,
		Side:        cfg.Side,
		Type:        cfg.Type,
		TimeInForce: cfg.TimeInForce,
		Price:       cfg.Price,
		Qty:         cfg.Qty,
	}, nil
}

func resolveFeatures(cfg FeatureFlagsConfig) FeatureFlags {
	flags := FeatureFlags{
		EnableOrderFlow: true,
		EnableFills:     true,
	}
	if cfg.EnableOrderFlow != nil {
		flags.EnableOrderFlow = *cfg.EnableOrderFlow
	}
	if cfg.EnableFills != nil {
		flags.EnableFills = *cfg.EnableFills
	}
	return flags
}
