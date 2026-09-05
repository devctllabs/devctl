package runtimeconfig_test

import (
	"testing"

	projectdomain "github.com/devctllabs/devctl/internal/domain/project"
	"github.com/devctllabs/devctl/internal/service/runtimeconfig"
	"github.com/stretchr/testify/require"
)

func TestRenderProducesCanonicalManagedOutput(t *testing.T) {
	t.Parallel()

	manifest := projectdomain.Manifest{
		Project: projectdomain.Identity{Name: "sample", Language: "go"},
		Components: projectdomain.Components{
			HTTP:  &projectdomain.HTTP{Server: &projectdomain.HTTPServer{Start: &projectdomain.Start{}}},
			Kafka: &projectdomain.Kafka{Consumers: []projectdomain.KafkaConsumer{{Name: "billing"}}},
		},
	}
	catalog, err := projectdomain.NewRuntimeConfigCatalog(manifest)
	require.NoError(t, err)

	output, err := runtimeconfig.Render(catalog)

	require.NoError(t, err)
	require.Len(t, output.Directory.Files, 1)
	require.Equal(t, "config.gen.go", output.Directory.Files[0].Path)
	source := string(output.Directory.Files[0].Content)
	require.Regexp(t, `HTTP\s+HTTPConfig`, source)
	require.Regexp(t, `Address\s+string\s+`+"`"+`env:"SAMPLE_HTTP_ADDR" default:":8080"`+"`", source)
	require.Regexp(t, `Enabled\s+bool\s+`+"`"+`env:"SAMPLE_HTTP_SERVER_ENABLED" default:"false"`+"`", source)
	require.Regexp(t, `Brokers\s+\[\]string\s+`+"`"+`env:"SAMPLE_KAFKA_BROKERS" default:"localhost:29092"`+"`", source)
	require.Len(t, output.Files.Files, 1)
	require.Equal(t, ".env.example", output.Files.Files[0].Path)
	environment := string(output.Files.Files[0].Content)
	for _, line := range []string{
		"SAMPLE_HTTP_ADDR=:8080\n", "SAMPLE_HTTP_SERVER_ENABLED=false\n",
		"SAMPLE_KAFKA_BILLING_GROUP=sample-billing-group\n", "SAMPLE_KAFKA_BILLING_TOPIC=\n",
		"SAMPLE_KAFKA_BILLING_BATCH_MAX_SIZE=1\n", "SAMPLE_KAFKA_BILLING_RETRY_MAX_ATTEMPTS=3\n",
		"SAMPLE_KAFKA_BILLING_REBALANCE_TIMEOUT=30s\n", "SAMPLE_KAFKA_BILLING_SHUTDOWN_TIMEOUT=30s\n",
		"SAMPLE_KAFKA_BROKERS=localhost:29092\n",
	} {
		require.Contains(t, environment, line)
	}
}
