# sshdb

sshdb enables accessing databases over an ssh connection (tunnel) by registering a mock database (databases/sql) to tunnel data via the underlying database driver.

Packages for the following databases/packages are included:

- mysql [github.com/go-sql-driver/mysql](https://pkg.go.dev/github.com/go-sql-driver/mysql)
- mssql [github.com/denisekom/go-mssqldb](https://pkg.go.dev/github.com/denisenkom/go-mssqldb)
- postgres [github.com/jackc/pgx/v4](https://pkg.go.dev/github.com/jackc/pgx/v4)
