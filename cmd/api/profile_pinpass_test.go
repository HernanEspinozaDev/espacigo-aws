package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	password "github.com/HernanEspinozaDev/espaciGo/internal/adapters/password/bcrypt"
	"github.com/HernanEspinozaDev/espaciGo/internal/profilepin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresPinpassPasswordStepUp(t *testing.T) {
	dsn := os.Getenv("ESPACIGO_PROFILE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("run against the disposable profile database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET ROLE espacigo_runtime")
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()

	const secret = "Synthetic-Pass-596!"
	hash, err := (password.Hasher{}).Hash(secret)
	if err != nil {
		t.Fatal(err)
	}
	account, session := uuid.NewString(), uuid.NewString()
	if _, err := admin.Exec(ctx, `INSERT INTO public.usuario(id,correo_original,hash_clave,estado) VALUES($1,'reauth@example.test',$2,'activo')`, account, string(hash)); err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `INSERT INTO public.sesion(id,usuario_id,expira_en,ultima_actividad_en) VALUES ($1,$2,now()+interval '1 hour',now())`, session, account); err != nil {
		t.Fatal(err)
	}
	gate := pinpassPasswordStepUp{pool: pool}
	if err := gate.Verify(ctx, account, session, "password:"+secret); err != nil {
		t.Fatalf("current password and session rejected: %v", err)
	}
	if err := gate.Verify(ctx, account, uuid.NewString(), "password:"+secret); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("proof accepted with missing session: %v", err)
	}
	for range 5 {
		if err := gate.Verify(ctx, account, session, "password:synthetic-wrong"); !errors.Is(err, profilepin.ErrStepUpRejected) {
			t.Fatalf("wrong password result: %v", err)
		}
	}
	if err := gate.Verify(ctx, account, session, "password:"+secret); !errors.Is(err, profilepin.ErrStepUpRejected) {
		t.Fatalf("valid password bypassed lockout: %v", err)
	}
}
