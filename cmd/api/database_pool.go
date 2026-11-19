package main

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newDatabasePool(ctx context.Context, cfg config) (*pgxpool.Pool, func(), error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.databaseURL())
	if err != nil {
		return nil, nil, errors.New("invalid database connection configuration")
	}
	poolConfig.MaxConns = cfg.dbPoolMaxConns
	poolConfig.MinConns = cfg.dbPoolMinConns
	poolConfig.ConnConfig.ConnectTimeout = 3 * time.Second
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = "5000"
	if cfg.production {
		if poolConfig.ConnConfig.TLSConfig == nil || poolConfig.ConnConfig.TLSConfig.InsecureSkipVerify {
			return nil, nil, errors.New("production database requires verified TLS")
		}
		poolConfig.ConnConfig.TLSConfig.MinVersion = tls.VersionTLS13
		for _, fallback := range poolConfig.ConnConfig.Fallbacks {
			if fallback.TLSConfig == nil || fallback.TLSConfig.InsecureSkipVerify {
				return nil, nil, errors.New("production database fallback requires verified TLS")
			}
			fallback.TLSConfig.MinVersion = tls.VersionTLS13
		}
	}
	if cfg.dbSetRole != "" {
		role := pgx.Identifier{cfg.dbSetRole}.Sanitize()
		poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
			_, err := conn.Exec(ctx, "SET ROLE "+role)
			if err != nil {
				return errors.New("could not assume configured database role")
			}
			return nil
		}
	}
	closeConnector := func() {}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, nil, errors.New("database pool initialization failed")
	}
	return pool, closeConnector, nil
}
