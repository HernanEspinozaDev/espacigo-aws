package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/HernanEspinozaDev/espaciGo/internal/kyb"
)

type kybReminderMailer interface {
	SendLocalBookingNotice(context.Context, string, string, string) error
}

func runKYBComplianceWorker(ctx context.Context, service *kyb.Service, mailer kybReminderMailer, now func() time.Time) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for ctx.Err() == nil {
		if _, err := service.RunGraceMaintenance(ctx, now(), 100); err != nil && ctx.Err() == nil {
			log.Print("KYB compliance maintenance pass failed")
		}
		if mailer != nil {
			if err := dispatchKYBReminders(ctx, service, mailer, now); err != nil && ctx.Err() == nil {
				log.Print("KYB compliance email pass failed")
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func dispatchKYBReminders(ctx context.Context, service *kyb.Service, mailer kybReminderMailer, now func() time.Time) error {
	location, err := time.LoadLocation("America/Santiago")
	if err != nil {
		return err
	}
	for range 100 {
		notice, ok, err := service.ClaimReminderEmail(ctx, now())
		if err != nil || !ok {
			return err
		}
		subject := "Documentación comercial de EspaciGo"
		rule := "Revisa la documentación del titular y el respaldo de cada espacio en la sección Documentación comercial de EspaciGo."
		if notice.Stage == kyb.ReminderDeadline {
			rule = "Venció el plazo documental. Regulariza los documentos pendientes para volver a publicar o administrar los espacios afectados. Puedes seguir arrendando espacios como cliente."
		}
		body := fmt.Sprintf("Fecha límite: %s (Chile continental).\n\n%s", notice.DueAt.In(location).Format("02/01/2006 15:04"), rule)
		if notice.Stage == kyb.ReminderReactivated {
			subject = "Perfil comercial habilitado en EspaciGo"
			body = "Tu perfil comercial se habilitó nuevamente. Revisa el estado documental de tus espacios en EspaciGo para saber cuáles están disponibles para publicar."
		}
		mailCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		sendErr := mailer.SendLocalBookingNotice(mailCtx, notice.RecipientEmail, subject, body)
		cancel()
		code := ""
		if sendErr != nil {
			code = "mail_delivery_failed"
		}
		if err := service.FinishReminderEmail(ctx, notice, sendErr == nil, code, now()); err != nil {
			return err
		}
	}
	return nil
}
