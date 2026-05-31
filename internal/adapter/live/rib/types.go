package rib

import (
	liveadapter "github.com/81ueman/hoyan-lab/internal/adapter/live"
	"github.com/81ueman/hoyan-lab/internal/domain/observation"
)

type RIBRoute = observation.RIBRoute
type CompareOptions = observation.CompareOptions
type Runner = liveadapter.Runner

var DefaultCompareOptions = observation.DefaultCompareOptions
var SortRoutes = observation.SortRoutes
