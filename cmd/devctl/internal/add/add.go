package add

import "github.com/urfave/cli/v3"

// NewCmd constructs the namespace for project resource additions.
func NewCmd() *cli.Command {
	return &cli.Command{
		Name:        "add",
		Usage:       "Add a named Project resource",
		Description: "Add or update a named Source, client, Kafka endpoint, database Variant, Redis Connection, S3 Connection, or S3 bucket in the Manifest. This command changes only devctl.yaml.",
		Commands: []*cli.Command{
			newDBCmd(dbCmdOpts{}, buildDB),
			newSourceCmd(sourceCmdOpts{}, buildSource),
			newHTTPClientCmd(httpClientCmdOpts{}, buildHTTPClient),
			newGRPCClientCmd(grpcClientCmdOpts{}, buildGRPCClient),
			newKafkaConsumerCmd(kafkaCmdOpts{}, buildKafka),
			newKafkaProducerCmd(kafkaCmdOpts{}, buildKafka),
			newRedisCmd(storageCmdOpts{}, buildStorage),
			newS3ConnectionCmd(storageCmdOpts{}, buildStorage),
			newS3Cmd(storageCmdOpts{}, buildStorage),
		},
	}
}
