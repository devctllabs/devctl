package scaffold

type storageTemplateData struct {
	Name             string
	Connection       string
	Telemetry        bool
	ClickHouse       bool
	ClickHouseConfig string
	Kinds            []storageKind
	Variants         []storageVariant
}

type storageKind struct {
	Name  string
	Field string
}

type storageVariant struct {
	Name        string
	Kind        string
	Field       string
	ConfigField string
	SQLite      bool
	ClickHouse  bool
}

func renderStorage(data storageTemplateData) (string, error) {
	return executeTemplate("storage.go.gotmpl", data)
}
