package rpc

import repository "github.com/goliatone/go-repository-bun"

// RequestMeta carries transport metadata used to enrich crud.Context.
type RequestMeta struct {
	ActorID       string              `json:"actorId,omitempty"`
	Roles         []string            `json:"roles,omitempty"`
	Tenant        string              `json:"tenant,omitempty"`
	RequestID     string              `json:"requestId,omitempty"`
	CorrelationID string              `json:"correlationId,omitempty"`
	Permissions   []string            `json:"permissions,omitempty"`
	Scope         map[string]any      `json:"scope,omitempty"`
	Headers       map[string]string   `json:"headers,omitempty"`
	Params        map[string]string   `json:"params,omitempty"`
	Query         map[string][]string `json:"query,omitempty"`
}

// RequestEnvelope is the canonical RPC request shape.
type RequestEnvelope[T any] struct {
	Data T           `json:"data"`
	Meta RequestMeta `json:"meta,omitempty"`
}

// Error is an RPC-friendly error envelope.
type Error struct {
	Code      string         `json:"code"`
	Message   string         `json:"message"`
	Category  string         `json:"category,omitempty"`
	Retryable bool           `json:"retryable,omitempty"`
	Details   map[string]any `json:"details,omitempty"`
}

// ResponseEnvelope is the canonical RPC response shape.
type ResponseEnvelope[T any] struct {
	Data  T      `json:"data,omitempty"`
	Error *Error `json:"error,omitempty"`
}

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
