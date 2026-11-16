package spaces

import (
	"context"
	"time"
)

type AvailabilityRule struct {
	SpaceID       string    `json:"space_id"`
	DiasOperacion []int     `json:"dias_operacion"`
	HoraApertura  string    `json:"hora_apertura"`
	HoraCierre    string    `json:"hora_cierre"`
	CheckInHora   string    `json:"check_in_hora"`
	CheckOutHora  string    `json:"check_out_hora"`
	TarifaBaseDia int       `json:"tarifa_base_dia"`
	EstadiaMinima int       `json:"estadia_minima"`
	EstadiaMaxima int       `json:"estadia_maxima"`
	ActualizadoEn time.Time `json:"actualizado_en"`
}

type CalendarOverride struct {
	ID                     string    `json:"id"`
	SpaceID                string    `json:"space_id"`
	Fecha                  string    `json:"fecha"`
	Estado                 string    `json:"estado"` // 'disponible', 'bloqueado', 'mantenimiento'
	TarifaPersonalizadaCLP *int      `json:"tarifa_personalizada_clp,omitempty"`
	Motivo                 string    `json:"motivo,omitempty"`
	CreadoEn               time.Time `json:"creado_en"`
}

type CalendarOverrideInput struct {
	Fecha                  string `json:"fecha"`
	Estado                 string `json:"estado"`
	TarifaPersonalizadaCLP *int   `json:"tarifa_personalizada_clp,omitempty"`
	Motivo                 string `json:"motivo,omitempty"`
}

type AvailabilityUpdateInput struct {
	TimeZone      *string                 `json:"time_zone,omitempty"`
	DiasOperacion []int                   `json:"dias_operacion,omitempty"`
	HoraApertura  *string                 `json:"hora_apertura,omitempty"`
	HoraCierre    *string                 `json:"hora_cierre,omitempty"`
	CheckInHora   *string                 `json:"check_in_hora,omitempty"`
	CheckOutHora  *string                 `json:"check_out_hora,omitempty"`
	TarifaBaseDia *int                    `json:"tarifa_base_dia,omitempty"`
	EstadiaMinima *int                    `json:"estadia_minima,omitempty"`
	EstadiaMaxima *int                    `json:"estadia_maxima,omitempty"`
	Overrides     []CalendarOverrideInput `json:"overrides,omitempty"`
}

type SlotAvailability struct {
	Name      string `json:"name"`
	Ticket    string `json:"ticket,omitempty"`
	Start     string `json:"start,omitempty"`
	End       string `json:"end,omitempty"`
	Available bool   `json:"available"`
}

type DayAvailability struct {
	Date     string             `json:"date"`
	Status   string             `json:"status"` // AVAILABLE, PARTIAL, BLOCKED, BOOKED, PAST
	PriceCLP int                `json:"price_clp"`
	Slots    []SlotAvailability `json:"slots"`
}

type MonthlyAvailability struct {
	Month        string            `json:"month"`
	TimeZone     string            `json:"time_zone"`
	RateUnit     string            `json:"rate_unit"`
	BasePriceCLP int               `json:"base_price_clp"`
	Days         []DayAvailability `json:"days"`
}

type AvailabilityRepository interface {
	GetMonthlyAvailability(ctx context.Context, spaceID, month string) (MonthlyAvailability, error)
	UpdateAvailability(ctx context.Context, owner, spaceID string, in AvailabilityUpdateInput) (AvailabilityRule, string, error)
}
