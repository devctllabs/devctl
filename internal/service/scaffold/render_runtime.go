package scaffold

type runtimeTemplateData struct {
	HTTP              bool
	HTTPToggle        bool
	GRPC              bool
	GRPCToggle        bool
	Health            bool
	HealthToggle      bool
	Pprof             bool
	PprofToggle       bool
	HealthConnections []runtimeHealthConnection
}

type runtimeHealthConnection struct {
	Connection string
	Probe      string
}

func renderRuntime(data runtimeTemplateData) (string, error) {
	return executeTemplate("runtime.go.gotmpl", data)
}
