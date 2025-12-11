package routerws

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/errcode"
	"github.com/goliatone/go-router"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

const (
	graphqlwsSubprotocol          = "graphql-ws"
	graphqltransportwsSubprotocol = "graphql-transport-ws"
)

var errReadTimeout = errors.New("read timeout")

type (
	// Websocket is a gqlgen-compatible transport that operates on go-router's WebSocketContext
	// instead of gorilla/websocket.Conn. It mirrors gqlgen's built-in websocket transport.
	Websocket struct {
		InitFunc              WebsocketInitFunc
		InitTimeout           time.Duration
		ErrorFunc             WebsocketErrorFunc
		CloseFunc             WebsocketCloseFunc
		KeepAlivePingInterval time.Duration
		PongOnlyInterval      time.Duration
		PingPongInterval      time.Duration
		MissingPongOk         bool
	}

	WebsocketInitFunc  func(ctx context.Context, initPayload InitPayload) (context.Context, *InitPayload, error)
	WebsocketErrorFunc func(ctx context.Context, err error)
	WebsocketCloseFunc func(ctx context.Context, closeCode int)

	messageType int
	message     struct {
		payload json.RawMessage
		id      string
		t       messageType
	}

	messageExchanger interface {
		NextMessage() (message, error)
		Send(m *message) error
	}

	wsConnection struct {
		Websocket
		ctx             context.Context
		ws              router.WebSocketContext
		me              messageExchanger
		active          map[string]context.CancelFunc
		mu              sync.Mutex
		keepAliveTicker *time.Ticker
		pongOnlyTicker  *time.Ticker
		pingPongTicker  *time.Ticker
		receivedPong    bool
		exec            graphql.GraphExecutor
		closed          bool
		headers         http.Header
		initPayload     InitPayload
	}
)

const (
	initMessageType messageType = iota
	connectionAckMessageType
	keepAliveMessageType
	connectionErrorMessageType
	connectionCloseMessageType
	startMessageType
	stopMessageType
	dataMessageType
	completeMessageType
	errorMessageType
	pingMessageType
	pongMessageType
)

// Handler returns a go-router websocket handler bound to the provided executor.
func (t Websocket) Handler(exec graphql.GraphExecutor) func(router.WebSocketContext) error {
	return func(ws router.WebSocketContext) error {
		t.injectSubprotocols(ws)

		me, err := selectMessageExchanger(ws)
		if err != nil {
			_ = ws.CloseWithStatus(router.CloseProtocolError, err.Error())
			return nil
		}

		conn := wsConnection{
			active:    map[string]context.CancelFunc{},
			ws:        ws,
			ctx:       ws.Context(),
			exec:      exec,
			me:        me,
			headers:   headersFromWS(ws),
			Websocket: t,
		}

		if conn.ctx == nil {
			conn.ctx = context.Background()
		}

		if !conn.init() {
			return nil
		}

		conn.run()
		return nil
	}
}

func (t Websocket) injectSubprotocols(ws router.WebSocketContext) {
	for _, subprotocol := range []string{graphqlwsSubprotocol, graphqltransportwsSubprotocol} {
		// Fiber/go-router already negotiates subprotocols during upgrade using config.
		// Here we ensure the current connection reports a supported protocol when the client omits it.
		if ws.Subprotocol() == "" {
			ws.SetContext(context.WithValue(ws.Context(), subprotocolKey{}, subprotocol))
			return
		}
	}
}

type subprotocolKey struct{}

func negotiatedSubprotocol(ctx router.WebSocketContext) string {
	if proto := ctx.Subprotocol(); proto != "" {
		return proto
	}
	if v, ok := ctx.Context().Value(subprotocolKey{}).(string); ok {
		return v
	}
	return ""
}

func selectMessageExchanger(ws router.WebSocketContext) (messageExchanger, error) {
	switch proto := negotiatedSubprotocol(ws); proto {
	case graphqltransportwsSubprotocol:
		return graphqltransportwsMessageExchanger{ws}, nil
	case graphqlwsSubprotocol, "":
		return graphqlwsMessageExchanger{ws}, nil
	default:
		return nil, fmt.Errorf("unsupported negotiated subprotocol %s", proto)
	}
}

func (c *wsConnection) handlePossibleError(err error, isReadError bool) {
	if c.ErrorFunc != nil && err != nil {
		c.ErrorFunc(c.ctx, WebsocketError{
			Err:         err,
			IsReadError: isReadError,
		})
	}
}

func (c *wsConnection) nextMessageWithTimeout(timeout time.Duration) (message, error) {
	messages, errs := make(chan message, 1), make(chan error, 1)

	go func() {
		if m, err := c.me.NextMessage(); err != nil {
			errs <- err
		} else {
			messages <- m
		}
	}()

	select {
	case m := <-messages:
		return m, nil
	case err := <-errs:
		return message{}, err
	case <-time.After(timeout):
		return message{}, errReadTimeout
	}
}

func (c *wsConnection) init() bool {
	var m message
	var err error

	if c.InitTimeout != 0 {
		m, err = c.nextMessageWithTimeout(c.InitTimeout)
	} else {
		m, err = c.me.NextMessage()
	}

	if err != nil {
		if err == errReadTimeout {
			c.close(router.CloseProtocolError, "connection initialisation timeout")
			return false
		}

		if err == errInvalidMsg {
			c.sendConnectionError("invalid json")
		}

		c.close(router.CloseProtocolError, "decoding error")
		return false
	}

	switch m.t {
	case initMessageType:
		if len(m.payload) > 0 {
			c.initPayload = make(InitPayload)
			err := json.Unmarshal(m.payload, &c.initPayload)
			if err != nil {
				return false
			}
		}

		var initAckPayload *InitPayload
		if c.InitFunc != nil {
			var ctx context.Context
			ctx, initAckPayload, err = c.InitFunc(c.ctx, c.initPayload)
			if err != nil {
				c.sendConnectionError("%s", err.Error())
				c.close(router.CloseNormalClosure, "terminated")
				return false
			}
			c.ctx = ctx
		}

		if initAckPayload != nil {
			initJSONAckPayload, err := json.Marshal(*initAckPayload)
			if err != nil {
				panic(err)
			}
			c.write(&message{t: connectionAckMessageType, payload: initJSONAckPayload})
		} else {
			c.write(&message{t: connectionAckMessageType})
		}
		c.write(&message{t: keepAliveMessageType})
	case connectionCloseMessageType:
		c.close(router.CloseNormalClosure, "terminated")
		return false
	default:
		c.sendConnectionError("unexpected message %s", m.t)
		c.close(router.CloseProtocolError, "unexpected message")
		return false
	}

	return true
}

func (c *wsConnection) write(msg *message) {
	c.mu.Lock()
	c.handlePossibleError(c.me.Send(msg), false)
	c.mu.Unlock()
}

func (c *wsConnection) run() {
	ctx, cancel := context.WithCancel(c.ctx)
	defer cancel()

	switch proto := negotiatedSubprotocol(c.ws); proto {
	case "", graphqlwsSubprotocol:
		if c.KeepAlivePingInterval != 0 {
			c.mu.Lock()
			c.keepAliveTicker = time.NewTicker(c.KeepAlivePingInterval)
			c.mu.Unlock()
			go c.keepAlive(ctx)
		}
	case graphqltransportwsSubprotocol:
		if c.PongOnlyInterval != 0 {
			c.mu.Lock()
			c.pongOnlyTicker = time.NewTicker(c.PongOnlyInterval)
			c.mu.Unlock()
			go c.keepAlivePongOnly(ctx)
		}
		if c.PingPongInterval != 0 {
			c.mu.Lock()
			c.pingPongTicker = time.NewTicker(c.PingPongInterval)
			c.mu.Unlock()
			if !c.MissingPongOk {
				_ = c.ws.SetReadDeadline(time.Now().UTC().Add(2 * c.PingPongInterval))
			}
			go c.ping(ctx)
		}
	}

	go c.closeOnCancel(ctx)

	for {
		start := graphql.Now()
		m, err := c.me.NextMessage()
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				c.handlePossibleError(err, true)
			}
			return
		}

		switch m.t {
		case startMessageType:
			c.subscribe(start, &m)
		case stopMessageType:
			c.mu.Lock()
			closer := c.active[m.id]
			c.mu.Unlock()
			if closer != nil {
				closer()
			}
		case connectionCloseMessageType:
			c.close(router.CloseNormalClosure, "terminated")
			return
		case pingMessageType:
			c.write(&message{t: pongMessageType, payload: m.payload})
		case pongMessageType:
			c.mu.Lock()
			c.receivedPong = true
			c.mu.Unlock()
			_ = c.ws.SetReadDeadline(time.Time{})
		default:
			c.sendConnectionError("unexpected message %s", m.t)
			c.close(router.CloseProtocolError, "unexpected message")
			return
		}
	}
}

func (c *wsConnection) keepAlivePongOnly(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.pongOnlyTicker.Stop()
			return
		case <-c.pongOnlyTicker.C:
			c.write(&message{t: pongMessageType, payload: json.RawMessage{}})
		}
	}
}

func (c *wsConnection) keepAlive(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.keepAliveTicker.Stop()
			return
		case <-c.keepAliveTicker.C:
			c.write(&message{t: keepAliveMessageType})
		}
	}
}

func (c *wsConnection) ping(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			c.pingPongTicker.Stop()
			return
		case <-c.pingPongTicker.C:
			c.write(&message{t: pingMessageType, payload: json.RawMessage{}})
			c.mu.Lock()
			if !c.MissingPongOk && c.receivedPong {
				_ = c.ws.SetReadDeadline(time.Now().UTC().Add(2 * c.PingPongInterval))
			}
			c.receivedPong = false
			c.mu.Unlock()
		}
	}
}

func (c *wsConnection) closeOnCancel(ctx context.Context) {
	<-ctx.Done()

	if r := closeReasonForContext(ctx); r != "" {
		c.sendConnectionError("%s", r)
	}
	c.close(router.CloseNormalClosure, "terminated")
}

func (c *wsConnection) subscribe(start time.Time, msg *message) {
	ctx := graphql.StartOperationTrace(c.ctx)
	var params *graphql.RawParams
	if err := jsonDecode(bytes.NewReader(msg.payload), &params); err != nil {
		c.sendError(msg.id, &gqlerror.Error{Message: "invalid json"})
		c.complete(msg.id)
		return
	}

	params.ReadTime = graphql.TraceTiming{
		Start: start,
		End:   graphql.Now(),
	}
	params.Headers = c.headers

	rc, err := c.exec.CreateOperationContext(ctx, params)
	if err != nil {
		resp := c.exec.DispatchError(graphql.WithOperationContext(ctx, rc), err)
		switch errcode.GetErrorKind(err) {
		case errcode.KindProtocol:
			c.sendError(msg.id, resp.Errors...)
		default:
			c.sendResponse(msg.id, &graphql.Response{Errors: err})
		}

		c.complete(msg.id)
		return
	}

	ctx = graphql.WithOperationContext(ctx, rc)

	if c.initPayload != nil {
		ctx = withInitPayload(ctx, c.initPayload)
	}

	ctx, cancel := context.WithCancel(ctx)
	c.mu.Lock()
	c.active[msg.id] = cancel
	c.mu.Unlock()

	go func() {
		ctx = withSubscriptionErrorContext(ctx)
		defer func() {
			if r := recover(); r != nil {
				err := rc.Recover(ctx, r)
				var gqlerr *gqlerror.Error
				if !errors.As(err, &gqlerr) {
					gqlerr = &gqlerror.Error{}
					if err != nil {
						gqlerr.Message = err.Error()
					}
				}
				c.sendError(msg.id, gqlerr)
			}
			if errs := getSubscriptionError(ctx); len(errs) != 0 {
				c.sendError(msg.id, errs...)
			} else {
				c.complete(msg.id)
			}
			c.mu.Lock()
			delete(c.active, msg.id)
			c.mu.Unlock()
			cancel()
		}()

		responses, ctx := c.exec.DispatchOperation(ctx, rc)
		for {
			response := responses(ctx)
			if response == nil {
				break
			}

			c.sendResponse(msg.id, response)
		}
	}()
}

func (c *wsConnection) sendResponse(id string, response *graphql.Response) {
	b, err := json.Marshal(response)
	if err != nil {
		panic(err)
	}
	c.write(&message{
		payload: b,
		id:      id,
		t:       dataMessageType,
	})
}

func (c *wsConnection) complete(id string) {
	c.write(&message{id: id, t: completeMessageType})
}

func (c *wsConnection) sendError(id string, errors ...*gqlerror.Error) {
	errs := make([]error, len(errors))
	for i, err := range errors {
		errs[i] = err
	}
	b, err := json.Marshal(errs)
	if err != nil {
		panic(err)
	}
	c.write(&message{t: errorMessageType, id: id, payload: b})
}

func (c *wsConnection) sendConnectionError(format string, args ...any) {
	b, err := json.Marshal(&gqlerror.Error{Message: fmt.Sprintf(format, args...)})
	if err != nil {
		panic(err)
	}

	c.write(&message{t: connectionErrorMessageType, payload: b})
}

func (c *wsConnection) close(closeCode int, message string) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	for _, closer := range c.active {
		closer()
	}
	c.closed = true
	c.mu.Unlock()
	_ = c.ws.CloseWithStatus(closeCode, message)

	if c.CloseFunc != nil {
		c.CloseFunc(c.ctx, closeCode)
	}
}

// InitPayload is parsed from the websocket init message payload.
type InitPayload map[string]any

// GetString safely gets a string value from the payload.
func (p InitPayload) GetString(key string) string {
	if p == nil {
		return ""
	}

	if value, ok := p[key]; ok {
		res, _ := value.(string)
		return res
	}

	return ""
}

// Authorization is a short hand for getting the Authorization header from the payload.
func (p InitPayload) Authorization() string {
	if value := p.GetString("Authorization"); value != "" {
		return value
	}

	if value := p.GetString("authorization"); value != "" {
		return value
	}

	return ""
}

func withInitPayload(ctx context.Context, payload InitPayload) context.Context {
	return context.WithValue(ctx, initpayload, payload)
}

// GetInitPayload gets a map of the data sent with the connection_init message.
func GetInitPayload(ctx context.Context) InitPayload {
	payload, ok := ctx.Value(initpayload).(InitPayload)
	if !ok {
		return nil
	}

	return payload
}

type key string

const (
	initpayload key = "ws_initpayload_context"
)

// AddSubscriptionError is used to let websocket return an error message after subscription resolver returns a channel.
func AddSubscriptionError(ctx context.Context, err *gqlerror.Error) {
	subscriptionErrStruct := getSubscriptionErrorStruct(ctx)
	subscriptionErrStruct.errs = append(subscriptionErrStruct.errs, err)
}

func withSubscriptionErrorContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, wsSubscriptionErrorCtxKey, &subscriptionError{})
}

func getSubscriptionErrorStruct(ctx context.Context) *subscriptionError {
	v, _ := ctx.Value(wsSubscriptionErrorCtxKey).(*subscriptionError)
	return v
}

func getSubscriptionError(ctx context.Context) []*gqlerror.Error {
	return getSubscriptionErrorStruct(ctx).errs
}

var wsSubscriptionErrorCtxKey = &wsSubscriptionErrorContextKey{"subscription-error"}

type wsSubscriptionErrorContextKey struct {
	name string
}

type subscriptionError struct {
	errs []*gqlerror.Error
}

var closeReasonCtxKey = &wsCloseReasonContextKey{"close-reason"}

type wsCloseReasonContextKey struct {
	name string
}

func AppendCloseReason(ctx context.Context, reason string) context.Context {
	return context.WithValue(ctx, closeReasonCtxKey, reason)
}

func closeReasonForContext(ctx context.Context) string {
	reason, _ := ctx.Value(closeReasonCtxKey).(string)
	return reason
}

func jsonDecode(r *bytes.Reader, val any) error {
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec.Decode(val)
}

func headersFromWS(ws router.WebSocketContext) http.Header {
	h := http.Header{}
	if auth := strings.TrimSpace(ws.Header("Authorization")); auth != "" {
		h.Set("Authorization", auth)
	}
	if proto := ws.Subprotocol(); proto != "" {
		h.Set("Sec-Websocket-Protocol", proto)
	}
	return h
}

type WebsocketError struct {
	Err error

	// IsReadError flags whether the error occurred on read or write to the websocket
	IsReadError bool
}

func (e WebsocketError) Error() string {
	if e.IsReadError {
		return fmt.Sprintf("websocket read: %v", e.Err)
	}
	return fmt.Sprintf("websocket write: %v", e.Err)
}

var (
	errInvalidMsg = errors.New("invalid message received")
)

// graphql-ws (legacy) message exchanger using go-router WebSocketContext.
type graphqlwsMessageExchanger struct {
	c router.WebSocketContext
}

type graphqlwsMessage struct {
	Payload json.RawMessage      `json:"payload,omitempty"`
	ID      string               `json:"id,omitempty"`
	Type    graphqlwsMessageType `json:"type"`
	noOp    bool
}

type graphqlwsMessageType string

var allGraphqlwsMessageTypes = []graphqlwsMessageType{
	graphqlwsConnectionInitMsg,
	graphqlwsConnectionTerminateMsg,
	graphqlwsStartMsg,
	graphqlwsStopMsg,
	graphqlwsConnectionAckMsg,
	graphqlwsConnectionErrorMsg,
	graphqlwsDataMsg,
	graphqlwsErrorMsg,
	graphqlwsCompleteMsg,
	graphqlwsConnectionKeepAliveMsg,
}

const (
	graphqlwsConnectionInitMsg      = graphqlwsMessageType("connection_init")
	graphqlwsConnectionTerminateMsg = graphqlwsMessageType("connection_terminate")
	graphqlwsStartMsg               = graphqlwsMessageType("start")
	graphqlwsStopMsg                = graphqlwsMessageType("stop")
	graphqlwsConnectionAckMsg       = graphqlwsMessageType("connection_ack")
	graphqlwsConnectionErrorMsg     = graphqlwsMessageType("connection_error")
	graphqlwsDataMsg                = graphqlwsMessageType("data")
	graphqlwsErrorMsg               = graphqlwsMessageType("error")
	graphqlwsCompleteMsg            = graphqlwsMessageType("complete")
	graphqlwsConnectionKeepAliveMsg = graphqlwsMessageType("ka")
)

func (me graphqlwsMessageExchanger) NextMessage() (message, error) {
	_, data, err := me.c.ReadMessage()
	if err != nil {
		return message{}, err
	}

	var graphqlwsMsg graphqlwsMessage
	if err := json.Unmarshal(data, &graphqlwsMsg); err != nil {
		return message{}, errInvalidMsg
	}

	return graphqlwsMsg.toMessage()
}

func (me graphqlwsMessageExchanger) Send(m *message) error {
	msg := &graphqlwsMessage{}
	if err := msg.fromMessage(m); err != nil {
		return err
	}

	if msg.noOp {
		return nil
	}

	return me.c.WriteJSON(msg)
}

func (m graphqlwsMessage) toMessage() (message, error) {
	var t messageType
	var err error
	switch m.Type {
	default:
		err = fmt.Errorf("invalid client->server message type %s", m.Type)
	case graphqlwsConnectionInitMsg:
		t = initMessageType
	case graphqlwsConnectionTerminateMsg:
		t = connectionCloseMessageType
	case graphqlwsStartMsg:
		t = startMessageType
	case graphqlwsStopMsg:
		t = stopMessageType
	case graphqlwsConnectionAckMsg:
		t = connectionAckMessageType
	case graphqlwsConnectionErrorMsg:
		t = connectionErrorMessageType
	case graphqlwsDataMsg:
		t = dataMessageType
	case graphqlwsErrorMsg:
		t = errorMessageType
	case graphqlwsCompleteMsg:
		t = completeMessageType
	case graphqlwsConnectionKeepAliveMsg:
		t = keepAliveMessageType
	}

	return message{
		payload: m.Payload,
		id:      m.ID,
		t:       t,
	}, err
}

func (m *graphqlwsMessage) fromMessage(msg *message) (err error) {
	m.ID = msg.id
	m.Payload = msg.payload

	switch msg.t {
	default:
		err = fmt.Errorf("invalid server->client message type %s", msg.t)
	case initMessageType:
		m.Type = graphqlwsConnectionInitMsg
	case connectionAckMessageType:
		m.Type = graphqlwsConnectionAckMsg
	case keepAliveMessageType:
		m.Type = graphqlwsConnectionKeepAliveMsg
	case connectionErrorMessageType:
		m.Type = graphqlwsConnectionErrorMsg
	case connectionCloseMessageType:
		m.Type = graphqlwsConnectionTerminateMsg
	case startMessageType:
		m.Type = graphqlwsStartMsg
	case stopMessageType:
		m.Type = graphqlwsStopMsg
	case dataMessageType:
		m.Type = graphqlwsDataMsg
	case completeMessageType:
		m.Type = graphqlwsCompleteMsg
	case errorMessageType:
		m.Type = graphqlwsErrorMsg
	case pingMessageType:
		m.noOp = true
	case pongMessageType:
		m.noOp = true
	}

	return err
}

// graphql-transport-ws message exchanger using go-router WebSocketContext.
type graphqltransportwsMessageExchanger struct {
	c router.WebSocketContext
}

type graphqltransportwsMessage struct {
	Payload json.RawMessage               `json:"payload,omitempty"`
	ID      string                        `json:"id,omitempty"`
	Type    graphqltransportwsMessageType `json:"type"`
	noOp    bool
}

type graphqltransportwsMessageType string

var allGraphqltransportwsMessageTypes = []graphqltransportwsMessageType{
	graphqltransportwsConnectionInitMsg,
	graphqltransportwsConnectionAckMsg,
	graphqltransportwsSubscribeMsg,
	graphqltransportwsNextMsg,
	graphqltransportwsErrorMsg,
	graphqltransportwsCompleteMsg,
	graphqltransportwsPingMsg,
	graphqltransportwsPongMsg,
}

const (
	graphqltransportwsConnectionInitMsg = graphqltransportwsMessageType("connection_init")
	graphqltransportwsConnectionAckMsg  = graphqltransportwsMessageType("connection_ack")
	graphqltransportwsSubscribeMsg      = graphqltransportwsMessageType("subscribe")
	graphqltransportwsNextMsg           = graphqltransportwsMessageType("next")
	graphqltransportwsErrorMsg          = graphqltransportwsMessageType("error")
	graphqltransportwsCompleteMsg       = graphqltransportwsMessageType("complete")
	graphqltransportwsPingMsg           = graphqltransportwsMessageType("ping")
	graphqltransportwsPongMsg           = graphqltransportwsMessageType("pong")
)

func (me graphqltransportwsMessageExchanger) NextMessage() (message, error) {
	_, data, err := me.c.ReadMessage()
	if err != nil {
		return message{}, err
	}

	var msg graphqltransportwsMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		return message{}, errInvalidMsg
	}

	return msg.toMessage()
}

func (me graphqltransportwsMessageExchanger) Send(m *message) error {
	msg := &graphqltransportwsMessage{}
	if err := msg.fromMessage(m); err != nil {
		return err
	}

	if msg.noOp {
		return nil
	}

	return me.c.WriteJSON(msg)
}

func (m graphqltransportwsMessage) toMessage() (message, error) {
	var t messageType
	var err error
	switch m.Type {
	default:
		err = fmt.Errorf("invalid client->server message type %s", m.Type)
	case graphqltransportwsConnectionInitMsg:
		t = initMessageType
	case graphqltransportwsSubscribeMsg:
		t = startMessageType
	case graphqltransportwsCompleteMsg:
		t = stopMessageType
	case graphqltransportwsPingMsg:
		t = pingMessageType
	case graphqltransportwsPongMsg:
		t = pongMessageType
	}

	return message{
		payload: m.Payload,
		id:      m.ID,
		t:       t,
	}, err
}

func (m *graphqltransportwsMessage) fromMessage(msg *message) (err error) {
	m.ID = msg.id
	m.Payload = msg.payload

	switch msg.t {
	default:
		err = fmt.Errorf("invalid server->client message type %s", msg.t)
	case connectionAckMessageType:
		m.Type = graphqltransportwsConnectionAckMsg
	case keepAliveMessageType:
		m.noOp = true
	case connectionErrorMessageType:
		m.noOp = true
	case dataMessageType:
		m.Type = graphqltransportwsNextMsg
	case completeMessageType:
		m.Type = graphqltransportwsCompleteMsg
	case errorMessageType:
		m.Type = graphqltransportwsErrorMsg
	case pingMessageType:
		m.Type = graphqltransportwsPingMsg
	case pongMessageType:
		m.Type = graphqltransportwsPongMsg
	}

	return err
}
