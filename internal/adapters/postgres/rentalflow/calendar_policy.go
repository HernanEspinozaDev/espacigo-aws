package rentalflowpg

import (
	"context"
	"errors"
	"strconv"

	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/pricing"
	"github.com/HernanEspinozaDev/espaciGo/internal/rentalflow"
	"github.com/jackc/pgx/v5"
)

// calendarRestrictions adds the V98 host exceptions to the existing weekly and
// occupancy authority. It never writes, quotes a price, or creates a hold.
type calendarRestrictions struct {
	loc                           *time.Location
	days                          map[int]bool
	open, close, minStay, maxStay int
	overrides                     map[string]string
}

func (r *Repository) readCalendarRestrictions(ctx context.Context, l rentalflow.Listing, start, end time.Time) (calendarRestrictions, error) {
	loc, err := time.LoadLocation(l.TimeZone)
	if err != nil {
		return calendarRestrictions{}, rentalflow.ErrInvalid
	}
	p := calendarRestrictions{loc: loc, days: map[int]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true}, open: 9 * 60, close: 20 * 60, minStay: 1, maxStay: 30, overrides: map[string]string{}}
	var days []int16
	var open, close string
	var minStay, maxStay int
	err = r.db.QueryRow(ctx, `SELECT dias_operacion,to_char(hora_apertura,'HH24:MI'),to_char(hora_cierre,'HH24:MI'),estadia_minima,estadia_maxima FROM public.espacio_regla_disponibilidad WHERE espacio_id=$1`, l.ID).Scan(&days, &open, &close, &minStay, &maxStay)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return calendarRestrictions{}, err
	}
	if err == nil {
		p.days = map[int]bool{}
		for _, d := range days {
			if d < 1 || d > 7 {
				return calendarRestrictions{}, rentalflow.ErrUnavailable
			}
			p.days[int(d)] = true
		}
		var ok bool
		p.open, ok = calendarClock(open)
		if !ok {
			return calendarRestrictions{}, rentalflow.ErrUnavailable
		}
		p.close, ok = calendarClock(close)
		if !ok || p.open >= p.close || minStay < 1 || maxStay < minStay {
			return calendarRestrictions{}, rentalflow.ErrUnavailable
		}
		p.minStay, p.maxStay = minStay, maxStay
	}
	rows, err := r.db.Query(ctx, `SELECT fecha::text,estado FROM public.espacio_calendario_override WHERE espacio_id=$1 AND fecha >= $2::date AND fecha <= $3::date`, l.ID, start.In(loc).Format("2006-01-02"), end.In(loc).Format("2006-01-02"))
	if err != nil {
		return calendarRestrictions{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var date, state string
		if err = rows.Scan(&date, &state); err != nil {
			return calendarRestrictions{}, err
		}
		if !rentalflow.ValidDate(date) || (state != "disponible" && state != "bloqueado" && state != "mantenimiento") {
			return calendarRestrictions{}, rentalflow.ErrUnavailable
		}
		if _, duplicate := p.overrides[date]; duplicate {
			return calendarRestrictions{}, rentalflow.ErrUnavailable
		}
		p.overrides[date] = state
	}
	if err = rows.Err(); err != nil {
		return calendarRestrictions{}, err
	}
	return p, nil
}
func calendarClock(value string) (int, bool) {
	if len(value) != 5 || value[2] != ':' {
		return 0, false
	}
	for _, i := range []int{0, 1, 3, 4} {
		if value[i] < '0' || value[i] > '9' {
			return 0, false
		}
	}
	h, e1 := strconv.Atoi(value[:2])
	m, e2 := strconv.Atoi(value[3:])
	if e1 != nil || e2 != nil || h > 24 || m > 59 || (h == 24 && m != 0) {
		return 0, false
	}
	return h*60 + m, true
}
func calendarCivil(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
func (p calendarRestrictions) allows(l rentalflow.Listing, in rentalflow.Interval) bool {
	if !in.EndAt.After(in.StartAt) {
		return false
	}
	start, end := in.StartAt.In(p.loc), in.EndAt.In(p.loc)
	first, last := calendarCivil(start), calendarCivil(end)
	if l.RateUnit == "dia" {
		units, err := pricing.BilledUnits("dia", in.StartAt, in.EndAt, l.TimeZone)
		if err != nil || units < int64(p.minStay) || units > int64(p.maxStay) {
			return false
		}
	}
	if l.RateUnit == "hora" && (p.open != 0 || p.close != 24*60) {
		endMinutes := end.Hour()*60 + end.Minute()
		if last.Equal(first.AddDate(0, 0, 1)) && endMinutes == 0 && end.Second() == 0 && end.Nanosecond() == 0 {
			endMinutes = 24 * 60
		} else if !last.Equal(first) {
			return false
		}
		if start.Hour()*60+start.Minute() < p.open || endMinutes > p.close || (endMinutes == p.close && (end.Second() != 0 || end.Nanosecond() != 0)) {
			return false
		}
	}
	if end.Hour() != 0 || end.Minute() != 0 || end.Second() != 0 || end.Nanosecond() != 0 {
		last = last.AddDate(0, 0, 1)
	}
	count := 0
	for date := first; date.Before(last); date = date.AddDate(0, 0, 1) {
		count++
		if count > 366 {
			return false
		}
		state := p.overrides[date.Format("2006-01-02")]
		if state == "bloqueado" || state == "mantenimiento" {
			return false
		}
		day := int(date.Weekday())
		if day == 0 {
			day = 7
		}
		if !p.days[day] && state != "disponible" {
			return false
		}
	}
	return count > 0
}
