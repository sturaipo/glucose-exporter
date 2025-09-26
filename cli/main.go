package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/alecthomas/kong"
	"github.com/sturaipo/glucose-exporter/api/librelink"
	"github.com/sturaipo/glucose-exporter/collector"
	"go.uber.org/zap"
)

type CliCredentials struct {
	UserId string    `kong:"help='LibrelLink UserId',env='LIBRELINK_USERID'"`
	Token  string    `kong:"help='LibrelLink Token',env='LIBRELINK_TOKEN'"`
	Expiry time.Time `kong:"help='LibrelLink Token Expiry',env='LIBRELINK_TOKEN_EXPIRY'"`
}

func (c CliCredentials) Validate(kctx *kong.Context) error {
	fmt.Println("Validating credentials...")
	if (c.UserId == "" && c.Token != "") || (c.UserId != "" && c.Token == "") {
		return fmt.Errorf("both UserId and Token must be provided together")
	}

	if !c.Expiry.IsZero() && c.Expiry.Before(time.Now()) {
		return fmt.Errorf("token expiry must be in the future")
	}

	return nil
}

func (c CliCredentials) IsSet() bool {
	return c.UserId != "" && c.Token != ""
}

func (c CliCredentials) UserHash() string {
	sha256Sum := sha256.Sum256([]byte(c.UserId))
	return fmt.Sprintf("%x", sha256Sum)
}

type Config struct {
	Bind     string `kong:"help='Bind address',env='BIND',default=':5656'"`
	Username string `kong:"help='LibrelLink username',env='LIBRELINK_USERNAME',required"`
	Password string `kong:"help='LibrelLink password',env='LIBRELINK_PASSWORD',required"`

	Creds CliCredentials `kong:"embed,prefix='credentials.',help='LibrelLink credentials (optional)'"`

	Log struct {
		Level  string `kong:"help='Log level',env='LOG_LEVEL',default='info',enum='debug,info'"`
		Format string `kong:"help='Log format',env='LOG_FORMAT',default='console',enum='console,json'"`
	} `kong:"embed,prefix='log.',help='Logging options'"`
}

func configureLogger(cfg Config) (*zap.Logger, error) {
	zapConfig := zap.NewProductionConfig()

	if cfg.Log.Format == "console" {
		zapConfig.Encoding = "console"
	}

	switch cfg.Log.Level {
	case "debug":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.DebugLevel)
	case "info":
		zapConfig.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	default:
		return nil, fmt.Errorf("unknown log level: %s", cfg.Log.Level)
	}

	return zapConfig.Build()
}

func main() {

	config := Config{}
	kong.Parse(&config)

	logger, err := configureLogger(config)
	if err != nil {
		panic(err)
	}
	defer logger.Sync()

	options := []func(*librelink.LibreLinkClient){
		librelink.WithLogger(logger),
	}

	if config.Creds.IsSet() {
		logger.Info("Using provided credentials")
		options = append(options, librelink.WithExpiringCredentials(config.Creds.UserId, config.Creds.Token, config.Creds.Expiry))
	}

	// Initialize your LibreLink client here
	client := librelink.NewLibreLinkClient(
		config.Username,
		config.Password,
		options...,
	)

	prometheus.Unregister(collectors.NewGoCollector())
	// Enable Go metrics with pre-defined rules. Or your custom rules.
	prometheus.MustRegister(
		collectors.NewGoCollector(
			collectors.WithGoCollectorMemStatsMetricsDisabled(),
			collectors.WithGoCollectorRuntimeMetrics(
				collectors.MetricsScheduler,
				collectors.MetricsMemory,
			),
		),
	)

	collector := collector.NewGlucoseCollector(client)

	glucoseRegistry := prometheus.NewPedanticRegistry()
	glucoseRegistry.MustRegister(collector)

	handler := http.NewServeMux()
	handler.Handle(
		"/glucose",
		promhttp.InstrumentMetricHandler(
			prometheus.DefaultRegisterer,
			promhttp.HandlerFor(glucoseRegistry, promhttp.HandlerOpts{Registry: glucoseRegistry}),
		),
	)
	handler.Handle(
		"/metrics",
		promhttp.HandlerFor(prometheus.DefaultGatherer, promhttp.HandlerOpts{}),
	)

	server := &http.Server{
		Addr:    config.Bind,
		Handler: handler,
	}

	go func() {
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, os.Interrupt)

		sig := <-sigs
		logger.Info("Shutting down server", zap.String("signal", sig.String()))

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := server.Shutdown(ctx); err != nil {
			logger.Fatal("Server shutdown failed", zap.Error(err))
		}
		defer cancel()
	}()

	logger.Info("Starting server", zap.String("address", config.Bind))

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatal("Server failed", zap.Error(err))
	}
}
