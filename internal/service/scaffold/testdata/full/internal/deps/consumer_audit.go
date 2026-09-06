package deps

import (
	"context"
	"fmt"

	auditconsumer "example.test/sample-api/internal/transport/consumerkafka/audit"
	"github.com/devctllabs/go-libs/di"
	kafka "github.com/devctllabs/go-libs/kafka"
	retry "github.com/devctllabs/go-libs/retry"
)

// provideAuditConsumer is user-owned: change []byte and the decoder for generated schema types.
func provideAuditConsumer(ctx context.Context, graph *di.Container, cfg *Config) error {
	key := kafkaConsumerKey("audit")
	if err := provideAuditConsumerConfig(ctx, graph, cfg); err != nil {
		return fmt.Errorf("provide consumer config: %w", err)
	}
	if err := di.ProvideNamedValue[kafka.Decoder[[]byte]](graph, key, rawKafkaDecoder()); err != nil {
		return fmt.Errorf("provide consumer decoder: %w", err)
	}
	if err := di.ProvideNamedValue[kafka.BatchHandler[[]byte]](graph, key, auditconsumer.NewHandler()); err != nil {
		return fmt.Errorf("provide consumer handler: %w", err)
	}
	if err := di.ProvideNamed[retry.Policy](graph, key, func(di.Resolver) (retry.Policy, error) {
		return retry.NewExponential(retry.ExponentialConfig{
			InitialDelay: cfg.Kafka.AuditRetryInitialDelay,
			MaxDelay:     cfg.Kafka.AuditRetryMaxDelay,
			Multiplier:   2,
		})
	}); err != nil {
		return fmt.Errorf("provide consumer retry policy: %w", err)
	}
	return provideConsumer[[]byte](graph, key)
}
