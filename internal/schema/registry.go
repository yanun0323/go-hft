package schema

import "fmt"

// Scale is the number of decimal places used by a scaled integer.
// Example: Scale=8 means the integer value is scaled by 1e8.
type Scale int32

// ScaleSpec defines scaling for common numeric fields.
type ScaleSpec struct {
	PriceScale    Scale
	QuantityScale Scale
	NotionalScale Scale
	FeeScale      Scale
}

// VenueID is the numeric identifier for a venue.
type VenueID uint16

// SymbolID is the numeric identifier for a symbol.
type SymbolID uint32

// Venue describes a trading venue or broker.
type Venue struct {
	ID   VenueID
	Name string
}

// Symbol describes a tradable instrument.
type Symbol struct {
	ID      SymbolID
	VenueID VenueID
	Name    string
	Scale   ScaleSpec
}

// Registry stores venue and symbol mappings in a compact form.
type Registry struct {
	venues       []Venue
	symbols      []Symbol
	venueByName  map[string]VenueID
	symbolByName map[string]SymbolID
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{
		venueByName:  make(map[string]VenueID),
		symbolByName: make(map[string]SymbolID),
	}
}

// AddVenue registers a new venue and returns its ID.
func (r *Registry) AddVenue(name string) (VenueID, error) {
	if name == "" {
		return 0, fmt.Errorf("venue name is empty")
	}
	if id, ok := r.venueByName[name]; ok {
		return id, fmt.Errorf("venue already exists: %s", name)
	}
	id := VenueID(len(r.venues) + 1)
	r.venues = append(r.venues, Venue{ID: id, Name: name})
	r.venueByName[name] = id
	return id, nil
}

// AddSymbol registers a new symbol and returns its ID.
func (r *Registry) AddSymbol(name string, venueID VenueID, scale ScaleSpec) (SymbolID, error) {
	if name == "" {
		return 0, fmt.Errorf("symbol name is empty")
	}
	if venueID == 0 {
		return 0, fmt.Errorf("venue id is invalid")
	}
	if _, ok := r.Venue(venueID); !ok {
		return 0, fmt.Errorf("venue id not found: %d", venueID)
	}
	if id, ok := r.symbolByName[name]; ok {
		return id, fmt.Errorf("symbol already exists: %s", name)
	}
	id := SymbolID(len(r.symbols) + 1)
	r.symbols = append(r.symbols, Symbol{
		ID:      id,
		VenueID: venueID,
		Name:    name,
		Scale:   scale,
	})
	r.symbolByName[name] = id
	return id, nil
}

// Venue returns the venue by ID.
func (r *Registry) Venue(id VenueID) (Venue, bool) {
	if id == 0 || int(id) > len(r.venues) {
		return Venue{}, false
	}
	return r.venues[id-1], true
}

// Symbol returns the symbol by ID.
func (r *Registry) Symbol(id SymbolID) (Symbol, bool) {
	if id == 0 || int(id) > len(r.symbols) {
		return Symbol{}, false
	}
	return r.symbols[id-1], true
}

// SymbolCount returns the number of symbols in the registry.
func (r *Registry) SymbolCount() int {
	return len(r.symbols)
}

// SymbolAt returns the symbol by zero-based index.
func (r *Registry) SymbolAt(index int) (Symbol, bool) {
	if index < 0 || index >= len(r.symbols) {
		return Symbol{}, false
	}
	return r.symbols[index], true
}

// VenueIDByName returns the venue ID for a name.
func (r *Registry) VenueIDByName(name string) (VenueID, bool) {
	id, ok := r.venueByName[name]
	return id, ok
}

// SymbolIDByName returns the symbol ID for a name.
func (r *Registry) SymbolIDByName(name string) (SymbolID, bool) {
	id, ok := r.symbolByName[name]
	return id, ok
}
