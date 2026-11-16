package spacespg

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
)

type occItem struct {
	start time.Time
	end   time.Time
}

func (r *Repository) GetMonthlyAvailability(ctx context.Context, spaceID, month string) (spaces.MonthlyAvailability, error) {
	if len(month) != 7 || month[4] != '-' {
		return spaces.MonthlyAvailability{}, spaces.ErrInvalid
	}
	year, err := strconv.Atoi(month[:4])
	if err != nil || year < 2000 || year > 2100 {
		return spaces.MonthlyAvailability{}, spaces.ErrInvalid
	}
	m, err := strconv.Atoi(month[5:7])
	if err != nil || m < 1 || m > 12 {
		return spaces.MonthlyAvailability{}, spaces.ErrInvalid
	}

	var estado, rateUnit string
	var zonePtr *string
	var precioBaseCLP int64
	err = r.pool.QueryRow(ctx, `
		SELECT estado, zona_horaria, modalidad_tarifa, precio_base_clp
		FROM public.espacio
		WHERE id = $1 AND estado='activa'
	`, spaceID).Scan(&estado, &zonePtr, &rateUnit, &precioBaseCLP)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return spaces.MonthlyAvailability{}, spaces.ErrNotFound
		}
		return spaces.MonthlyAvailability{}, err
	}
	if estado != "activa" {
		return spaces.MonthlyAvailability{}, spaces.ErrNotFound
	}

	timeZone := "America/Santiago"
	if zonePtr != nil && strings.TrimSpace(*zonePtr) != "" {
		timeZone = strings.TrimSpace(*zonePtr)
	}
	loc, err := time.LoadLocation(timeZone)
	if err != nil {
		return spaces.MonthlyAvailability{}, fmt.Errorf("invalid space timezone %q: %w", timeZone, err)
	}

	rule := spaces.AvailabilityRule{
		SpaceID:       spaceID,
		DiasOperacion: []int{1, 2, 3, 4, 5, 6, 7},
		HoraApertura:  "09:00",
		HoraCierre:    "20:00",
		CheckInHora:   "15:00",
		CheckOutHora:  "12:00",
		TarifaBaseDia: int(precioBaseCLP),
		EstadiaMinima: 1,
		EstadiaMaxima: 30,
	}

	var diasOpRaw []int16
	var hApertura, hCierre, chkIn, chkOut *string
	var tarifaBase *int
	var estMin, estMax *int16
	ruleErr := r.pool.QueryRow(ctx, `
		SELECT dias_operacion, hora_apertura::text, hora_cierre::text,
		       check_in_hora::text, check_out_hora::text, tarifa_base_dia,
		       estadia_minima, estadia_maxima
		FROM public.espacio_regla_disponibilidad
		WHERE espacio_id = $1
	`, spaceID).Scan(&diasOpRaw, &hApertura, &hCierre, &chkIn, &chkOut, &tarifaBase, &estMin, &estMax)
	if ruleErr == nil {
		if len(diasOpRaw) > 0 {
			rule.DiasOperacion = make([]int, len(diasOpRaw))
			for i, v := range diasOpRaw {
				rule.DiasOperacion[i] = int(v)
			}
		}
		if hApertura != nil && len(*hApertura) >= 5 {
			rule.HoraApertura = (*hApertura)[:5]
		}
		if hCierre != nil && len(*hCierre) >= 5 {
			rule.HoraCierre = (*hCierre)[:5]
		}
		if chkIn != nil && len(*chkIn) >= 5 {
			rule.CheckInHora = (*chkIn)[:5]
		}
		if chkOut != nil && len(*chkOut) >= 5 {
			rule.CheckOutHora = (*chkOut)[:5]
		}
		if tarifaBase != nil && *tarifaBase > 0 {
			rule.TarifaBaseDia = *tarifaBase
		}
		if estMin != nil && *estMin > 0 {
			rule.EstadiaMinima = int(*estMin)
		}
		if estMax != nil && *estMax > 0 {
			rule.EstadiaMaxima = int(*estMax)
		}
	} else if !errors.Is(ruleErr, pgx.ErrNoRows) {
		return spaces.MonthlyAvailability{}, ruleErr
	}

	monthStart := time.Date(year, time.Month(m), 1, 0, 0, 0, 0, loc)
	lastDay := time.Date(year, time.Month(m)+1, 0, 0, 0, 0, 0, loc).Day()
	monthEnd := time.Date(year, time.Month(m)+1, 1, 0, 0, 0, 0, loc)
	monthStartDate := fmt.Sprintf("%04d-%02d-01", year, m)
	monthEndDate := fmt.Sprintf("%04d-%02d-%02d", year, m, lastDay)

	overridesMap := make(map[string]spaces.CalendarOverride)
	ovRows, err := r.pool.Query(ctx, `
		SELECT fecha::text, estado, tarifa_personalizada_clp, COALESCE(motivo, '')
		FROM public.espacio_calendario_override
		WHERE espacio_id = $1 AND fecha >= $2::date AND fecha <= $3::date
	`, spaceID, monthStartDate, monthEndDate)
	if err != nil {
		return spaces.MonthlyAvailability{}, err
	}
	{
		defer ovRows.Close()
		for ovRows.Next() {
			var ov spaces.CalendarOverride
			var fechaStr string
			if err := ovRows.Scan(&fechaStr, &ov.Estado, &ov.TarifaPersonalizadaCLP, &ov.Motivo); err != nil {
				return spaces.MonthlyAvailability{}, err
			}
			ov.Fecha = fechaStr
			overridesMap[fechaStr] = ov
		}
		if err := ovRows.Err(); err != nil {
			return spaces.MonthlyAvailability{}, err
		}
	}

	var occupancies []occItem
	occRows, err := r.pool.Query(ctx, `
		SELECT lower(intervalo), upper(intervalo)
		FROM public.ocupacion
		WHERE espacio_id = $1 AND activo = true
		  AND intervalo && tstzrange($2, $3, '[)')
	`, spaceID, monthStart, monthEnd)
	if err != nil {
		return spaces.MonthlyAvailability{}, err
	}
	{
		defer occRows.Close()
		for occRows.Next() {
			var occ occItem
			if err := occRows.Scan(&occ.start, &occ.end); err != nil {
				return spaces.MonthlyAvailability{}, err
			}
			occupancies = append(occupancies, occ)
		}
		if err := occRows.Err(); err != nil {
			return spaces.MonthlyAvailability{}, err
		}
	}

	now := time.Now().In(loc)
	todayStr := now.Format("2006-01-02")

	diasOpSet := make(map[int]bool, len(rule.DiasOperacion))
	for _, d := range rule.DiasOperacion {
		diasOpSet[d] = true
	}

	openH, openM := parseClock(rule.HoraApertura, 9, 0)
	closeH, closeM := parseClock(rule.HoraCierre, 20, 0)

	days := make([]spaces.DayAvailability, 0, lastDay)
	for day := 1; day <= lastDay; day++ {
		dateStr := fmt.Sprintf("%04d-%02d-%02d", year, m, day)
		dayTime := time.Date(year, time.Month(m), day, 0, 0, 0, 0, loc)

		weekday := int(dayTime.Weekday())
		if weekday == 0 {
			weekday = 7
		}

		isPast := dateStr < todayStr
		isOperatingDay := diasOpSet[weekday]

		dayPrice := rule.TarifaBaseDia
		isOverrideBlocked := false
		isOverrideAvailable := false

		if ov, ok := overridesMap[dateStr]; ok {
			if ov.Estado == "bloqueado" || ov.Estado == "mantenimiento" {
				isOverrideBlocked = true
			} else if ov.Estado == "disponible" {
				isOverrideAvailable = true
			}
			if ov.TarifaPersonalizadaCLP != nil && *ov.TarifaPersonalizadaCLP > 0 {
				dayPrice = *ov.TarifaPersonalizadaCLP
			}
		}

		slot1Start := time.Date(year, time.Month(m), day, 9, 0, 0, 0, loc)
		slot1End := time.Date(year, time.Month(m), day, 13, 0, 0, 0, loc)

		slot2Start := time.Date(year, time.Month(m), day, 14, 0, 0, 0, loc)
		slot2End := time.Date(year, time.Month(m), day, 18, 0, 0, 0, loc)

		slotFullStart := time.Date(year, time.Month(m), day, openH, openM, 0, 0, loc)
		slotFullEnd := time.Date(year, time.Month(m), day, closeH, closeM, 0, 0, loc)

		s1Occupied := intervalOverlaps(slot1Start, slot1End, occupancies)
		s2Occupied := intervalOverlaps(slot2Start, slot2End, occupancies)
		sFullOccupied := intervalOverlaps(slotFullStart, slotFullEnd, occupancies)

		var status string
		var s1Avail, s2Avail, s3Avail bool

		if isPast {
			status = "PAST"
		} else if isOverrideBlocked || (!isOperatingDay && !isOverrideAvailable) {
			status = "BLOCKED"
		} else {
			s1Avail = !s1Occupied
			s2Avail = !s2Occupied
			s3Avail = !sFullOccupied && s1Avail && s2Avail

			if !s1Avail && !s2Avail {
				status = "BOOKED"
			} else if s1Avail && s2Avail && s3Avail {
				status = "AVAILABLE"
			} else {
				status = "PARTIAL"
			}
		}

		slots := []spaces.SlotAvailability{
			{
				Name:      "Mañana 09:00–13:00",
				Ticket:    "Mañana 09:00–13:00",
				Start:     "09:00",
				End:       "13:00",
				Available: s1Avail,
			},
			{
				Name:      "Tarde 14:00–18:00",
				Ticket:    "Tarde 14:00–18:00",
				Start:     "14:00",
				End:       "18:00",
				Available: s2Avail,
			},
			{
				Name:      "Día Completo",
				Ticket:    "Día Completo",
				Start:     rule.HoraApertura,
				End:       rule.HoraCierre,
				Available: s3Avail,
			},
		}

		days = append(days, spaces.DayAvailability{
			Date:     dateStr,
			Status:   status,
			PriceCLP: dayPrice,
			Slots:    slots,
		})
	}

	return spaces.MonthlyAvailability{
		Month:        month,
		TimeZone:     timeZone,
		RateUnit:     rateUnit,
		BasePriceCLP: rule.TarifaBaseDia,
		Days:         days,
	}, nil
}

func (r *Repository) UpdateAvailability(ctx context.Context, owner, spaceID string, in spaces.AvailabilityUpdateInput) (spaces.AvailabilityRule, string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return spaces.AvailabilityRule{}, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if err := lockActiveOwner(ctx, tx, owner); err != nil {
		return spaces.AvailabilityRule{}, "", err
	}

	var currentZone *string
	var estado string
	var currentUpdatedAt time.Time
	var currentBasePrice int64
	err = tx.QueryRow(ctx, `
		SELECT zona_horaria, estado, precio_base_clp, actualizado_en
		FROM public.espacio
		WHERE id = $1 AND propietario_id = $2
		FOR UPDATE
	`, spaceID, owner).Scan(&currentZone, &estado, &currentBasePrice, &currentUpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return spaces.AvailabilityRule{}, "", spaces.ErrNotFound
		}
		return spaces.AvailabilityRule{}, "", err
	}
	if estado == "eliminada" {
		return spaces.AvailabilityRule{}, "", spaces.ErrNotFound
	}
	if in.ExpectedUpdatedAt.IsZero() || !currentUpdatedAt.Equal(in.ExpectedUpdatedAt.UTC()) {
		return spaces.AvailabilityRule{}, "", spaces.ErrPreconditionFailed
	}

	timeZone := "America/Santiago"
	if currentZone != nil && strings.TrimSpace(*currentZone) != "" {
		timeZone = strings.TrimSpace(*currentZone)
	}

	if in.TimeZone != nil && strings.TrimSpace(*in.TimeZone) != "" {
		newZone := strings.TrimSpace(*in.TimeZone)
		if _, err := time.LoadLocation(newZone); err != nil {
			return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
		}
		if currentZone != nil && strings.TrimSpace(*currentZone) != "" && estado != "borrador" && newZone != strings.TrimSpace(*currentZone) {
			return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
		}
		timeZone = newZone
		if currentZone == nil || newZone != strings.TrimSpace(*currentZone) {
			_, err = tx.Exec(ctx, `
				UPDATE public.espacio
				SET zona_horaria = $1, actualizado_en = now()
				WHERE id = $2
			`, newZone, spaceID)
			if err != nil {
				return spaces.AvailabilityRule{}, "", err
			}
		}
	}

	if in.TarifaBaseDia != nil && *in.TarifaBaseDia <= 0 {
		return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
	}
	if in.EstadiaMinima != nil && *in.EstadiaMinima <= 0 {
		return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
	}
	if in.EstadiaMaxima != nil && *in.EstadiaMaxima <= 0 {
		return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
	}
	if in.EstadiaMinima != nil && in.EstadiaMaxima != nil && *in.EstadiaMinima > *in.EstadiaMaxima {
		return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
	}
	for _, d := range in.DiasOperacion {
		if d < 1 || d > 7 {
			return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
		}
	}

	hasRuleFields := len(in.DiasOperacion) > 0 || in.HoraApertura != nil || in.HoraCierre != nil ||
		in.CheckInHora != nil || in.CheckOutHora != nil || in.TarifaBaseDia != nil ||
		in.EstadiaMinima != nil || in.EstadiaMaxima != nil

	if hasRuleFields {
		var diasOpSmallint []int16
		if len(in.DiasOperacion) > 0 {
			diasOpSmallint = make([]int16, len(in.DiasOperacion))
			for i, v := range in.DiasOperacion {
				diasOpSmallint[i] = int16(v)
			}
		}

		tarifa := int(currentBasePrice)
		if in.TarifaBaseDia != nil {
			tarifa = *in.TarifaBaseDia
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO public.espacio_regla_disponibilidad (
				espacio_id, dias_operacion, hora_apertura, hora_cierre,
				check_in_hora, check_out_hora, tarifa_base_dia,
				estadia_minima, estadia_maxima, actualizado_en
			) VALUES (
				$1,
				COALESCE($2, '{1,2,3,4,5,6,7}'::smallint[]),
				COALESCE($3::time, '09:00'::time),
				COALESCE($4::time, '20:00'::time),
				COALESCE($5::time, '15:00'::time),
				COALESCE($6::time, '12:00'::time),
				$7,
				COALESCE($8, 1::smallint),
				COALESCE($9, 30::smallint),
				now()
			)
			ON CONFLICT (espacio_id) DO UPDATE SET
				dias_operacion = CASE WHEN $2 IS NOT NULL THEN $2 ELSE espacio_regla_disponibilidad.dias_operacion END,
				hora_apertura = CASE WHEN $3 IS NOT NULL THEN $3::time ELSE espacio_regla_disponibilidad.hora_apertura END,
				hora_cierre = CASE WHEN $4 IS NOT NULL THEN $4::time ELSE espacio_regla_disponibilidad.hora_cierre END,
				check_in_hora = CASE WHEN $5 IS NOT NULL THEN $5::time ELSE espacio_regla_disponibilidad.check_in_hora END,
				check_out_hora = CASE WHEN $6 IS NOT NULL THEN $6::time ELSE espacio_regla_disponibilidad.check_out_hora END,
				tarifa_base_dia = CASE WHEN $10 THEN $7 ELSE espacio_regla_disponibilidad.tarifa_base_dia END,
				estadia_minima = CASE WHEN $8 IS NOT NULL THEN $8 ELSE espacio_regla_disponibilidad.estadia_minima END,
				estadia_maxima = CASE WHEN $9 IS NOT NULL THEN $9 ELSE espacio_regla_disponibilidad.estadia_maxima END,
				actualizado_en = now()
		`, spaceID, diasOpSmallint, in.HoraApertura, in.HoraCierre, in.CheckInHora, in.CheckOutHora,
			tarifa, in.EstadiaMinima, in.EstadiaMaxima, in.TarifaBaseDia != nil)
		if err != nil {
			return spaces.AvailabilityRule{}, "", err
		}
	}

	for _, ov := range in.Overrides {
		date, dateErr := time.Parse("2006-01-02", ov.Fecha)
		if dateErr != nil || date.Format("2006-01-02") != ov.Fecha {
			return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
		}
		if ov.Estado != "disponible" && ov.Estado != "bloqueado" && ov.Estado != "mantenimiento" {
			return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
		}
		if ov.TarifaPersonalizadaCLP != nil && *ov.TarifaPersonalizadaCLP <= 0 {
			return spaces.AvailabilityRule{}, "", spaces.ErrInvalid
		}

		_, err = tx.Exec(ctx, `
			INSERT INTO public.espacio_calendario_override (
				espacio_id, fecha, estado, tarifa_personalizada_clp, motivo, creado_en
			) VALUES (
				$1, $2::date, $3, $4, $5, now()
			)
			ON CONFLICT (espacio_id, fecha) DO UPDATE SET
				estado = EXCLUDED.estado,
				tarifa_personalizada_clp = EXCLUDED.tarifa_personalizada_clp,
				motivo = EXCLUDED.motivo
		`, spaceID, ov.Fecha, ov.Estado, ov.TarifaPersonalizadaCLP, ov.Motivo)
		if err != nil {
			return spaces.AvailabilityRule{}, "", err
		}
	}

	resRule := spaces.AvailabilityRule{
		SpaceID:       spaceID,
		DiasOperacion: []int{1, 2, 3, 4, 5, 6, 7},
		HoraApertura:  "09:00",
		HoraCierre:    "20:00",
		CheckInHora:   "15:00",
		CheckOutHora:  "12:00",
		TarifaBaseDia: int(currentBasePrice),
		EstadiaMinima: 1,
		EstadiaMaxima: 30,
	}
	var diasOpRaw []int16
	var hApertura, hCierre, chkIn, chkOut *string
	var tarifaBase *int
	var estMin, estMax *int16
	var actEn time.Time
	err = tx.QueryRow(ctx, `
		SELECT dias_operacion, hora_apertura::text, hora_cierre::text,
		       check_in_hora::text, check_out_hora::text, tarifa_base_dia,
		       estadia_minima, estadia_maxima, actualizado_en
		FROM public.espacio_regla_disponibilidad
		WHERE espacio_id = $1
	`, spaceID).Scan(&diasOpRaw, &hApertura, &hCierre, &chkIn, &chkOut, &tarifaBase, &estMin, &estMax, &actEn)
	if err == nil {
		if len(diasOpRaw) > 0 {
			resRule.DiasOperacion = make([]int, len(diasOpRaw))
			for i, v := range diasOpRaw {
				resRule.DiasOperacion[i] = int(v)
			}
		}
		if hApertura != nil && len(*hApertura) >= 5 {
			resRule.HoraApertura = (*hApertura)[:5]
		}
		if hCierre != nil && len(*hCierre) >= 5 {
			resRule.HoraCierre = (*hCierre)[:5]
		}
		if chkIn != nil && len(*chkIn) >= 5 {
			resRule.CheckInHora = (*chkIn)[:5]
		}
		if chkOut != nil && len(*chkOut) >= 5 {
			resRule.CheckOutHora = (*chkOut)[:5]
		}
		if tarifaBase != nil {
			resRule.TarifaBaseDia = *tarifaBase
		}
		if estMin != nil {
			resRule.EstadiaMinima = int(*estMin)
		}
		if estMax != nil {
			resRule.EstadiaMaxima = int(*estMax)
		}
		resRule.ActualizadoEn = actEn
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return spaces.AvailabilityRule{}, "", err
	}

	// Inventory and overrides share the space revision with the other owner
	// editors. A rule timestamp is not a valid space ETag, especially when only
	// an override changed. The row stays locked from the CAS comparison above.
	err = tx.QueryRow(ctx, `UPDATE public.espacio
		SET actualizado_en = GREATEST(clock_timestamp(), actualizado_en + interval '1 microsecond')
		WHERE id = $1 AND propietario_id = $2
		RETURNING actualizado_en`, spaceID, owner).Scan(&resRule.ActualizadoEn)
	if err != nil {
		return spaces.AvailabilityRule{}, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return spaces.AvailabilityRule{}, "", err
	}

	return resRule, timeZone, nil
}

func parseClock(s string, defH, defM int) (int, int) {
	if len(s) >= 5 && s[2] == ':' {
		h, err1 := strconv.Atoi(s[:2])
		m, err2 := strconv.Atoi(s[3:5])
		if err1 == nil && err2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60 {
			return h, m
		}
	}
	return defH, defM
}

func intervalOverlaps(start, end time.Time, occupancies []occItem) bool {
	for _, o := range occupancies {
		if o.start.Before(end) && o.end.After(start) {
			return true
		}
	}
	return false
}
