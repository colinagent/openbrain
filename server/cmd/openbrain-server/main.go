package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	"github.com/colinagent/openbrain/opagent-protocol/go-sdk/op"
	sharedmarketplace "github.com/colinagent/openbrain/opagent-runtime/marketplace"
	"github.com/colinagent/openbrain/server/internal/server/cache"
	"github.com/colinagent/openbrain/server/internal/server/chat"
	"github.com/colinagent/openbrain/server/internal/server/chatindex"
	gbrainserver "github.com/colinagent/openbrain/server/internal/server/gbrain"
	hostcfg "github.com/colinagent/openbrain/server/internal/server/hostcfg"
	"github.com/colinagent/openbrain/server/internal/server/notify"
	"github.com/colinagent/openbrain/server/internal/server/resources"
	"github.com/colinagent/openbrain/server/internal/server/sse"
	"github.com/colinagent/openbrain/server/internal/server/transfer"
	"github.com/colinagent/openbrain/server/internal/server/treeimport"
	"github.com/colinagent/openbrain/server/internal/server/ws"
	"github.com/gin-gonic/gin"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Version is the server version string (overridden at build time).
var Version = "dev"

func main() {
	host := flag.String("host", "127.0.0.1", "bind host")
	port := flag.Int("port", 19530, "bind port")
	verbose := flag.Bool("v", false, "verbose logging")
	flag.Parse()

	addr := fmt.Sprintf("%s:%d", *host, *port)
	ws.Version = Version

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	sseManager := sse.NewManager()
	go sseManager.Start(ctx)

	notifySvc := notify.NewService(sseManager)
	chatSvc := chat.NewService(notifySvc)
	wsServer := ws.NewServer(addr, *verbose)
	notifySvc.SetMessengerBroadcast(wsServer.BroadcastMessengerMessage)
	chatHandler := chat.NewHandler(sseManager, chatSvc)

	opServer := op.NewServer(&op.Implementation{Name: "openbrain-server"}, &op.ServerOptions{
		InitializedHandler: func(ctx context.Context, req *op.InitializedRequest) {
			log.Printf("OpAgent session initialized")
			hostcfg.Init(req.Session)
			chatSvc.SetHostSession(req.Session)
			wsServer.SetHostSession(req.Session)
		},
		InfoNotificationHandler: func(ctx context.Context, req *op.InfoNotificationServerRequest) {
			chatSvc.HandleHostNotification(req)
			notifySvc.HandleHostNotification(req)
		},
	})

	go func() {
		if err := opServer.Run(ctx, &op.StdioTransport{}); err != nil {
			log.Printf("op server exited: %v", err)
		}
		// If stdio session ends (parent disconnected), shutdown this process so
		// pid lock and listening port are released for the next restart.
		stop()
	}()

	// Wait for host initialisation (InitializedHandler called).
	log.Printf("waiting for host initialisation...")
	if err := hostcfg.WaitReady(ctx); err != nil {
		log.Fatalf("host init cancelled: %v", err)
	}

	// Fetch config from host and set up logging + paths.
	hostCfg, err := setupFromHostConfig(notifySvc)
	if err != nil {
		log.Fatalf("setupFromHostConfig: %v", err)
	}

	wsServer.GetHandler().SetMarketplaceService(sharedmarketplace.NewService(hostCfg.System.BaseDir, sharedmarketplace.Options{}))

	transferHandler := transfer.NewHandler(transfer.NewService(hostCfg.System.BaseDir))
	resourceService := resources.NewService(hostCfg.System.BaseDir, chatSvc)
	resourceHandler := resources.NewHandler(resourceService)
	treeImportHandler := treeimport.NewHandler(treeimport.NewService(hostCfg.System.BaseDir, resourceService))
	gbrainHandler := gbrainserver.NewHandler(gbrainserver.NewService(hostCfg.System.BaseDir))

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(recoveryWithSlog())
	router.Use(requestLogMiddleware(*verbose))
	router.Use(corsMiddleware())

	wsServer.RegisterHandlers(router)
	createHandler := chat.NewCreateHandler(chatSvc)
	forkHandler := chat.NewForkHandler(chatSvc)
	metaHandler := chat.NewMetaHandler(chatSvc)
	retitleHandler := chat.NewRetitleHandler(chatSvc)
	router.POST("/v1/thread/create", createHandler.Create)
	router.POST("/v1/thread/fork", forkHandler.Fork)
	router.POST("/v1/thread/retitle", retitleHandler.Retitle)
	router.POST("/v1/chat/stream", chatHandler.Stream)
	router.POST("/v1/chat/control", chatHandler.Control)
	router.GET("/v1/thread/meta", metaHandler.Get)
	router.GET("/v1/thread/snapshot", metaHandler.Snapshot)
	router.POST("/v1/thread/meta", metaHandler.Update)
	router.POST("/v1/transfers", transferHandler.Create)
	router.PUT("/v1/transfers/:id/content", transferHandler.PutContent)
	router.GET("/v1/transfers/:id/content", transferHandler.GetContent)
	router.GET("/v1/transfers/:id/meta", transferHandler.GetMeta)

	router.POST("/v1/resources/grants", resourceHandler.CreateGrant)
	router.POST("/v1/resources/inspect", resourceHandler.Inspect)
	router.POST("/v1/resources/handle", resourceHandler.CreateHandle)
	router.GET("/v1/resources/content/:handleId", resourceHandler.GetContent)
	router.GET("/v1/resources/content/:handleId/*resourcePath", resourceHandler.GetPackageContent)
	router.POST("/v1/resources/import-sessions", resourceHandler.CreateImportSession)
	router.PUT("/v1/resources/import-sessions/:sessionId/content", resourceHandler.UploadImportSession)
	router.POST("/v1/tree-import/sessions", treeImportHandler.CreateSession)
	router.PUT("/v1/tree-import/sessions/:sessionId/files/*relativePath", treeImportHandler.UploadFile)
	router.POST("/v1/tree-import/sessions/:sessionId/commit", treeImportHandler.CommitSession)
	router.DELETE("/v1/tree-import/sessions/:sessionId", treeImportHandler.CancelSession)
	router.GET("/v1/openbrain/status", gbrainHandler.Status)
	router.GET("/v1/openbrain/sources", gbrainHandler.ListSources)
	router.GET("/v1/openbrain/cached-sources", gbrainHandler.CachedListSources)
	router.POST("/v1/openbrain/query", gbrainHandler.Query)
	router.GET("/v1/openbrain/cloud/sources", gbrainHandler.CloudListSources)
	router.POST("/v1/openbrain/cloud/query", gbrainHandler.CloudQuery)
	router.POST("/v1/openbrain/cloud/sources", gbrainHandler.CloudCreateSource)
	router.POST("/v1/openbrain/cloud/sources/verify", gbrainHandler.CloudVerifySource)
	router.POST("/v1/openbrain/cloud/sources/recovery-candidates", gbrainHandler.CloudSourceRecoveryCandidates)
	router.POST("/v1/openbrain/cloud/sources/remove", gbrainHandler.CloudRemoveSourceFromDevice)
	router.POST("/v1/openbrain/cloud/sources/archive", gbrainHandler.CloudArchiveSource)
	router.POST("/v1/openbrain/cloud/sources/action", gbrainHandler.CloudSourceAction)
	router.GET("/v1/openbrain/cloud/orgs/:orgID/resources/:resourceID/source-share", gbrainHandler.CloudGetSourceShare)
	router.PUT("/v1/openbrain/cloud/orgs/:orgID/resources/:resourceID/source-share/users", gbrainHandler.CloudShareSourceWithUser)
	router.DELETE("/v1/openbrain/cloud/orgs/:orgID/resources/:resourceID/source-share/users/:uid", gbrainHandler.CloudRevokeSourceUserShare)
	router.PUT("/v1/openbrain/cloud/orgs/:orgID/resources/:resourceID/source-share/public", gbrainHandler.CloudSetSourcePublic)
	router.DELETE("/v1/openbrain/cloud/orgs/:orgID/resources/:resourceID/source-share/public", gbrainHandler.CloudRevokeSourcePublic)
	router.GET("/v1/openbrain/cloud/public-profile", gbrainHandler.CloudGetPublicBrainProfile)
	router.PUT("/v1/openbrain/cloud/public-profile", gbrainHandler.CloudUpdatePublicBrainProfile)
	router.GET("/v1/openbrain/cloud/public-brains", gbrainHandler.CloudListPublicBrains)
	router.GET("/v1/openbrain/cloud/public-brains/:ownerUID/sources", gbrainHandler.CloudResolvePublicBrainSources)
	router.PUT("/v1/openbrain/cloud/public-brains/:ownerUID/subscription", gbrainHandler.CloudSubscribePublicBrain)
	router.DELETE("/v1/openbrain/cloud/public-brains/:ownerUID/subscription", gbrainHandler.CloudUnsubscribePublicBrain)

	httpServer := &http.Server{
		Addr:    addr,
		Handler: router,
	}

	go func() {
		slog.Info("openbrain-server listening", "addr", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("http server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down http server")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		slog.Error("http server shutdown error", "error", err)
	}
}

// recoveryWithSlog recovers panics and logs them via slog (into openbrain-server.log) with stack trace, then returns 500.
func recoveryWithSlog() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				slog.Error("panic recovered", "panic", err, "stack", string(debug.Stack()))
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}

// requestLogMiddleware logs 5xx responses always; logs all requests when verbose is true.
func requestLogMiddleware(verbose bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		method := c.Request.Method
		clientIP := c.ClientIP()
		c.Next()
		status := c.Writer.Status()
		latency := time.Since(start)
		if status >= 500 {
			slog.Error("request 5xx", "method", method, "path", path, "status", status, "client", clientIP, "latency", latency)
		} else if verbose {
			slog.Info("request", "method", method, "path", path, "status", status, "client", clientIP, "latency", latency)
		}
	}
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Max-Age", "600")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// setupFromHostConfig fetches HostConfig via host/config/get opcode,
// configures lumberjack-rotated logging and returns the OpAgent base dir.
func setupFromHostConfig(notifySvc *notify.Service) (*op.Config, error) {
	h := hostcfg.Get()
	if h == nil {
		return nil, fmt.Errorf("host not initialised")
	}

	cfg, err := h.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("get host config: %w", err)
	}

	openbrainBase := strings.TrimSpace(cfg.System.BaseDir)
	if openbrainBase == "" {
		// Fallback to ~/.openbrain if host config has no baseDir.
		home, _ := os.UserHomeDir()
		openbrainBase = filepath.Join(home, ".openbrain")
	}
	chatindex.SetBaseDir(openbrainBase)
	_ = os.Setenv("OPENBRAIN_BASE_DIR", openbrainBase)

	// ---- log rotation with lumberjack ----
	logDir := filepath.Join(openbrainBase, "logs", "openbrain-server")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("create log dir: %w", err)
	}
	logFile := filepath.Join(logDir, "openbrain-server.log")

	lj := &lumberjack.Logger{
		Filename:   logFile,
		MaxSize:    10, // MB
		MaxAge:     7,  // days
		MaxBackups: 10,
		LocalTime:  true,
		Compress:   true,
	}
	logger := slog.New(slog.NewTextHandler(lj, &slog.HandlerOptions{
		Level:     slog.LevelDebug,
		AddSource: true,
	}))
	slog.SetDefault(logger)
	slog.Info("logging initialised", "file", logFile)

	// cache config
	cache.Set("config", *cfg, cache.NoExpiration)

	return cfg, nil
}
