package conn

import (
	"fmt"
	"net/url"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	defaultPostgresHost    = "localhost"
	defaultPostgresPort    = 5432
	defaultPostgresSSLMode = "disable"
)

// Option defines connection options for PostgreSQL.
type Option struct {
	Host       string
	Port       int
	User       string
	Password   string
	Database   string
	SSLMode    string
	Params     map[string]string
	ConnString string
	Config     *gorm.Config
}

// Client wraps a PostgreSQL connection pool.
type Client struct {
	opt Option
	db  *gorm.DB
}

// New creates a PostgreSQL client from the provided options.
func New(option Option) (*Client, error) {
	connString, err := option.dsn()
	if err != nil {
		return nil, err
	}

	config := option.Config
	if config == nil {
		config = &gorm.Config{}
	}

	db, err := gorm.Open(postgres.Open(connString), config)
	if err != nil {
		return nil, err
	}

	return &Client{opt: option, db: db}, nil
}

// DB returns the underlying gorm.DB instance.
func (c *Client) DB() *gorm.DB {
	if c == nil {
		return nil
	}
	return c.db
}

// Close closes the underlying connection pool.
func (c *Client) Close() error {
	if c == nil || c.db == nil {
		return nil
	}
	sqlDB, err := c.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (opt Option) dsn() (string, error) {
	if opt.ConnString != "" {
		return opt.ConnString, nil
	}

	host := opt.Host
	if host == "" {
		host = defaultPostgresHost
	}

	port := opt.Port
	if port == 0 {
		port = defaultPostgresPort
	}

	sslMode := opt.SSLMode
	if sslMode == "" {
		sslMode = defaultPostgresSSLMode
	}

	u := &url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", host, port),
	}

	if opt.User != "" {
		if opt.Password != "" {
			u.User = url.UserPassword(opt.User, opt.Password)
		} else {
			u.User = url.User(opt.User)
		}
	}

	if opt.Database != "" {
		u.Path = "/" + opt.Database
	}

	query := url.Values{}
	query.Set("sslmode", sslMode)
	for key, value := range opt.Params {
		if key == "" {
			continue
		}
		query.Set(key, value)
	}
	if len(query) != 0 {
		u.RawQuery = query.Encode()
	}

	return u.String(), nil
}
