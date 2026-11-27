package privacy

import (
	"errors"
	"testing"
	"time"
)

func TestComputeInitialDeadline(t *testing.T) {
	loc := SantiagoLocation()

	t.Run("non-blocking right gets 30 calendar days end of Santiago day", func(t *testing.T) {
		submitted := time.Date(2026, 6, 10, 15, 30, 0, 0, loc)
		got := ComputeInitialDeadline(submitted, "acceso").In(loc)
		want := time.Date(2026, 7, 10, 23, 59, 59, 0, loc)
		if !got.Equal(want) {
			t.Fatalf("ComputeInitialDeadline(%v, acceso) = %v, want %v", submitted, got, want)
		}
	})

	t.Run("calendar deadline preserves end-of-day across Santiago DST transition", func(t *testing.T) {
		// Chile DST starts first Saturday of September (2026-09-05 24:00 -> 2026-09-06 01:00).
		submitted := time.Date(2026, 8, 20, 11, 15, 0, 0, loc)
		got := ComputeInitialDeadline(submitted, "supresion").In(loc)
		want := time.Date(2026, 9, 19, 23, 59, 59, 0, loc)
		if !got.Equal(want) {
			t.Fatalf("ComputeInitialDeadline across DST = %v, want %v", got, want)
		}
	})

	t.Run("blocking right skips weekend", func(t *testing.T) {
		// Friday 2026-06-12 -> Monday (1) -> Tuesday 2026-06-16 (2)
		submitted := time.Date(2026, 6, 12, 16, 45, 0, 0, loc)
		got := ComputeInitialDeadline(submitted, "bloqueo").In(loc)
		want := time.Date(2026, 6, 16, 23, 59, 59, 0, loc)
		if !got.Equal(want) {
			t.Fatalf("ComputeInitialDeadline(Fri, bloqueo) = %v, want %v", got, want)
		}
	})

	t.Run("blocking right skips Chilean Sep 18-19 holidays and weekend", func(t *testing.T) {
		// Thursday 2026-09-17 -> Fri Sep 18 (holiday), Sat Sep 19 (holiday+wknd), Sun Sep 20 (wknd)
		// -> Mon Sep 21 (1) -> Tue Sep 22 (2)
		submitted := time.Date(2026, 9, 17, 10, 0, 0, 0, loc)
		got := ComputeInitialDeadline(submitted, "bloqueo").In(loc)
		want := time.Date(2026, 9, 22, 23, 59, 59, 0, loc)
		if !got.Equal(want) {
			t.Fatalf("ComputeInitialDeadline(Sep 17, bloqueo) = %v, want %v", got, want)
		}
	})
}

func TestComputeExtensionDeadline(t *testing.T) {
	loc := SantiagoLocation()
	base := time.Date(2026, 7, 1, 23, 59, 59, 0, loc)

	got, err := ComputeExtensionDeadline(base, 30)
	if err != nil {
		t.Fatalf("ComputeExtensionDeadline(30) err = %v", err)
	}
	want := time.Date(2026, 7, 31, 23, 59, 59, 0, loc)
	if !got.Equal(want) {
		t.Fatalf("ComputeExtensionDeadline(30) = %v, want %v", got.In(loc), want)
	}

	for _, tc := range []struct {
		name string
		base time.Time
		days int
	}{
		{name: "zero base", base: time.Time{}, days: 10},
		{name: "zero days", base: base, days: 0},
		{name: "more than 30 days", base: base, days: 31},
	} {
		if _, err := ComputeExtensionDeadline(tc.base, tc.days); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s: err = %v, want ErrInvalid", tc.name, err)
		}
	}
}

func TestEffectiveDeadline(t *testing.T) {
	due := time.Date(2026, 7, 1, 12, 0, 0, 0, time.UTC)
	ext := due.AddDate(0, 0, 15)

	if got := EffectiveDeadline(nil, nil); got != nil {
		t.Fatalf("EffectiveDeadline(nil, nil) = %v, want nil", got)
	}
	if got := EffectiveDeadline(&due, nil); got == nil || !got.Equal(due) {
		t.Fatalf("EffectiveDeadline(due, nil) = %v, want %v", got, due)
	}
	if got := EffectiveDeadline(&due, &ext); got == nil || !got.Equal(ext) {
		t.Fatalf("EffectiveDeadline(due, ext) = %v, want %v", got, ext)
	}
}
