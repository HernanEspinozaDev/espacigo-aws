package main

import (
	"context"
	"errors"
	"math"
	"strings"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	spaceshttp "github.com/HernanEspinozaDev/espaciGo/internal/spaces/transport/http"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/geoplaces"
	"github.com/aws/aws-sdk-go-v2/service/geoplaces/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type locationCatalogDB struct{ pool *pgxpool.Pool }

func (c locationCatalogDB) ResolveLocation(ctx context.Context, regionCode, communeCode string) (string, string, bool, error) {
	var region, commune string
	err := c.pool.QueryRow(ctx, `SELECT r.nombre, c.nombre FROM public.comuna_publica c
JOIN public.region_publica r ON r.catalogo_version=c.catalogo_version AND r.codigo=c.region_codigo
JOIN public.catalogo_ubicacion_publica v ON v.version=c.catalogo_version AND v.activo
WHERE c.codigo=$1 AND c.region_codigo=$2`, communeCode, regionCode).Scan(&region, &commune)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	return region, commune, true, nil
}

type placesSearcher interface {
	Geocode(context.Context, *geoplaces.GeocodeInput, ...func(*geoplaces.Options)) (*geoplaces.GeocodeOutput, error)
}
type placesReverser interface {
	ReverseGeocode(context.Context, *geoplaces.ReverseGeocodeInput, ...func(*geoplaces.Options)) (*geoplaces.ReverseGeocodeOutput, error)
}

type placesGeocoder struct {
	client placesSearcher
	apiKey *string
}

func (g placesGeocoder) Search(ctx context.Context, query string) ([]spaceshttp.GeocodeCandidate, error) {
	output, err := g.client.Geocode(ctx, &geoplaces.GeocodeInput{
		Key: g.apiKey, QueryText: aws.String(query), Filter: &types.GeocodeFilter{IncludeCountries: []string{"CHL"}},
		IntendedUse: types.GeocodeIntendedUseStorage, Language: aws.String("es"), MaxResults: aws.Int32(5),
	})
	if err != nil || output == nil {
		return nil, errors.New("Places search unavailable")
	}
	results := make([]spaceshttp.GeocodeCandidate, 0, len(output.ResultItems))
	for _, item := range output.ResultItems {
		if item.PlaceType == types.PlaceTypePointOfInterest {
			continue
		}
		if len(item.Position) != 2 || item.Title == nil || strings.TrimSpace(*item.Title) == "" ||
			math.IsNaN(item.Position[0]) || math.IsNaN(item.Position[1]) ||
			math.Abs(item.Position[0]) > 180 || math.Abs(item.Position[1]) > 90 {
			continue
		}
		precision := "approximate"
		switch item.PlaceType {
		case types.PlaceTypePointAddress, types.PlaceTypeSecondaryAddress:
			precision = "address"
		}
		if item.EstimatedPointAddress != nil && *item.EstimatedPointAddress {
			precision = "approximate"
		}
		label := *item.Title
		if item.Address != nil && item.Address.Label != nil && strings.TrimSpace(*item.Address.Label) != "" {
			label = *item.Address.Label
		}
		results = append(results, spaceshttp.GeocodeCandidate{Label: spaces.StripPostalCodeFromAddress(label), Longitude: item.Position[0], Latitude: item.Position[1], Precision: precision})
	}
	return results, nil
}

type placesReverseGeocoder struct {
	client placesReverser
	apiKey *string
}

func (g placesReverseGeocoder) Reverse(ctx context.Context, latitude, longitude float64) (string, error) {
	out, err := g.client.ReverseGeocode(ctx, &geoplaces.ReverseGeocodeInput{Key: g.apiKey, QueryPosition: []float64{longitude, latitude}, IntendedUse: types.ReverseGeocodeIntendedUseStorage, Language: aws.String("es"), MaxResults: aws.Int32(1), QueryRadius: aws.Int64(500), Filter: &types.ReverseGeocodeFilter{IncludePlaceTypes: []types.ReverseGeocodeFilterPlaceType{types.ReverseGeocodeFilterPlaceTypePointAddress, types.ReverseGeocodeFilterPlaceTypeStreet, types.ReverseGeocodeFilterPlaceTypeInterpolatedAddress, types.ReverseGeocodeFilterPlaceTypeSecondaryAddress}}})
	if err != nil || out == nil {
		return "", errors.New("Places reverse geocoding unavailable")
	}
	if len(out.ResultItems) == 0 || out.ResultItems[0].Title == nil {
		return "", nil
	}
	switch out.ResultItems[0].PlaceType {
	case types.PlaceTypePointAddress, types.PlaceTypeStreet, types.PlaceTypeInterpolatedAddress, types.PlaceTypeSecondaryAddress:
	default:
		return "", nil
	}
	if out.ResultItems[0].Address == nil || out.ResultItems[0].Address.Country == nil || out.ResultItems[0].Address.Country.Code3 == nil || *out.ResultItems[0].Address.Country.Code3 != "CHL" {
		return "", nil
	}
	return spaces.StripPostalCodeFromAddress(*out.ResultItems[0].Title), nil
}

func configureLocationGeocoder(ctx context.Context, handler *spaceshttp.Handler, pool *pgxpool.Pool, getenv func(string) string, local bool) error {
	catalog := locationCatalogDB{pool}
	if getenv("AWS_PLACES_GEOCODING_ENABLED") != "true" {
		handler.SetReverseGeocoder(nil, catalog)
		return nil
	}
	if local {
		region := strings.TrimSpace(getenv("AWS_PLACES_PILOT_REGION"))
		key := strings.TrimSpace(getenv("AWS_PLACES_PILOT_API_KEY"))
		if region == "" || !strings.HasPrefix(key, "v1.public.") || key == "v1.public." {
			return errors.New("Places local pilot requires AWS_PLACES_PILOT_REGION and AWS_PLACES_PILOT_API_KEY")
		}
		// Deliberately construct an independent client. The local AWS endpoint,
		// CA and credentials belong exclusively to the emulator.
		client := geoplaces.New(geoplaces.Options{
			Region: region, Credentials: aws.AnonymousCredentials{}, RetryMaxAttempts: 1,
		})
		handler.SetGeocoder(placesGeocoder{client: client, apiKey: &key}, catalog)
		handler.SetReverseGeocoder(placesReverseGeocoder{client: client, apiKey: &key}, catalog)
		return nil
	}
	region := strings.TrimSpace(getenv("AWS_REGION"))
	if region == "" || getenv("AWS_ENDPOINT_URL") != "" || getenv("AWS_ENDPOINT_URL_GEO_PLACES") != "" || getenv("AWS_ENDPOINT_URL_GEOPLACES") != "" {
		return errors.New("Places geocoding requires an explicit AWS_REGION and standard AWS endpoint")
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(region), awsconfig.WithRetryMaxAttempts(1))
	if err != nil {
		return errors.New("Places geocoding configuration failed")
	}
	client := geoplaces.NewFromConfig(cfg)
	handler.SetGeocoder(placesGeocoder{client: client}, catalog)
	handler.SetReverseGeocoder(placesReverseGeocoder{client: client}, catalog)
	return nil
}
