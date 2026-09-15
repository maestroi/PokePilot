package main

import (
	"context"
	"database/sql/driver"
)

func (c *controlPlaneConn) Exec(query string, args []driver.Value) (driver.Result, error) {
	if execer, ok := c.Conn.(driver.Execer); ok {
		return execer.Exec(rebindPostgresQuery(query), args) //nolint:staticcheck // database/sql compatibility adapter
	}
	return nil, driver.ErrSkip
}

func (c *controlPlaneConn) Query(query string, args []driver.Value) (driver.Rows, error) {
	if queryer, ok := c.Conn.(driver.Queryer); ok {
		return queryer.Query(rebindPostgresQuery(query), args) //nolint:staticcheck // database/sql compatibility adapter
	}
	return nil, driver.ErrSkip
}

func (c *controlPlaneConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if execer, ok := c.Conn.(driver.ExecerContext); ok {
		return execer.ExecContext(ctx, rebindPostgresQuery(query), args)
	}
	return nil, driver.ErrSkip
}

func (c *controlPlaneConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	if queryer, ok := c.Conn.(driver.QueryerContext); ok {
		return queryer.QueryContext(ctx, rebindPostgresQuery(query), args)
	}
	return nil, driver.ErrSkip
}
