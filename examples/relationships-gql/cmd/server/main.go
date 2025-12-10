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
	fiberws "github.com/gofiber/contrib/websocket"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/goliatone/go-auth"
	"github.com/goliatone/go-router"
	"github.com/google/uuid"

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

	const upgradeAuthKey = "authorization"

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

	app := router.NewFiberAdapter()
	_ = app.Router() // ensure router is initialized before Serve()

	wsConfig := router.DefaultWebSocketConfig()
	wsConfig.Subprotocols = []string{"graphql-transport-ws", "graphql-ws"}
	wsConfig.PingPeriod = 15 * time.Second
	wsConfig.PongWait = 30 * time.Second
	wsConfig.CheckOrigin = func(origin string) bool { return true }
	wsConfig.OnPreUpgrade = func(c router.Context) (router.UpgradeData, error) {
		return router.UpgradeData{
			upgradeAuthKey: c.Header("Authorization"),
		}, nil
	}

	wsTransport := routerws.Websocket{
		KeepAlivePingInterval: 15 * time.Second,
		PingPongInterval:      0,
		PongOnlyInterval:      0,
		MissingPongOk:         true,
	}
	wsHandler := wsTransport.Handler(wsExec)

	wsFiberHandler := router.FiberWebSocketHandler(wsConfig, func(ws router.WebSocketContext) error {
		ctx := context.Background()
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

		return wsHandler(newWebsocketContext(ctx, headers, ws))
	})

	fiberApp := app.WrappedRouter()
	fiberApp.All("/graphql", func(c *fiber.Ctx) error {
		if fiberws.IsWebSocketUpgrade(c) {
			return wsFiberHandler(c)
		}
		return adaptor.HTTPHandler(secured)(c)
	})
	fiberApp.Get("/playground", adaptor.HTTPHandler(playground.Handler("GraphQL playground", "/graphql")))
	fiberApp.Get("/", func(c *fiber.Ctx) error {
		return c.Redirect("/playground", fiber.StatusTemporaryRedirect)
	})

	addr := ":9091"
	log.Printf("GraphQL endpoint ready at http://localhost%s/graphql (HTTP & WebSocket)", addr)
	log.Printf("Playground available at http://localhost%s/playground", addr)

	errCh := make(chan error, 1)
	go func() {
		errCh <- app.Serve(addr)
	}()

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	}

	log.Println("Shutting down server...")
	if err := app.Shutdown(context.Background()); err != nil {
		return err
	}
	return nil
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
