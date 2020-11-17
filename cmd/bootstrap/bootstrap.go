// Copyright Elasticsearch B.V. and/or licensed to Elasticsearch B.V. under one
// or more contributor license agreements. Licensed under the Elastic License;
// you may not use this file except in compliance with the Elastic License.

package bootstrap

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/elastic/cloud-on-k8s/pkg/bootstrap"
	logconf "github.com/elastic/cloud-on-k8s/pkg/utils/log"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
)

var (
	errInterrupted  = errors.New("interrupted by signal")
	errJobSucceeded = errors.New("job succeeded")
	jobDefFile      string
)

func Command() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bootstrap",
		Short: "Run bootstrap job to initialize a resource",
		RunE:  doRun,
	}

	cmd.Flags().StringVar(&jobDefFile, "jobdef", "-", "Path to the job definition")
	cmd.MarkFlagFilename("jobdef")

	logconf.BindFlags(cmd.Flags())

	return cmd
}

func doRun(_ *cobra.Command, _ []string) error {
	logconf.InitLogger()
	logger := logf.Log.WithName("bootstrap").WithValues("path", jobDefFile)

	logger.Info("Opening job definition")

	r, cleanup, err := getJobDefReader(jobDefFile)
	if err != nil {
		logger.Error(err, "Failed to open job definition")
		return err
	}

	defer cleanup()

	logger.Info("Parsing job definition")

	jd, err := bootstrap.Load(r)
	if err != nil {
		logger.Error(err, "Failed to parse job definition")
		return err
	}

	logger.V(1).Info("Parsed job definition", "definition", jd)

	g, ctx := errgroup.WithContext(logf.IntoContext(context.Background(), logger))

	g.Go(func() error {
		signalChan := signals.SetupSignalHandler()
		select {
		case <-ctx.Done():
			return nil
		case <-signalChan:
			logger.Info("Interrupted by signal")
			return errInterrupted
		}
	})

	g.Go(func() error {
		logger.Info("Executing job")
		if err := bootstrap.Run(ctx, jd); err != nil {
			logger.Error(err, "Job failed")
			return err
		}

		logger.Info("Job succeeded")
		return errJobSucceeded
	})

	err = g.Wait()
	if errors.Is(err, errJobSucceeded) {
		return nil
	}

	return err
}

func getJobDefReader(name string) (io.Reader, func() error, error) {
	if name == "-" {
		return os.Stdin, func() error { return nil }, nil
	}

	f, err := os.Open(name)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open %s: %w", name, err)
	}

	return bufio.NewReader(f), f.Close, nil
}
