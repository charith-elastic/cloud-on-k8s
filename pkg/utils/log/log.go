// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package log

import (
	"context"
	"flag"
	"os"
	"strconv"

	"github.com/elastic/cloud-on-k8s/pkg/about"
	"github.com/elastic/cloud-on-k8s/pkg/controller/common/tracing"
	"github.com/elastic/cloud-on-k8s/pkg/dev"
	"github.com/go-logr/logr"
	"github.com/spf13/pflag"
	"go.elastic.co/apm"
	"go.elastic.co/apm/module/apmzap"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	klog "k8s.io/klog/v2"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
	crzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	EcsVersion     = "1.4.0"
	EcsServiceType = "eck"
	FlagName       = "log-verbosity"
)

var verbosity = flag.Int(FlagName, 0, "Verbosity level of logs (-2=Error, -1=Warn, 0=Info, >0=Debug)")

// BindFlags attaches logging flags to the given flag set.
func BindFlags(flags *pflag.FlagSet) {
	flags.AddGoFlag(flag.Lookup("log-verbosity"))
}

type logBuilder struct {
	tracer    *apm.Tracer
	verbosity *int
}

// Option represents log configuration options.
type Option func(*logBuilder)

// WithVerbosity is the option to pass to InitLogger to set the log verbosity level.
// Verbosity levels from 2 are custom levels that increase the verbosity as the value increases.
// Standard levels are as follows:
// level | Zap level | name
// -------------------------
//  1    | -1        | Debug
//  0    |  0        | Info
// -1    |  1        | Warn
// -2    |  2        | Error
func WithVerbosity(verbosity int) Option {
	return func(lb *logBuilder) {
		lb.verbosity = &verbosity
	}
}

// WithTracer is the option to pass to InitLogger to set the tracer for the log backend.
func WithTracer(tracer *apm.Tracer) Option {
	return func(lb *logBuilder) {
		lb.tracer = tracer
	}
}

// InitLogger initializes the global logger.
func InitLogger(opts ...Option) {
	lb := &logBuilder{
		verbosity: verbosity,
	}

	for _, opt := range opts {
		opt(lb)
	}

	setLogger(lb.verbosity, lb.tracer)
}

func setLogger(v *int, tracer *apm.Tracer) {
	zapLevel := determineLogLevel(v)

	// if the Zap custom level is less than debug (verbosity level 2 and above) set the klog level to the same level
	if zapLevel.Level() < zap.DebugLevel {
		flagset := flag.NewFlagSet("", flag.ContinueOnError)
		klog.InitFlags(flagset)
		_ = flagset.Set("v", strconv.Itoa(int(zapLevel.Level())*-1))
	}

	opts := []zap.Option{
		zap.Fields(
			zap.String("service.version", getVersionString()),
		),
	}

	// use instrumented core if tracing is enabled
	if tracer != nil {
		opts = append(opts, zap.WrapCore((&apmzap.Core{Tracer: tracer}).WrapCore))
	}

	var encoder zapcore.Encoder
	if dev.Enabled {
		encoderConf := zap.NewDevelopmentEncoderConfig()
		encoderConf.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderConf)
	} else {
		encoderConf := zap.NewProductionEncoderConfig()
		encoderConf.MessageKey = "message"
		encoderConf.TimeKey = "@timestamp"
		encoderConf.LevelKey = "log.level"
		encoderConf.NameKey = "log.logger"
		encoderConf.StacktraceKey = "error.stack_trace"
		encoderConf.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewJSONEncoder(encoderConf)
		opts = append(opts,
			zap.Fields(
				zap.String("service.type", EcsServiceType),
				zap.String("ecs.version", EcsVersion),
			))
	}

	stackTraceLevel := zap.NewAtomicLevelAt(zapcore.ErrorLevel)
	crlog.SetLogger(crzap.New(func(o *crzap.Options) {
		o.DestWritter = os.Stderr
		o.Development = dev.Enabled
		o.Level = &zapLevel
		o.StacktraceLevel = &stackTraceLevel
		o.Encoder = encoder
		o.ZapOpts = opts
	}))
}

func determineLogLevel(v *int) zap.AtomicLevel {
	switch {
	case v != nil && *v > -3:
		return zap.NewAtomicLevelAt(zapcore.Level(*v * -1))
	case dev.Enabled:
		return zap.NewAtomicLevelAt(zapcore.DebugLevel)
	default:
		return zap.NewAtomicLevelAt(zapcore.InfoLevel)
	}
}

func getVersionString() string {
	buildInfo := about.GetBuildInfo()
	return buildInfo.VersionString()
}

// WithName returns a logger with the given name.
func WithName(name string) logr.Logger {
	return crlog.Log.WithName(name)
}

// WithContext returns a logger with tags set to information from the context.
func WithContext(ctx context.Context, parent logr.Logger) logr.Logger {
	keyValues := tracing.TraceContextKV(ctx)

	return parent.WithValues(keyValues...)
}

type ctxKey struct{}

var loggerCtxKey = ctxKey{}

// FromContext returns the logger in the context if it exists or the default logger.
// The returned logger will have context information added to its tags.
func FromContext(ctx context.Context) logr.Logger {
	var logger logr.Logger = crlog.Log

	if ctx != nil {
		if lv := ctx.Value(loggerCtxKey); lv != nil {
			logger = lv.(logr.Logger)
		}
	}

	return WithContext(ctx, logger)
}

// IntoContext inserts the given logger into the context.
func IntoContext(ctx context.Context, logger logr.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, logger)
}
