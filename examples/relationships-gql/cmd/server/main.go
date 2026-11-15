package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql"
	"github.com/99designs/gqlgen/graphql/executor"
	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	fiberws "github.com/gofiber/contrib/v3/websocket"
	"github.com/gofiber/fiber/v3"
	"github.com/goliatone/go-auth"
	"github.com/goliatone/go-crud/gql/helpers"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"
	"github.com/uptrace/bun"

	relationships "github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/dataloader"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/generated"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/resolvers"
	"github.com/goliatone/go-crud/examples/relationships-gql/internal/loader"
	"github.com/goliatone/go-crud/examples/relationships-gql/internal/routerws"
)

func main() {
	if err := run(context.Background()); err != nil {
		log.Fatalf("relationships-gql server failed: %v", err)
	}
}

func run(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	client, err := relationships.SetupDatabase(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if client == nil {
			return
		}
		if db := client.DB(); db != nil {
			_ = db.Close()
		}
	}()

	db := client.DB()

	repos := relationships.RegisterRepositories(db)
	if err := relationships.MigrateSchema(ctx, db); err != nil {
		return err
	}
	if err := relationships.SeedDatabase(ctx, client); err != nil {
		return err
	}

	resolver := resolvers.NewResolver(repos)
	app, sockets, err := newServer(resolver, db)
	if err != nil {
		return err
	}
	addr := os.Getenv("RELATIONSHIPS_GQL_ADDR")
	if addr == "" {
		addr = ":9091"
	}
	return serve(ctx, app.(router.ManagedServer), sockets, addr, 5*time.Second)
}

func newServer(resolver *resolvers.Resolver, db *bun.DB, configure ...func(*router.WebSocketConfig)) (router.Server[*fiber.App], *socketOwner, error) {
	const upgradeAuthKey = "authorization"

	resolver.ScopeGuard = func(ctx context.Context, entity, action string) error {
		user, ok := auth.FromContext(ctx)
		if !ok || user == nil {
			return errors.New("unauthorized")
		}
		_ = entity
		_ = action
		return nil
	}

	httpSrv, wsExec := buildGraphQLServer(resolver)
	secured := loader.Middleware(resolver, db)(graphqlAuthMiddleware(httpSrv))
	app := router.NewFiberV3AdapterWithConfig(router.FiberV3AdapterConfig{ListenConfig: fiber.ListenConfig{DisableStartupMessage: true}})
	sockets := newSocketOwner()

	wsConfig := router.DefaultWebSocketConfig()
	wsConfig.Subprotocols = []string{"graphql-transport-ws", "graphql-ws"}
	wsConfig.WriteTimeout = time.Second
	wsConfig.PingPeriod = 15 * time.Second
	wsConfig.PongWait = 30 * time.Second
	wsConfig.CheckOrigin = func(origin string) bool { return true }
	wsConfig.OnPreUpgrade = func(c router.Context) (router.UpgradeData, error) {
		if authUserFromHeader(c.Header("Authorization")) == nil {
			return nil, router.ErrUnauthorized
		}
		return router.UpgradeData{
			upgradeAuthKey: c.Header("Authorization"),
		}, nil
	}

	for _, option := range configure {
		option(&wsConfig)
	}

	wsTransport := routerws.Websocket{
		KeepAlivePingInterval: 15 * time.Second,
		PingPongInterval:      0,
		PongOnlyInterval:      0,
		MissingPongOk:         true,
	}
	wsHandler := wsTransport.Handler(wsExec)

	wsFiberHandler := router.FiberV3WebSocketHandler(wsConfig, sockets.wrap(func(ws router.WebSocketContext) error {
		ctx := ws.Context()
		headers := map[string]string{}

		var authHeader string
		if v, ok := ws.UpgradeData(upgradeAuthKey); ok {
			authHeader, _ = v.(string)
		}
		if user := authUserFromHeader(authHeader); user != nil {
			ctx = auth.WithContext(ctx, user)
			ctx = auth.WithActorContext(ctx, &auth.ActorContext{
				ActorID: user.ID.String(),
				Subject: user.Username,
				Role:    string(user.Role),
			})
			if authHeader != "" {
				headers["authorization"] = authHeader
			}
		}

		if ldr := loader.New(resolver, db); ldr != nil {
			ctx = dataloader.Inject(ctx, ldr)
		}

		ctx, scope := helpers.WithSubscriptionScope(ctx)
		defer scope.Close(context.Background())
		return wsHandler(newWebsocketContext(ctx, headers, ws))
	}))

	// Native upgrades are registered through the owned capability before routes.
	err := app.(router.FiberV3NativeMiddlewareRegistrar).UseNative(func(c fiber.Ctx) error {
		if strings.EqualFold(strings.TrimRight(c.Path(), "/"), "/graphql") && fiberws.IsWebSocketUpgrade(c) {
			return wsFiberHandler(c)
		}
		return c.Next()
	})
	if err != nil {
		return nil, nil, err
	}
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodConnect, http.MethodOptions, http.MethodTrace, http.MethodPatch} {
		app.Router().Handle(router.HTTPMethod(method), "/graphql", router.HandlerFromHTTP(secured))
	}
	app.Router().Get("/playground", router.HandlerFromHTTP(playground.Handler("GraphQL playground", "/graphql")))
	app.Router().Get("/", func(c router.Context) error { return c.Redirect("/playground", http.StatusTemporaryRedirect) })
	return app, sockets, nil
}

func buildGraphQLServer(resolver *resolvers.Resolver) (*handler.Server, graphql.GraphExecutor) {
	executableSchema := generated.NewExecutableSchema(generated.Config{Resolvers: resolver})

	srv := handler.New(executableSchema)
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.Use(extension.Introspection{})

	exec := executor.New(executableSchema)
	return srv, exec
}

// graphqlAuthMiddleware attaches a demo auth user to the request context when a bearer token is present.
func graphqlAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := authUserFromRequest(r)
		if user != nil {
			ctx := auth.WithContext(r.Context(), user)
			ctx = auth.WithActorContext(ctx, &auth.ActorContext{
				ActorID: user.ID.String(),
				Subject: user.Username,
				Role:    string(user.Role),
			})
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}

func authUserFromRequest(r *http.Request) *auth.User {
	return authUserFromHeader(r.Header.Get("Authorization"))
}

func authUserFromHeader(header string) *auth.User {
	header = strings.TrimSpace(header)
	const bearer = "Bearer "
	if header == "" || !strings.HasPrefix(header, bearer) {
		return nil
	}

	token := strings.TrimSpace(strings.TrimPrefix(header, bearer))
	if token == "" {
		return nil
	}

	return &auth.User{
		ID:       uuid.NewSHA1(uuid.Nil, []byte(token)),
		Username: token,
		Role:     auth.RoleMember,
		Status:   auth.UserStatusActive,
	}
}

type websocketContext struct {
	router.WebSocketContext
	ctx     context.Context
	headers map[string]string
}

func newWebsocketContext(ctx context.Context, headers map[string]string, ws router.WebSocketContext) *websocketContext {
	if ctx == nil {
		ctx = context.Background()
	}
	return &websocketContext{
		WebSocketContext: ws,
		ctx:              ctx,
		headers:          headers,
	}
}

func (w *websocketContext) Context() context.Context {
	if w.ctx != nil {
		return w.ctx
	}
	return context.Background()
}

func (w *websocketContext) SetContext(ctx context.Context) {
	w.ctx = ctx
}

func (w *websocketContext) Header(key string) string {
	if w.headers == nil {
		return ""
	}
	return w.headers[strings.ToLower(key)]
}
