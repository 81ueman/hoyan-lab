package rib

import (
	"context"

	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type RIBRoute = observation.RIBRoute
type CompareOptions = observation.CompareOptions

func DefaultCompareOptions() observation.CompareOptions {
	return observation.DefaultCompareOptions()
}

var SortRoutes = observation.SortRoutes
