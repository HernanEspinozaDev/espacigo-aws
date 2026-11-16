package spacespg

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/HernanEspinozaDev/espaciGo/internal/spaces"
	"github.com/jackc/pgx/v5"
)

// GetCalendarConfiguration returns one consistent owner-only snapshot. The space
// FOR SHARE lock serializes with availability writers; no rows are modified.
func (r *Repository) GetCalendarConfiguration(ctx context.Context, owner, spaceID string) (spaces.CalendarConfiguration, error) {
	var out spaces.CalendarConfiguration
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return out, err
	}
	defer tx.Rollback(ctx)
	if err = lockActiveOwner(ctx, tx, owner); err != nil {
		return out, err
	}

	var zone *string
	var basePrice int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(zona_horaria,'America/Santiago'), precio_base_clp, actualizado_en
		FROM public.espacio WHERE id=$1 AND propietario_id=$2 AND estado <> 'eliminada' FOR SHARE`, spaceID, owner).
		Scan(&zone, &basePrice, &out.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return out, spaces.ErrNotFound
		}
		return out, err
	}
	out.SpaceID = spaceID
	out.TimeZone = "America/Santiago"
	if zone != nil && strings.TrimSpace(*zone) != "" {
		out.TimeZone = strings.TrimSpace(*zone)
	}
	if _, err = time.LoadLocation(out.TimeZone); err != nil {
		return spaces.CalendarConfiguration{}, fmt.Errorf("invalid space timezone %q: %w", out.TimeZone, err)
	}

	out.Rule = spaces.AvailabilityRule{SpaceID: spaceID, DiasOperacion: []int{1, 2, 3, 4, 5, 6, 7}, HoraApertura: "09:00", HoraCierre: "20:00", CheckInHora: "15:00", CheckOutHora: "12:00", TarifaBaseDia: basePrice, EstadiaMinima: 1, EstadiaMaxima: 30}
	var days []int16
	var open, close, checkIn, checkOut *string
	var rate *int
	var minStay, maxStay *int16
	var ruleUpdated time.Time
	err = tx.QueryRow(ctx, `SELECT dias_operacion,hora_apertura::text,hora_cierre::text,check_in_hora::text,check_out_hora::text,tarifa_base_dia,estadia_minima,estadia_maxima,actualizado_en FROM public.espacio_regla_disponibilidad WHERE espacio_id=$1`, spaceID).Scan(&days, &open, &close, &checkIn, &checkOut, &rate, &minStay, &maxStay, &ruleUpdated)
	if err == nil {
		if len(days) > 0 {
			out.Rule.DiasOperacion = make([]int, len(days))
			for i, v := range days {
				out.Rule.DiasOperacion[i] = int(v)
			}
		}
		if open != nil && len(*open) >= 5 {
			out.Rule.HoraApertura = (*open)[:5]
		}
		if close != nil && len(*close) >= 5 {
			out.Rule.HoraCierre = (*close)[:5]
		}
		if checkIn != nil && len(*checkIn) >= 5 {
			out.Rule.CheckInHora = (*checkIn)[:5]
		}
		if checkOut != nil && len(*checkOut) >= 5 {
			out.Rule.CheckOutHora = (*checkOut)[:5]
		}
		if rate != nil && *rate > 0 {
			out.Rule.TarifaBaseDia = *rate
		}
		if minStay != nil && *minStay > 0 {
			out.Rule.EstadiaMinima = int(*minStay)
		}
		if maxStay != nil && *maxStay > 0 {
			out.Rule.EstadiaMaxima = int(*maxStay)
		}
		out.Rule.ActualizadoEn = ruleUpdated
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return spaces.CalendarConfiguration{}, err
	}

	rows, err := tx.Query(ctx, `SELECT fecha::text,estado,tarifa_personalizada_clp,COALESCE(motivo,'') FROM public.espacio_calendario_override WHERE espacio_id=$1 ORDER BY fecha`, spaceID)
	if err != nil {
		return spaces.CalendarConfiguration{}, err
	}
	defer rows.Close()
	out.Overrides = make([]spaces.CalendarOverrideInput, 0)
	for rows.Next() {
		var v spaces.CalendarOverrideInput
		if err = rows.Scan(&v.Fecha, &v.Estado, &v.TarifaPersonalizadaCLP, &v.Motivo); err != nil {
			return spaces.CalendarConfiguration{}, err
		}
		out.Overrides = append(out.Overrides, v)
	}
	if err = rows.Err(); err != nil {
		return spaces.CalendarConfiguration{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return spaces.CalendarConfiguration{}, err
	}
	return out, nil
}
