module example.test/sample-api

go 1.26.0

require (
	github.com/ClickHouse/clickhouse-go/v2 v2.48.0
	github.com/aws/aws-sdk-go-v2 v1.45.1
	github.com/aws/aws-sdk-go-v2/config v1.33.1
	github.com/aws/aws-sdk-go-v2/credentials v1.20.1
	github.com/aws/aws-sdk-go-v2/service/s3 v1.109.1
	github.com/bufbuild/buf v1.72.0
	github.com/devctllabs/go-libs/config v0.1.0
	github.com/devctllabs/go-libs/debugserver v0.1.0
	github.com/devctllabs/go-libs/di v0.1.0
	github.com/devctllabs/go-libs/health v0.1.0
	github.com/devctllabs/go-libs/healthserver v0.1.0
	github.com/devctllabs/go-libs/kafka v0.1.0
	github.com/devctllabs/go-libs/kafkaproto v0.1.0
	github.com/devctllabs/go-libs/lifecycle v0.2.0
	github.com/devctllabs/go-libs/log v0.2.0
	github.com/devctllabs/go-libs/oapivalidator v0.2.0
	github.com/devctllabs/go-libs/postgresdb v0.2.0
	github.com/devctllabs/go-libs/retry v0.1.0
	github.com/devctllabs/go-libs/sqlitedb v0.1.0
	github.com/devctllabs/go-libs/telemetry v0.1.0
	github.com/devctllabs/go-libs/txmanager v0.1.0
	github.com/labstack/echo/v5 v5.3.1
	github.com/redis/go-redis/v9 v9.22.0
	github.com/twmb/franz-go v1.21.6
	github.com/urfave/cli/v3 v3.10.1
	go.uber.org/zap v1.28.0
	google.golang.org/grpc v1.83.2
	google.golang.org/grpc/cmd/protoc-gen-go-grpc v1.6.2
	google.golang.org/protobuf v1.36.12
)

tool github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen

require github.com/oapi-codegen/oapi-codegen/v2 v2.8.0

tool (
	github.com/bufbuild/buf/cmd/buf
	google.golang.org/grpc/cmd/protoc-gen-go-grpc
	google.golang.org/protobuf/cmd/protoc-gen-go
)
