package scaffold

type containerTemplateData struct {
	Project             string
	Logging             bool
	Telemetry           bool
	TelemetryToggle     bool
	Database            bool
	Server              bool
	Kafka               bool
	RuntimeUsesResolver bool
	Connections         []containerConnection
	Consumers           []containerConsumer
}

type containerConnection struct {
	Name       string
	Connection string
}

type containerConsumer struct {
	Name    string
	Enabled string
}

func renderContainer(data containerTemplateData) (string, error) {
	return executeTemplate("container.go.gotmpl", data)
}

type applicationTemplateData struct {
	HTTP  bool
	GRPC  bool
	Calls []string
}

func renderApplication(data applicationTemplateData) (string, error) {
	return executeTemplate("application.go.gotmpl", data)
}
