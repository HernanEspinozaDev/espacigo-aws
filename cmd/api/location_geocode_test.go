package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/geoplaces"
	"github.com/aws/aws-sdk-go-v2/service/geoplaces/types"
)

type fakePlacesClient struct{ input *geoplaces.GeocodeInput }

func (f *fakePlacesClient) Geocode(_ context.Context, input *geoplaces.GeocodeInput, _ ...func(*geoplaces.Options)) (*geoplaces.GeocodeOutput, error) {
	f.input = input
	label := "Alameda 123, Santiago"
	return &geoplaces.GeocodeOutput{ResultItems: []types.GeocodeResultItem{
		{Title: &label, Position: []float64{-70.66, -33.45}, PlaceType: types.PlaceTypePointAddress},
		{Title: &label, Position: []float64{-70.67, -33.46}, PlaceType: types.PlaceTypeInterpolatedAddress},
		{Title: &label, Position: []float64{-70.68, -33.47}, PlaceType: types.PlaceTypePointOfInterest},
	}}, nil
}

type fakeReversePlacesClient struct {
	input  *geoplaces.ReverseGeocodeInput
	output *geoplaces.ReverseGeocodeOutput
}

func (f *fakeReversePlacesClient) ReverseGeocode(_ context.Context, input *geoplaces.ReverseGeocodeInput, _ ...func(*geoplaces.Options)) (*geoplaces.ReverseGeocodeOutput, error) {
	f.input = input
	return f.output, nil
}

func TestPlacesGeocoderUsesChileStorageAndAddressPrecision(t *testing.T) {
	client := &fakePlacesClient{}
	results, err := (placesGeocoder{client: client}).Search(context.Background(), "Alameda 123, Santiago, Chile")
	if err != nil {
		t.Fatal(err)
	}
	if client.input.IntendedUse != types.GeocodeIntendedUseStorage || len(client.input.Filter.IncludeCountries) != 1 || client.input.Filter.IncludeCountries[0] != "CHL" {
		t.Fatalf("request config: %#v", client.input)
	}
	if len(results) != 2 || results[0].Precision != "address" || results[1].Precision != "approximate" || results[0].Latitude != -33.45 || results[0].Longitude != -70.66 {
		t.Fatalf("results: %#v", results)
	}
}

type placesTestHTTPClient func(*http.Request) (*http.Response, error)

func (f placesTestHTTPClient) Do(req *http.Request) (*http.Response, error) { return f(req) }

func TestPlacesPilotUsesRegionalEndpointWithoutEmulatorCredentials(t *testing.T) {
	const syntheticKey = "v1.public.fixture"
	httpClient := placesTestHTTPClient(func(req *http.Request) (*http.Response, error) {
		if req.Method != "POST" || req.URL.Scheme != "https" || req.URL.Host != "places.geo.us-east-2.amazonaws.com" ||
			req.URL.Path != "/v2/geocode" || req.URL.Query().Get("key") != syntheticKey || req.Header.Get("Authorization") != "" {
			t.Fatal("unexpected endpoint or signing mode")
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ResultItems":[]}`)),
		}, nil
	})
	client := geoplaces.New(geoplaces.Options{
		Region: "us-east-2", Credentials: aws.AnonymousCredentials{}, HTTPClient: httpClient,
	})
	g := placesGeocoder{client: client, apiKey: aws.String(syntheticKey)}
	if _, err := g.Search(context.Background(), "Alameda 123, Santiago, Chile"); err != nil {
		t.Fatal(err)
	}
}

func TestPlacesReverseGeocoderConstrainsResultToChileAddresses(t *testing.T) {
	title := "Alameda 123, Santiago, Chile"
	country := "CHL"
	client := &fakeReversePlacesClient{output: &geoplaces.ReverseGeocodeOutput{ResultItems: []types.ReverseGeocodeResultItem{{Title: &title, PlaceType: types.PlaceTypePointAddress, Address: &types.Address{Country: &types.Country{Code3: &country}}}}}}
	got, err := (placesReverseGeocoder{client: client}).Reverse(context.Background(), -33.45, -70.66)
	if err != nil || got != title {
		t.Fatalf("got %q, %v", got, err)
	}
	if len(client.input.QueryPosition) != 2 || client.input.QueryPosition[0] != -70.66 || client.input.QueryPosition[1] != -33.45 || aws.ToInt64(client.input.QueryRadius) != 500 || len(client.input.Filter.IncludePlaceTypes) == 0 {
		t.Fatalf("request=%#v", client.input)
	}
	client.output.ResultItems[0].Address.Country.Code3 = aws.String("ARG")
	got, err = (placesReverseGeocoder{client: client}).Reverse(context.Background(), -33.45, -70.66)
	if err != nil || got != "" {
		t.Fatalf("foreign result accepted: %q %v", got, err)
	}
}

type fakePostalPlacesClient struct{}

func (f fakePostalPlacesClient) Geocode(_ context.Context, _ *geoplaces.GeocodeInput, _ ...func(*geoplaces.Options)) (*geoplaces.GeocodeOutput, error) {
	title := "Alameda 123"
	addressLabel := "Alameda 123, 8320000 Santiago, Región Metropolitana, Chile"
	return &geoplaces.GeocodeOutput{ResultItems: []types.GeocodeResultItem{
		{
			Title:     &title,
			Address:   &types.Address{Label: &addressLabel},
			Position:  []float64{-70.66, -33.45},
			PlaceType: types.PlaceTypePointAddress,
		},
	}}, nil
}

func TestPlacesGeocoderAndReverseStripPostalCode(t *testing.T) {
	results, err := (placesGeocoder{client: fakePostalPlacesClient{}}).Search(context.Background(), "Alameda 123, Santiago, Chile")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Label != "Alameda 123, Santiago, Región Metropolitana, Chile" {
		t.Fatalf("unexpected geocode label: %#v", results)
	}
	title := "Alameda 123, 8320000 Santiago, Chile"
	country := "CHL"
	revClient := &fakeReversePlacesClient{output: &geoplaces.ReverseGeocodeOutput{ResultItems: []types.ReverseGeocodeResultItem{{
		Title:     &title,
		PlaceType: types.PlaceTypePointAddress,
		Address:   &types.Address{Country: &types.Country{Code3: &country}},
	}}}}
	got, err := (placesReverseGeocoder{client: revClient}).Reverse(context.Background(), -33.45, -70.66)
	if err != nil || got != "Alameda 123, Santiago, Chile" {
		t.Fatalf("unexpected reverse label: %q, err=%v", got, err)
	}
}
