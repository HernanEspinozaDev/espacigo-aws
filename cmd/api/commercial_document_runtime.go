package main

import (
	"context"
	"errors"
	"log"
	"net/url"
	"strings"
	"time"

	commercialdocumentspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialdocuments"
	commercialproposalspg "github.com/HernanEspinozaDev/espaciGo/internal/adapters/postgres/commercialproposals"
	"github.com/HernanEspinozaDev/espaciGo/internal/adapters/s3storage"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialdocument"
	"github.com/HernanEspinozaDev/espaciGo/internal/commercialproposal"
	"github.com/jackc/pgx/v5/pgxpool"
)

func buildCommercialDocumentRuntime(ctx context.Context, workers *trackedWorkerGroup, pool *pgxpool.Pool, cipher commercialproposal.Cipher, getenv func(string) string) (*s3storage.Store, namedWorker, error) {
	if cipher == nil {
		return nil, namedWorker{}, nil
	}
	store, bucket, err := localCommercialObjectStore(ctx, getenv, "contracts")
	if err != nil || store == nil {
		return nil, namedWorker{}, err
	}
	processor := &commercialdocument.Processor{
		Repo:     commercialdocumentspg.New(pool, commercialproposalspg.New(pool)),
		Store:    store,
		Cipher:   cipher,
		Renderer: commercialdocument.PDFRenderer{},
		Bucket:   bucket,
		Now:      time.Now,
	}
	worker := namedWorker{"commercial-contract-pdf", func() {
		workers.Go(func(ctx context.Context) {
			if err := processor.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Print("commercial contract PDF worker stopped")
			}
		})
	}}
	return store, worker, nil
}

func localCommercialObjectStore(ctx context.Context, getenv func(string) string, prefix string) (*s3storage.Store, string, error) {
	backend, bucket, _, err := runtimeFileStoreConfig(getenv)
	if err != nil {
		return nil, "", err
	}
	if backend != "s3" {
		return nil, "", nil
	}
	if endpoint := strings.TrimSpace(getenv("AWS_ENDPOINT_URL")); endpoint != "" {
		parsed, parseErr := url.Parse(endpoint)
		if parseErr != nil || (parsed.Scheme != "https" && parsed.Scheme != "http") ||
			(!isLoopbackIP(parsed.Hostname()) && parsed.Hostname() != "localhost") {
			return nil, "", errors.New("local commercial document storage requires the loopback S3 emulator")
		}
	}
	store, err := s3storage.NewWithPrefix(ctx, bucket, prefix)
	if err != nil {
		return nil, "", errors.New("commercial document storage initialization failed")
	}
	return store, bucket, nil
}
