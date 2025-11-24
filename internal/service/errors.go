package service

import "errors"

var (
	ErrPermissionDenied = errors.New("permission denied")
	ErrNotFound         = errors.New("resource not found")
	ErrInvalidInput     = errors.New("invalid input")
	ErrConflict         = errors.New("conflict")
)

var (
	ErrAreaHasTickets   = errors.New("cannot delete cleaning area: it has related tickets")
	ErrPolygonHasTrips   = errors.New("cannot delete polygon: it has related trips")
)