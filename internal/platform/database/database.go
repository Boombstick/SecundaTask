package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/golang-migrate/migrate/v4"
	migratemysql "github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"taskservice/migrations"
)

const (
	errDuplicateEntry = 1062
	errNoReferenced   = 1452
)

const MigrationsTable = "migrations"

type txKey struct{}

type DB struct {
	gorm *gorm.DB
}

type Pool struct {
	MaxOpenConns    int
	MaxIdleConns    int
	ConnMaxLifetime time.Duration
}

func Open(ctx context.Context, dsn string, pool Pool) (*DB, error) {
	g, err := gorm.Open(gormmysql.Open(dsn), &gorm.Config{
		Logger:                 logger.Default.LogMode(logger.Silent),
		NowFunc:                func() time.Time { return time.Now().UTC() },
		SkipDefaultTransaction: true,
	})
	if err != nil {
		return nil, fmt.Errorf("open mysql: %w", err)
	}
	sqlDB, err := g.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(pool.MaxOpenConns)
	sqlDB.SetMaxIdleConns(pool.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(pool.ConnMaxLifetime)
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("ping mysql: %w", err)
	}
	return &DB{gorm: g}, nil
}

func Migrate(dsn string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	conn, err := sql.Open("mysql", dsn)
	if err != nil {
		return err
	}
	defer conn.Close()
	driver, err := migratemysql.WithInstance(conn, &migratemysql.Config{MigrationsTable: MigrationsTable})
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "mysql", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}

func (d *DB) WithinTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if _, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return fn(ctx)
	}
	return d.gorm.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

func (d *DB) Conn(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey{}).(*gorm.DB); ok {
		return tx
	}
	return d.gorm.WithContext(ctx)
}

func (d *DB) Now() time.Time { return d.gorm.NowFunc() }

func (d *DB) ScanOne(ctx context.Context, dst any, notFound error, query string, args ...any) error {
	res := d.Conn(ctx).Raw(query, args...).Scan(dst)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return notFound
	}
	return nil
}

func (d *DB) Exec(ctx context.Context, query string, args ...any) error {
	return d.Conn(ctx).Exec(query, args...).Error
}

func (d *DB) Ping(ctx context.Context) error {
	sqlDB, err := d.gorm.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

func (d *DB) Close() error {
	sqlDB, err := d.gorm.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func IsDuplicate(err error) bool { return isMySQLErr(err, errDuplicateEntry) }

func IsMissingReference(err error) bool { return isMySQLErr(err, errNoReferenced) }

func isMySQLErr(err error, code uint16) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == code
}
