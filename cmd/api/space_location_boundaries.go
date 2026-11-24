package main

import (
	"fmt"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/geoboundary"
	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
)

func configureSpaceLocationBoundaries(service *spaces.Service, getenv func(string) string) error {
	path := strings.TrimSpace(getenv("ESPACIGO_COMMUNE_BOUNDARIES_FILE"))
	if path == "" {
		return nil
	}
	boundaries, err := geoboundary.Load(path)
	if err != nil {
		return fmt.Errorf("configure commune boundaries: %w", err)
	}
	service.SetLocationBoundaryChecker(boundaries)
	return nil
}
