package migrator

import (
	"context"
	"strings"
	"testing"
)

func singleOwnerContractFixture() ([]Migration, SingleOwnerMigrationContract) {
	migrations := []Migration{
		{Version: 1, Name: "V000001__owner_base.sql", SQL: []byte("CREATE TABLE public.ocupacion(id bigint PRIMARY KEY);\n")},
		{Version: 2, Name: "V000002__owner_delta.sql", SQL: []byte("DO $migration$\nBEGIN\nALTER TABLE public.ocupacion ADD COLUMN probe boolean;\nEND\n$migration$;")},
	}
	for i := range migrations {
		migrations[i].Checksum = sha256Hex(migrations[i].SQL)
	}
	return migrations, SingleOwnerMigrationContract{
		InventorySHA256: inventoryFingerprint(migrations),
		Version:         2, Name: migrations[1].Name, Checksum: migrations[1].Checksum,
		Target: SingleOwnerMigrationTarget{
			Host: "/isolated/socket", Port: 5432, Database: "reviewed_local", DatabaseOID: 1234,
			SystemIdentifier: "123456789", ServerVersionNum: 180007, SessionUser: "postgres",
			OccupancyOwner: "postgres", LedgerOwner: "postgres",
		},
	}
}

func TestSingleOwnerContractRejectsUnpinnedInputs(t *testing.T) {
	migrations, valid := singleOwnerContractFixture()
	if err := validateSingleOwnerMigrationContract(valid, migrations); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(*SingleOwnerMigrationContract){

		"inventory":    func(c *SingleOwnerMigrationContract) { c.InventorySHA256 = strings.Repeat("0", 64) },
		"version":      func(c *SingleOwnerMigrationContract) { c.Version = 1 },
		"name":         func(c *SingleOwnerMigrationContract) { c.Name = "V000002__another_delta.sql" },
		"checksum":     func(c *SingleOwnerMigrationContract) { c.Checksum = strings.Repeat("0", 64) },
		"host":         func(c *SingleOwnerMigrationContract) { c.Target.Host = "cloud.example" },
		"database":     func(c *SingleOwnerMigrationContract) { c.Target.Database = "" },
		"oid":          func(c *SingleOwnerMigrationContract) { c.Target.DatabaseOID = 0 },
		"cluster":      func(c *SingleOwnerMigrationContract) { c.Target.SystemIdentifier = "" },
		"engine":       func(c *SingleOwnerMigrationContract) { c.Target.ServerVersionNum = 170000 },
		"session":      func(c *SingleOwnerMigrationContract) { c.Target.SessionUser = "" },
		"owner":        func(c *SingleOwnerMigrationContract) { c.Target.OccupancyOwner = "" },
		"ledger-owner": func(c *SingleOwnerMigrationContract) { c.Target.LedgerOwner = "" },
	} {
		t.Run(name, func(t *testing.T) {
			invalid := valid
			change(&invalid)
			if err := validateSingleOwnerMigrationContract(invalid, migrations); err == nil {
				t.Fatal("unreviewed contract accepted")
			}
		})
	}
}

func TestSingleOwnerContractRehashesSQLAndRequiresAtomicDO(t *testing.T) {
	migrations, contract := singleOwnerContractFixture()
	migrations[1].SQL = []byte("SELECT 1")
	if err := validateSingleOwnerMigrationContract(contract, migrations); err == nil {
		t.Fatal("changed SQL accepted under old checksum")
	}
	migrations[1].Checksum = sha256Hex(migrations[1].SQL)
	contract.Checksum = migrations[1].Checksum
	contract.InventorySHA256 = inventoryFingerprint(migrations)
	if err := validateSingleOwnerMigrationContract(contract, migrations); err == nil {
		t.Fatal("non-DO migration accepted")
	}
}

func TestSingleOwnerMigrationRequiresConnection(t *testing.T) {
	_, contract := singleOwnerContractFixture()
	if _, err := RunSingleOwnerMigration(context.Background(), nil, "unused", contract); err == nil {
		t.Fatal("nil connection accepted")
	}
}
