package types

import (
	"context"
)

// NOTE: In this minimal scaffold we don't define real protobuf files for the
// gasless module yet. If/when you add protobuf definitions under
// api/cosmos/evm/gasless/v1, you should regenerate types and replace this file
// with the generated code.

// For now, define minimal request/response structs for Params to satisfy the
// QueryServer interface used in keeper/query_server.go.

type QueryParamsRequest struct{}

type QueryParamsResponse struct {
	Params Params `json:"params" yaml:"params"`
}

// QueryServer defines the gRPC query service for the gasless module.
type QueryServer interface {
	Params(context.Context, *QueryParamsRequest) (*QueryParamsResponse, error)
}

// RegisterQueryServer is a placeholder for registering the query server.
func RegisterQueryServer(server interface{}, impl QueryServer) {
	// In a real implementation, this would register with grpc.Server
	// For now, this is a no-op placeholder
}
