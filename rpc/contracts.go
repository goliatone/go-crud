package rpc

import (
	commandrpc "github.com/goliatone/go-command/rpc"
	repository "github.com/goliatone/go-repository-bun"
)

// Shared go-command/rpc envelope contracts.
type RequestMeta = commandrpc.RequestMeta
type RequestEnvelope[T any] = commandrpc.RequestEnvelope[T]
type Error = commandrpc.Error
type ResponseEnvelope[T any] = commandrpc.ResponseEnvelope[T]

type CreateData[T any] struct {
	Record T `json:"record"`
}

type CreateBatchData[T any] struct {
	Records []T `json:"records"`
}

type UpdateData[T any] struct {
	ID     string `json:"id"`
	Record T      `json:"record"`
}

type UpdateBatchData[T any] struct {
	Records []T `json:"records"`
}

type DeleteData struct {
	ID string `json:"id"`
}

type DeleteBatchData[T any] struct {
	Records []T      `json:"records,omitempty"`
	IDs     []string `json:"ids,omitempty"`
}

type ShowData struct {
	ID       string                      `json:"id"`
	Criteria []repository.SelectCriteria `json:"criteria,omitempty"`
}

type IndexData[Opts any] struct {
	Options  Opts                        `json:"options,omitempty"`
	Criteria []repository.SelectCriteria `json:"criteria,omitempty"`
}

type ListResult[T any] struct {
	Items []T `json:"items"`
	Count int `json:"count"`
}

type DeleteResult struct {
	Deleted bool `json:"deleted"`
}

type DeleteBatchResult struct {
	Count int `json:"count"`
}
