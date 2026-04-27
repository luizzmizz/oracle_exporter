package target

import (
	"context"
	"database/sql"
	"strings"
	"time"

	_ "github.com/godror/godror"

	"github.com/luizzmizz/oracle_exporter/internal/config"
)

type Meta struct {
	IsCDB  bool
	IsASM  bool
	DBName string
}

type Target struct {
	Name string
	DB   *sql.DB
	Meta Meta
}

func Open(name string, cfg config.TargetConfig, timeout time.Duration) (*Target, error) {
	db, err := sql.Open("godror", cfg.DSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(3)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(5 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	meta, err := detect(ctx, db)
	if err != nil {
		db.Close()
		return nil, err
	}

	return &Target{Name: name, DB: db, Meta: meta}, nil
}

func detect(ctx context.Context, db *sql.DB) (Meta, error) {
	var instanceName string
	if err := db.QueryRowContext(ctx, "SELECT instance_name FROM v$instance").Scan(&instanceName); err != nil {
		return Meta{}, err
	}

	if strings.HasPrefix(instanceName, "+ASM") {
		return Meta{IsASM: true, DBName: instanceName}, nil
	}

	var dbName, cdb string
	err := db.QueryRowContext(ctx,
		"SELECT db_unique_name, NVL(cdb,'NO') FROM v$database",
	).Scan(&dbName, &cdb)
	if err != nil {
		// Pre-12c or detection failed — treat as non-CDB
		return Meta{DBName: instanceName}, nil
	}

	return Meta{IsCDB: cdb == "YES", DBName: dbName}, nil
}
