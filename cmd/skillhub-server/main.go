package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/skillhubhq/skillhub/internal/hub"
	"github.com/skillhubhq/skillhub/internal/hubhttp"
	"github.com/skillhubhq/skillhub/internal/serverconfig"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return
		}
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

// 备份中的标识可能包含控制字符；摘要必须保持为单条可安全显示的记录。
func maintenanceSummary(operation, registry string, schema, packages int) string {
	return fmt.Sprintf("%s complete: registry %q, schema %d, %d packages\n", operation, registry, schema, packages)
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		_, err := fmt.Fprint(os.Stdout, serverHelp)
		return err
	}
	flags := flag.NewFlagSet(args[0], flag.ContinueOnError)
	data := flags.String("data", ".local/hub", "persistent data directory")
	configFile := flags.String("config", "", "explicit YAML configuration file")
	databaseDriver := flags.String("database-driver", "", "database driver: sqlite or postgres")
	databaseDSNFile := flags.String("database-dsn-file", "", "file containing PostgreSQL connection string")
	loadDatabase := func() (serverconfig.Config, error) {
		cfg, err := serverconfig.Load(*configFile)
		if err != nil {
			return cfg, err
		}
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "data":
				cfg.Data = *data
			case "database-driver":
				cfg.Database.Driver = *databaseDriver
			case "database-dsn-file":
				cfg.Database.DSNFile = *databaseDSNFile
			}
		})
		return cfg, nil
	}
	openDatabase := func() (*hub.Store, error) {
		cfg, err := loadDatabase()
		if err != nil {
			return nil, err
		}
		if err := cfg.Validate(); err != nil {
			return nil, err
		}
		db, err := cfg.DatabaseConfig()
		if err != nil {
			return nil, err
		}
		*data = cfg.Data
		return hub.OpenDatabase(cfg.Data, db)
	}

	switch args[0] {
	case "backup", "verify-backup", "restore":
		input := flags.String("input", "", "backup directory to verify or restore")
		output := flags.String("output", "", "new backup directory")
		timeout := flags.Duration("timeout", 10*time.Minute, "maximum maintenance duration")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 || *timeout <= 0 {
			return errors.New("unexpected argument or invalid timeout")
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		ctx, cancel := context.WithTimeout(ctx, *timeout)
		defer cancel()
		cfg, configErr := loadDatabase()
		if configErr != nil {
			return configErr
		}
		*data = cfg.Data
		if cfg.Database.Driver != "sqlite" {
			dbConfig, e := cfg.DatabaseConfig()
			if e != nil && args[0] != "verify-backup" {
				return e
			}
			var pg hub.PostgresBackupInfo
			switch args[0] {
			case "backup":
				if *output == "" || *input != "" {
					return errors.New("backup requires --output")
				}
				pg, e = hub.BackupPostgres(ctx, dbConfig, *data, *output)
			case "verify-backup":
				if *input == "" || *output != "" {
					return errors.New("verify-backup requires --input")
				}
				pg, e = hub.VerifyPostgresBackup(ctx, *input)
			case "restore":
				if *input == "" || *output != "" {
					return errors.New("restore requires --input and new data/database destinations")
				}
				pg, e = hub.RestorePostgres(ctx, dbConfig, *input, *data)
			}
			if e != nil {
				return e
			}
			fmt.Print(maintenanceSummary(args[0], pg.RegistryID, pg.Schema, pg.Packages))
			return nil
		}
		var info hub.BackupInfo
		var err error
		switch args[0] {
		case "backup":
			if *input != "" || *output == "" {
				return errors.New("backup requires --output and does not accept --input")
			}
			entry, statErr := os.Lstat(filepath.Join(*data, "hub.db"))
			if statErr != nil || !entry.Mode().IsRegular() {
				return errors.New("backup requires an existing Hub database")
			}
			store, openErr := hub.Open(*data)
			if openErr != nil {
				return openErr
			}
			info, err = store.Backup(ctx, *output)
			err = errors.Join(err, store.Close())
		case "verify-backup":
			if *input == "" || *output != "" {
				return errors.New("verify-backup requires --input and does not accept --output")
			}
			info, err = hub.VerifyBackup(ctx, *input)
		case "restore":
			if *input == "" || *output != "" {
				return errors.New("restore requires --input and a new --data directory")
			}
			info, err = hub.RestoreBackup(ctx, *input, *data)
		}
		if err != nil {
			return err
		}
		fmt.Print(maintenanceSummary(args[0], info.RegistryID, info.Schema, info.Packages))
		if args[0] == "restore" {
			fmt.Println("Previous sessions and pending device authorizations were removed; clients must sign in again.")
		}
		return nil
	case "recover-admin":
		username := flags.String("username", "", "existing local account to recover")
		stdin := flags.Bool("password-stdin", false, "read replacement password from standard input")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if !*stdin || *username == "" || flags.NArg() != 0 {
			return errors.New("recover-admin requires --username and --password-stdin; stop the Hub first")
		}
		cfg, configErr := loadDatabase()
		if configErr != nil {
			return configErr
		}
		*data = cfg.Data
		if entry, err := os.Lstat(filepath.Join(*data, "hub.db")); cfg.Database.Driver == "sqlite" && (err != nil || !entry.Mode().IsRegular()) {
			return errors.New("recovery requires an existing Hub database")
		}
		unlock, err := hub.AcquireServingLock(*data)
		if err != nil {
			return err
		}
		defer unlock()
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
		if err != nil {
			return err
		}
		if len(raw) > 1024 {
			return errors.New("password input too large")
		}
		password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		store, err := openDatabase()
		if err != nil {
			return err
		}
		defer store.Close()
		if err = store.RecoverAdministrator(context.Background(), *username, password); err != nil {
			return err
		}
		fmt.Println("Local administrator recovered; previous sessions revoked. Restart the Hub and sign in again.")
		return nil
	case "init":
		username := flags.String("admin", "admin", "initial administrator username")
		stdin := flags.Bool("password-stdin", false, "read initial password from standard input")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected positional argument")
		}
		if !*stdin {
			return errors.New("use --password-stdin; minimum 12 and maximum 72 password bytes")
		}
		raw, err := io.ReadAll(io.LimitReader(os.Stdin, 1025))
		if err != nil {
			return err
		}
		if len(raw) > 1024 {
			return errors.New("password input too large")
		}
		password := strings.TrimSuffix(strings.TrimSuffix(string(raw), "\n"), "\r")
		store, err := openDatabase()
		if err != nil {
			return err
		}
		defer store.Close()
		user, err := store.Bootstrap(context.Background(), *username, password)
		if err != nil {
			return err
		}
		fmt.Printf("Initialized registry %s with administrator %s\n", store.RegistryID(), user.Username)
		return nil
	case "serve", "check-config":
		listen := flags.String("listen", "127.0.0.1:8080", "HTTP bind address")
		publicURL := flags.String("public-url", "http://127.0.0.1:8080", "external HTTPS origin (HTTP permitted on loopback)")
		name := flags.String("name", "SkillHub", "instance display name")
		trustedProxies := flags.String("trusted-proxies", "", "comma-separated trusted reverse proxy CIDRs; proxy must overwrite X-Forwarded-For")
		private := flags.Bool("private", true, "require login for catalog reads")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if flags.NArg() != 0 {
			return errors.New("unexpected positional argument")
		}
		cfg, err := loadDatabase()
		if err != nil {
			return err
		}
		// 只有显式传入的参数才覆盖文件，保留 false 等有意义的零值。
		flags.Visit(func(f *flag.Flag) {
			switch f.Name {
			case "data":
				cfg.Data = *data
			case "listen":
				cfg.Listen = *listen
			case "public-url":
				cfg.PublicURL = *publicURL
			case "name":
				cfg.Name = *name
			case "private":
				cfg.Private = *private
			case "trusted-proxies":
				cfg.TrustedProxies = strings.Split(*trustedProxies, ",")
			}
		})
		if err := cfg.Validate(); err != nil {
			return err
		}
		providers, err := cfg.Providers()
		if err != nil {
			return err
		}
		dbConfig, err := cfg.DatabaseConfig()
		if err != nil {
			return err
		}

		if args[0] == "check-config" {
			fmt.Println("Configuration valid. Network, identity-provider credentials and data-directory access have not been tested.")
			return nil
		}
		*data = cfg.Data
		unlock, err := hub.AcquireServingLock(*data)
		if err != nil {
			return err
		}
		defer unlock()
		store, err := hub.OpenDatabase(*data, dbConfig)
		if err != nil {
			return err
		}
		defer store.Close()
		logger := slog.New(slog.NewJSONHandler(os.Stderr, nil))
		handler, err := hubhttp.New(store, hubhttp.Config{Site: cfg.Site, Providers: providers, Policy: cfg.Policy(), PublicURL: cfg.PublicURL, DisplayName: cfg.Name, Private: cfg.Private, Logger: logger, TrustedProxies: cfg.TrustedProxies})
		if err != nil {
			return err
		}
		defer handler.CloseBackground()
		server := &http.Server{Addr: cfg.Listen, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 60 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		done := make(chan error, 1)
		go func() {
			logger.Info("hub listening", "address", cfg.Listen, "registry_id", store.RegistryID())
			done <- server.ListenAndServe()
		}()
		select {
		case err := <-done:
			if errors.Is(err, http.ErrServerClosed) {
				return nil
			}
			return err
		case <-store.DatabaseLeaseLost():
			_ = server.Close()
			return errors.New("database ownership lost; server stopped")
		case <-ctx.Done():
			shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			return server.Shutdown(shutdown)
		}
	default:
		return errors.New("unknown server command")
	}
}

const serverHelp = `SkillHub Server
Run and maintain a self-hosted skill community.

Usage:
  skillhub-server COMMAND [options]

Start a Hub:
  init            Create the Hub and its first administrator
  check-config    Validate deployment configuration
  serve           Start the Hub and embedded Web UI

Maintain data:
  backup          Create a paired database and package backup
  verify-backup   Verify a backup before recovery
  restore         Restore into a new destination
  recover-admin   Recover local administrator access

Examples:
  skillhub-server init --data .local/hub --admin admin --password-stdin
  skillhub-server serve --data .local/hub
  skillhub-server check-config --config /etc/skillhub/hub.yaml

Use "skillhub-server COMMAND --help" for command options.
`
