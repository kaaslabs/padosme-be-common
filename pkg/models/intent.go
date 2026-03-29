package models

// IntentResult is the shared intent object produced by padosme-query-intelligence
// and consumed by padosme-search-api / padosme-search-engine.
type IntentResult struct {
	Intent     string   `json:"intent"`
	Category   string   `json:"category"`
	Product    string   `json:"product"`
	Filters    []string `json:"filters"`
	Confidence float64  `json:"confidence"`
	Source     string   `json:"source"`
	Query      string   `json:"query"`
}

// Valid intent type values.
const (
	IntentProductSearch  = "product_search"
	IntentServiceSearch  = "service_search"
	IntentLocationSearch = "location_search"
)

// Valid source values.
const (
	SourceCache    = "cache"
	SourceOntology = "ontology"
	SourceKeyword  = "keyword"
)

// Valid category values.
const (
	CategoryRestaurant    = "restaurant"
	CategoryStreetFood    = "street_food"
	CategoryGrocery       = "grocery"
	CategoryElectronics   = "electronics"
	CategoryHomeServices  = "home_services"
	CategoryBeauty        = "beauty_wellness"
	CategoryMedical       = "medical"
	CategoryAutomotive    = "automotive"
	CategoryClothing      = "clothing"
	CategoryEducation     = "education"
	CategoryFinance       = "finance"
	CategoryTransport     = "transport"
	CategoryPetrolStation = "petrol_station"
	CategoryATM           = "atm"
	CategoryOther         = "other"
)
