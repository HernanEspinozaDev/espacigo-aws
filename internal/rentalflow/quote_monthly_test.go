package rentalflow

import (
	"context"
	"errors"
	"testing"
	"time"
)

type monthlyCatalogStub struct {
	listing Listing
	rule    MonthlyRule
	ruleErr error
}

func (s *monthlyCatalogStub) PublicListing(ctx context.Context, spaceID string) (Listing, error) {
	if ctx.Err() != nil {
		return Listing{}, ErrUnavailable
	}
	return s.listing, nil
}

func (s *monthlyCatalogStub) PublicMonthlyRule(ctx context.Context, spaceID string) (MonthlyRule, error) {
	if ctx.Err() != nil {
		return MonthlyRule{}, ErrUnavailable
	}
	return s.rule, s.ruleErr
}

type memoryQuoteRepo struct {
	billingModes map[string]string
	created      Quote
	multiCreated MultiQuote
}

func (r *memoryQuoteRepo) CreateValidatedQuote(ctx context.Context, renterID string, listing Listing, interval Interval, units, subtotal int64, now time.Time) (Quote, error) {
	q := Quote{
		ID:           "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa",
		SpaceID:      listing.ID,
		OwnerID:      listing.OwnerID,
		RenterID:     renterID,
		SpaceTitle:   listing.Title,
		RateVersion:  listing.RateVersion,
		RateUnit:     listing.RateUnit,
		UnitPriceCLP: listing.UnitPriceCLP,
		Currency:     listing.Currency,
		Units:        units,
		SubtotalCLP:  subtotal,
		StartAt:      interval.StartAt,
		EndAt:        interval.EndAt,
		TimeZone:     listing.TimeZone,
		CreatedAt:    now,
		ExpiresAt:    now.Add(QuoteLifetime),
	}
	r.created = q
	return q, nil
}

func (r *memoryQuoteRepo) GetForParticipant(ctx context.Context, quoteID, participantID string) (Quote, error) {
	return r.created, nil
}

func (r *memoryQuoteRepo) CreateMultiValidatedQuote(ctx context.Context, renterID string, listing Listing, intervals []QuoteInterval, units, subtotal int64, now time.Time) (MultiQuote, error) {
	mq := MultiQuote{
		Quote: Quote{
			ID:           "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb",
			SpaceID:      listing.ID,
			OwnerID:      listing.OwnerID,
			RenterID:     renterID,
			SpaceTitle:   listing.Title,
			RateVersion:  listing.RateVersion,
			RateUnit:     listing.RateUnit,
			UnitPriceCLP: listing.UnitPriceCLP,
			Currency:     listing.Currency,
			Units:        units,
			SubtotalCLP:  subtotal,
			StartAt:      intervals[0].StartAt,
			EndAt:        intervals[len(intervals)-1].EndAt,
			TimeZone:     listing.TimeZone,
			CreatedAt:    now,
			ExpiresAt:    now.Add(QuoteLifetime),
		},
		Intervals: intervals,
	}
	r.multiCreated = mq
	return mq, nil
}

func (r *memoryQuoteRepo) SaveQuoteBillingMode(ctx context.Context, quoteID, billingMode string) error {
	if r.billingModes == nil {
		r.billingModes = make(map[string]string)
	}
	r.billingModes[quoteID] = billingMode
	return nil
}

func TestNormalizeBillingModeRejectsInstallmentsOnHourlyAndDaily(t *testing.T) {
	for _, unit := range []string{"hora", "dia"} {
		_, err := normalizeBillingMode(unit, BillingModeMonthlyInstallments)
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("normalizeBillingMode(%q, monthly_installments) = %v, want ErrInvalid", unit, err)
		}
	}

	mode, err := normalizeBillingMode("mes", BillingModeMonthlyInstallments)
	if err != nil || mode != BillingModeMonthlyInstallments {
		t.Fatalf("normalizeBillingMode(mes, monthly_installments) = (%q, %v), want (%q, nil)", mode, err, BillingModeMonthlyInstallments)
	}

	for _, unit := range []string{"hora", "dia", "mes"} {
		mode, err := normalizeBillingMode(unit, "")
		if err != nil || mode != BillingModeFull {
			t.Fatalf("normalizeBillingMode(%q, '') = (%q, %v), want (%q, nil)", unit, mode, err, BillingModeFull)
		}
		mode, err = normalizeBillingMode(unit, BillingModeFull)
		if err != nil || mode != BillingModeFull {
			t.Fatalf("normalizeBillingMode(%q, 'full') = (%q, %v), want (%q, nil)", unit, mode, err, BillingModeFull)
		}
	}
}

func TestQuoteServiceRejectsMonthlyInstallmentsOnHourlyAndDaily(t *testing.T) {
	now := time.Date(2030, time.May, 1, 12, 0, 0, 0, time.UTC)
	start := time.Date(2030, time.May, 2, 9, 0, 0, 0, time.UTC)
	end := time.Date(2030, time.May, 2, 11, 0, 0, 0, time.UTC)

	for _, unit := range []string{"hora", "dia"} {
		listing := Listing{
			ID:           testSpaceID,
			OwnerID:      testOwnerID,
			State:        "activa",
			Title:        "Espacio de Prueba",
			RateUnit:     unit,
			UnitPriceCLP: 10000,
			Currency:     "CLP",
			RateVersion:  1,
			TimeZone:     "America/Santiago",
		}
		catalog := &monthlyCatalogStub{listing: listing}
		calendar := &calendarStub{available: true}
		quotes := &memoryQuoteRepo{}
		service, err := NewQuoteService(catalog, calendar, quotes, func() time.Time { return now })
		if err != nil {
			t.Fatal(err)
		}

		// Single quote
		_, err = service.Create(context.Background(), testRenterID, QuoteRequest{
			SpaceID:     testSpaceID,
			StartAt:     start.Format(time.RFC3339),
			EndAt:       end.Format(time.RFC3339),
			BillingMode: BillingModeMonthlyInstallments,
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("Create(%s, monthly_installments) err = %v, want ErrInvalid", unit, err)
		}

		// Multi quote
		_, err = service.CreateMulti(context.Background(), testRenterID, MultiQuoteRequest{
			SpaceID: testSpaceID,
			Intervals: []MultiQuoteIntervalInput{
				{StartAt: start.Format(time.RFC3339), EndAt: end.Format(time.RFC3339)},
			},
			BillingMode: BillingModeMonthlyInstallments,
		})
		if !errors.Is(err, ErrInvalid) {
			t.Fatalf("CreateMulti(%s, monthly_installments) err = %v, want ErrInvalid", unit, err)
		}
	}
}

func TestValidateMonthlyRuleEstadiaMinimaMeses(t *testing.T) {
	now := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation("America/Santiago")
	start := time.Date(2030, time.February, 1, 0, 0, 0, 0, loc)
	end2Months := time.Date(2030, time.April, 1, 0, 0, 0, 0, loc)
	end3Months := time.Date(2030, time.May, 1, 0, 0, 0, 0, loc)

	listing := Listing{
		ID:           testSpaceID,
		OwnerID:      testOwnerID,
		State:        "activa",
		Title:        "Espacio Mensual",
		RateUnit:     "mes",
		UnitPriceCLP: 150000,
		Currency:     "CLP",
		RateVersion:  1,
		TimeZone:     "America/Santiago",
	}

	catalog := &monthlyCatalogStub{
		listing: listing,
		rule: MonthlyRule{
			EstadiaMinimaMeses: 3,
		},
	}
	calendar := &calendarStub{available: true}
	quotes := &memoryQuoteRepo{}
	service, err := NewQuoteService(catalog, calendar, quotes, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	// 2 months fails min 3
	_, err = service.Create(context.Background(), testRenterID, QuoteRequest{
		SpaceID:     testSpaceID,
		StartAt:     start.UTC().Format(time.RFC3339),
		EndAt:       end2Months.UTC().Format(time.RFC3339),
		BillingMode: BillingModeMonthlyInstallments,
	})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("Create(2 months < min 3) err = %v, want ErrInvalid", err)
	}

	// 3 months succeeds and saves billing mode
	quote, err := service.Create(context.Background(), testRenterID, QuoteRequest{
		SpaceID:     testSpaceID,
		StartAt:     start.UTC().Format(time.RFC3339),
		EndAt:       end3Months.UTC().Format(time.RFC3339),
		BillingMode: BillingModeMonthlyInstallments,
	})
	if err != nil {
		t.Fatalf("Create(3 months >= min 3) err = %v, want nil", err)
	}
	if quote.BillingMode != BillingModeMonthlyInstallments {
		t.Fatalf("quote.BillingMode = %q, want %q", quote.BillingMode, BillingModeMonthlyInstallments)
	}
	if quotes.billingModes[quote.ID] != BillingModeMonthlyInstallments {
		t.Fatalf("saved billing mode = %q, want %q", quotes.billingModes[quote.ID], BillingModeMonthlyInstallments)
	}
}

func TestValidateMonthlyRuleDisponibleDesde(t *testing.T) {
	now := time.Date(2030, time.May, 1, 0, 0, 0, 0, time.UTC)
	loc, _ := time.LoadLocation("America/Santiago")
	startMay := time.Date(2030, time.May, 15, 0, 0, 0, 0, loc)
	endJune := time.Date(2030, time.June, 15, 0, 0, 0, 0, loc)
	startJuly := time.Date(2030, time.July, 1, 0, 0, 0, 0, loc)
	endAugust := time.Date(2030, time.August, 1, 0, 0, 0, 0, loc)

	dispDesde := "2030-07-01"
	listing := Listing{
		ID:           testSpaceID,
		OwnerID:      testOwnerID,
		State:        "activa",
		Title:        "Espacio Mensual Disponible Desde",
		RateUnit:     "mes",
		UnitPriceCLP: 200000,
		Currency:     "CLP",
		RateVersion:  1,
		TimeZone:     "America/Santiago",
	}

	catalog := &monthlyCatalogStub{
		listing: listing,
		rule: MonthlyRule{
			DisponibleDesde:    &dispDesde,
			EstadiaMinimaMeses: 1,
		},
	}
	calendar := &calendarStub{available: true}
	quotes := &memoryQuoteRepo{}
	service, err := NewQuoteService(catalog, calendar, quotes, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	// Booking in May (before July 1) must be rejected with ErrConflict
	_, err = service.Create(context.Background(), testRenterID, QuoteRequest{
		SpaceID: testSpaceID,
		StartAt: startMay.UTC().Format(time.RFC3339),
		EndAt:   endJune.UTC().Format(time.RFC3339),
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("Create(start before disponible_desde) err = %v, want ErrConflict", err)
	}

	// Booking on or after July 1 succeeds
	_, err = service.Create(context.Background(), testRenterID, QuoteRequest{
		SpaceID: testSpaceID,
		StartAt: startJuly.UTC().Format(time.RFC3339),
		EndAt:   endAugust.UTC().Format(time.RFC3339),
	})
	if err != nil {
		t.Fatalf("Create(start on disponible_desde) err = %v, want nil", err)
	}
}

func TestAvailabilityServiceExposesMonthlyRules(t *testing.T) {
	now := time.Date(2030, time.January, 1, 0, 0, 0, 0, time.UTC)
	dispDesde := "2030-03-01"
	minMonths := 2

	listing := Listing{
		ID:           testSpaceID,
		OwnerID:      testOwnerID,
		State:        "activa",
		Title:        "Espacio Mensual Rules",
		RateUnit:     "mes",
		UnitPriceCLP: 180000,
		Currency:     "CLP",
		RateVersion:  1,
		TimeZone:     "America/Santiago",
	}

	catalog := &monthlyCatalogStub{
		listing: listing,
		rule: MonthlyRule{
			DisponibleDesde:    &dispDesde,
			EstadiaMinimaMeses: minMonths,
		},
	}
	loc, _ := time.LoadLocation("America/Santiago")
	dateStart := time.Date(2030, time.March, 1, 0, 0, 0, 0, loc)
	dateEnd := time.Date(2030, time.April, 1, 0, 0, 0, 0, loc)
	calendar := &calendarStub{
		intervals: []Interval{{StartAt: dateStart.UTC(), EndAt: dateEnd.UTC()}},
		available: true,
	}

	service, err := NewAvailabilityService(catalog, calendar, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}

	// ListPublic
	avail, err := service.ListPublic(context.Background(), testSpaceID, "2030-03-01", 1)
	if err != nil {
		t.Fatalf("ListPublic() err = %v, want nil", err)
	}
	if avail.DisponibleDesde == nil || *avail.DisponibleDesde != dispDesde {
		t.Fatalf("avail.DisponibleDesde = %v, want %q", avail.DisponibleDesde, dispDesde)
	}
	if avail.EstadiaMinimaMeses == nil || *avail.EstadiaMinimaMeses != minMonths {
		t.Fatalf("avail.EstadiaMinimaMeses = %v, want %d", avail.EstadiaMinimaMeses, minMonths)
	}

	// ListPublicSchedule
	sched, err := service.ListPublicSchedule(context.Background(), testSpaceID, "2030-03-01", 1)
	if err != nil {
		t.Fatalf("ListPublicSchedule() err = %v, want nil", err)
	}
	if sched.DisponibleDesde == nil || *sched.DisponibleDesde != dispDesde {
		t.Fatalf("sched.DisponibleDesde = %v, want %q", sched.DisponibleDesde, dispDesde)
	}
	if sched.EstadiaMinimaMeses == nil || *sched.EstadiaMinimaMeses != minMonths {
		t.Fatalf("sched.EstadiaMinimaMeses = %v, want %d", sched.EstadiaMinimaMeses, minMonths)
	}
}
